package ai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pendingAction is a write/shell tool call awaiting user approval.
type pendingAction struct {
	ID         string   `json:"id"`
	Tool       string   `json:"tool"`
	Args       string   `json:"args"`
	Command    string   `json:"command,omitempty"`
	Level      string   `json:"level"`
	RiskHints  []string `json:"riskHints,omitempty"`
	ToolCallID string   `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// pendingTTL: how long an approval request stays valid.
const pendingTTL = 5 * time.Minute

// pendingStore keeps approval requests in memory, keyed by id, with the
// username that must approve them.
type pendingStore struct {
	mu      sync.Mutex
	entries map[string]pendingEntry
}

type pendingEntry struct {
	action pendingAction
	user   string
}

var pendings = &pendingStore{entries: map[string]pendingEntry{}}

// newPending registers an approval request owned by user. toolCallID links
// the request back to the tool message in the chat session.
func newPending(user, tool, argsJSON, command string, level safetyLevel, hints []string, toolCallID string) pendingAction {
	id := randID()
	now := time.Now()
	a := pendingAction{
		ID: id, Tool: tool, Args: argsJSON, Command: command,
		Level: level.String(), RiskHints: hints, ToolCallID: toolCallID,
		CreatedAt: now, ExpiresAt: now.Add(pendingTTL),
	}
	pendings.mu.Lock()
	pendings.entries[id] = pendingEntry{action: a, user: user}
	pendings.mu.Unlock()
	pendings.gc()
	return a
}

// takePending removes and returns the entry if id exists, belongs to user
// and has not expired.
func takePending(user, id string) (pendingAction, error) {
	pendings.mu.Lock()
	defer pendings.mu.Unlock()
	e, ok := pendings.entries[id]
	if !ok {
		return pendingAction{}, fmt.Errorf("批准请求不存在或已处理")
	}
	if e.user != user {
		return pendingAction{}, fmt.Errorf("只能处理自己发起的批准请求")
	}
	delete(pendings.entries, id)
	if time.Now().After(e.action.ExpiresAt) {
		return pendingAction{}, fmt.Errorf("批准请求已过期，请重新发起")
	}
	return e.action, nil
}

// gc drops expired entries lazily.
func (p *pendingStore) gc() {
	now := time.Now()
	for id, e := range p.entries {
		if now.After(e.action.ExpiresAt) {
			delete(p.entries, id)
		}
	}
}

func randID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// tokenUsage accumulates prompt/completion tokens across one chat round.
type tokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u *tokenUsage) add(o tokenUsage) {
	u.PromptTokens += o.PromptTokens
	u.CompletionTokens += o.CompletionTokens
	u.TotalTokens += o.TotalTokens
}

// ── chat sessions (per user, in memory, sliding TTL) ────────────────

const (
	sessionTTL     = 30 * time.Minute
	maxSessionMsgs = 80
	maxToolRounds  = 8 // tool-calling loops allowed per chat request
)

type aiSession struct {
	mu         sync.Mutex
	messages   []chatMessage
	lastActive time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]*aiSession
}

var sessions = &sessionStore{sessions: map[string]*aiSession{}}

// sessionKey namespaces sessions by user and conversation id.
func sessionKey(user string, conv int64) string {
	return user + ":" + strconv.FormatInt(conv, 10)
}

// getSession returns (creating if needed) the chat session for one user
// conversation. Memory sessions expire after sessionTTL; when expired or
// absent the transcript is restored from the history store so 继续
// conversation from history keeps full context.
func getSession(user string, conv int64) *aiSession {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	key := sessionKey(user, conv)
	s, ok := sessions.sessions[key]
	if !ok || time.Since(s.lastActive) > sessionTTL {
		s = &aiSession{lastActive: time.Now()}
		if history != nil && conv > 0 {
			if msgs, err := listHistory(user, conv); err == nil {
				for _, m := range msgs {
					if m.Role == "user" {
						s.messages = append(s.messages, chatMessage{Role: "user", Content: m.Text})
					} else if m.Role == "assistant" && m.Text != "" {
						s.messages = append(s.messages, chatMessage{Role: "assistant", Content: m.Text})
					}
				}
			}
		}
		sessions.sessions[key] = s
	}
	s.lastActive = time.Now()
	return s
}

// reset clears one conversation's memory session (conv<=0: all of user).
func resetSession(user string, conv int64) {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	if conv <= 0 {
		for k := range sessions.sessions {
			if strings.HasPrefix(k, user+":") {
				delete(sessions.sessions, k)
			}
		}
		return
	}
	delete(sessions.sessions, sessionKey(user, conv))
}

func (s *aiSession) append(m chatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
	if len(s.messages) > maxSessionMsgs {
		s.messages = safeTruncate(s.messages, maxSessionMsgs)
	}
}

// appendUserMessage records a user turn only when it is not already the
// latest entry — prevents duplicates when a failed request is retried.
func (s *aiSession) appendUserMessage(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.messages); n > 0 && s.messages[n-1].Role == "user" && s.messages[n-1].Content == text {
		return
	}
	s.messages = append(s.messages, chatMessage{Role: "user", Content: text})
	if len(s.messages) > maxSessionMsgs {
		s.messages = safeTruncate(s.messages, maxSessionMsgs)
	}
}

// safeTruncate slides the window to keep the last n messages, but moves the
// cut-point forward so the sequence never starts with an orphan tool reply
// (a tool message whose assistant tool_calls turn was cut) — otherwise the
// provider rejects the whole request.
func safeTruncate(msgs []chatMessage, n int) []chatMessage {
	if len(msgs) <= n {
		return msgs
	}
	start := len(msgs) - n
	for start < len(msgs) {
		m := msgs[start]
		if m.Role == "tool" {
			start++
			continue
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// keep an assistant-with-tool_calls only if all its tool results follow
			need := len(m.ToolCalls)
			got := 0
			for j := start + 1; j < len(msgs) && got < need; j++ {
				if msgs[j].Role == "tool" {
					got++
				} else {
					break
				}
			}
			if got < need {
				start++
				continue
			}
		}
		break
	}
	return msgs[start:]
}

func (s *aiSession) snapshot() []chatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]chatMessage, len(s.messages))
	copy(out, s.messages)
	return out
}

// ReplaceToolResult rewrites the content of the tool message created for
// toolCallID. Used when an approved action finishes: the waiting-for-
// approval placeholder is replaced with the real output so the model can
// see what happened and follow up.
func (s *aiSession) ReplaceToolResult(toolCallID, content string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.messages) - 1; i >= 0; i-- {
		if s.messages[i].Role == "tool" && s.messages[i].ToolCallID == toolCallID {
			s.messages[i].Content = content
			return true
		}
	}
	return false
}
