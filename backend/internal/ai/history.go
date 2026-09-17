package ai

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// HistoryStore persists per-user chat transcripts (pgsql-backed), grouped
// by conversation so users can revisit past chats.
type HistoryStore interface {
	Append(user string, conv int64, role, content, cards string) (int64, error)
	UpdateCards(user string, id int64, cards string) error
	TrimConvs(user string, keep int) error
	Convs(user string) ([]ConvInfo, error)
	ListRaw(user string, limit int) ([]HistoryRow, error)
	ClearConv(user string, conv int64) error
	Clear(user string) error
}

// HistoryRow is one raw stored row; Cards is JSON of []HistoryCard.
type HistoryRow struct {
	ID      int64
	Conv    int64
	Role    string
	Content string
	Cards   string
	Time    string
}

// ConvInfo describes one stored conversation.
type ConvInfo struct {
	ConvID   int64  `json:"convId"`
	Title    string `json:"title"`
	LastTime string `json:"lastTime"`
	Msgs     int    `json:"msgs"`
}

// HistoryMsg is one chat turn returned to the frontend.
type HistoryMsg struct {
	ID    int64         `json:"id"`
	Conv  int64         `json:"conv"`
	Role  string        `json:"role"`
	Text  string        `json:"text"`
	Cards []HistoryCard `json:"cards,omitempty"`
	Time  string        `json:"time"`
}

// HistoryCard is an approval card snapshot stored with the assistant turn.
type HistoryCard struct {
	ID        string   `json:"id"`
	Tool      string   `json:"tool"`
	Command   string   `json:"command"`
	Level     string   `json:"level"`
	RiskHints []string `json:"riskHints"`
	Status    string   `json:"status"`
	Output    string   `json:"output"`
}

const (
	historyKeepMsgs  = 400 // per-conversation row cap returned to frontend
	historyKeepConvs = 20  // conversations kept per user
)

var history HistoryStore

// SetHistoryStore installs the persistence backend (called from router).
func SetHistoryStore(s HistoryStore) { history = s }

// recordMessage appends a message to the given conversation, then trims old
// conversations beyond historyKeepConvs. Failures never break the chat.
func recordMessage(user string, conv int64, role, content string, cards []HistoryCard) {
	if history == nil || conv <= 0 {
		return
	}
	cardsJSON := ""
	if len(cards) > 0 {
		b, err := json.Marshal(cards)
		if err != nil {
			log.Printf("ai history marshal cards: %v", err)
			return
		}
		cardsJSON = string(b)
	}
	if _, err := history.Append(user, conv, role, content, cardsJSON); err != nil {
		log.Printf("ai history append: %v", err)
		return
	}
	if err := history.TrimConvs(user, historyKeepConvs); err != nil {
		log.Printf("ai history trim convs: %v", err)
	}
}

// recordCardUpdate rewrites the status/output of the newest card matching
// cardID, so approve/reject outcomes survive page reloads.
func recordCardUpdate(user string, cardID, status, output string) {
	if history == nil {
		return
	}
	rows, err := history.ListRaw(user, historyKeepMsgs)
	if err != nil {
		return
	}
	for i := len(rows) - 1; i >= 0; i-- {
		var cards []HistoryCard
		if rows[i].Cards == "" || json.Unmarshal([]byte(rows[i].Cards), &cards) != nil {
			continue
		}
		hit := false
		for j := range cards {
			if cards[j].ID == cardID {
				cards[j].Status = status
				cards[j].Output = output
				hit = true
			}
		}
		if !hit {
			continue
		}
		b, _ := json.Marshal(cards)
		if err := history.UpdateCards(user, rows[i].ID, string(b)); err != nil {
			log.Printf("ai history update cards: %v", err)
		}
		return
	}
}

// listHistory loads and parses one conversation, oldest first.
func listHistory(user string, conv int64) ([]HistoryMsg, error) {
	rows, err := history.ListRaw(user, historyKeepMsgs)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryMsg, 0, len(rows))
	for _, r := range rows {
		if r.Conv != conv {
			continue
		}
		m := HistoryMsg{ID: r.ID, Conv: r.Conv, Role: r.Role, Text: r.Content, Time: r.Time}
		if r.Cards != "" {
			var cards []HistoryCard
			if json.Unmarshal([]byte(r.Cards), &cards) == nil && len(cards) > 0 {
				m.Cards = cards
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// HistoryHandler: GET /api/ai/history?conv=<id> — messages of one
// conversation; without conv returns the conversation list instead.
func HistoryHandler(c *gin.Context) {
	user := c.MustGet("username").(string)
	if history == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []ConvInfo{}})
		return
	}
	if c.Query("conv") == "" {
		convs, err := history.Convs(user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "读取会话列表失败"})
			return
		}
		if convs == nil {
			convs = []ConvInfo{}
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": convs})
		return
	}
	conv, err := strconv.ParseInt(c.Query("conv"), 10, 64)
	if err != nil || conv <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	msgs, err := listHistory(user, conv)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "读取历史失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": msgs})
}

// HistoryClearHandler: POST /api/ai/history/clear { conv? } — wipes one or
// all saved conversations.
func HistoryClearHandler(c *gin.Context) {
	user := c.MustGet("username").(string)
	if history == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
		return
	}
	var req struct {
		Conv int64 `json:"conv"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Conv > 0 {
		if err := history.ClearConv(user, req.Conv); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "删除会话失败"})
			return
		}
	} else {
		if err := history.Clear(user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "清除历史失败"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}
