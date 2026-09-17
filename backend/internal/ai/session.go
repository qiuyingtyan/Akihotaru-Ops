package ai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// pendingAction is a write/shell tool call awaiting user approval.
type pendingAction struct {
	ID        string    `json:"id"`
	Tool      string    `json:"tool"`
	Args      string    `json:"args"`
	Command   string    `json:"command,omitempty"`
	Level     string    `json:"level"`
	RiskHints []string  `json:"riskHints,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
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

// newPending registers an approval request owned by user.
func newPending(user, tool, argsJSON, command string, level safetyLevel, hints []string) pendingAction {
	id := randID()
	now := time.Now()
	a := pendingAction{
		ID:        id,
		Tool:      tool,
		Args:      argsJSON,
		Command:   command,
		Level:     level.String(),
		RiskHints: hints,
		CreatedAt: now,
		ExpiresAt: now.Add(pendingTTL),
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

// ── chat sessions (per user, in memory, sliding TTL) ────────────────

const (
	sessionTTL     = 30 * time.Minute
	maxTurns       = 40 // messages kept per session (user+assistant+tool)
	maxSessionMsgs = 80
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

// getSession returns (creating if needed) the user's chat session.
func getSession(user string) *aiSession {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	s, ok := sessions.sessions[user]
	if !ok || time.Since(s.lastActive) > sessionTTL {
		s = &aiSession{lastActive: time.Now()}
		sessions.sessions[user] = s
	}
	s.lastActive = time.Now()
	return s
}

// reset clears the user's conversation.
func resetSession(user string) {
	sessions.mu.Lock()
	delete(sessions.sessions, user)
	sessions.mu.Unlock()
}

func (s *aiSession) append(m chatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
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
