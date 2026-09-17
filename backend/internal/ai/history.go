package ai

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HistoryStore persists per-user chat transcripts (pgsql-backed).
type HistoryStore interface {
	Append(user, role, content, cards string) (int64, error)
	UpdateCards(user string, id int64, cards string) error
	Trim(user string, keep int) error
	ListRaw(user string, limit int) ([]HistoryRow, error)
	Clear(user string) error
}

// HistoryRow is one raw stored row; Cards is JSON of []HistoryCard.
type HistoryRow struct {
	ID      int64
	Role    string
	Content string
	Cards   string
	Time    string
}

// HistoryMsg is one chat turn returned to the frontend.
type HistoryMsg struct {
	ID    int64         `json:"id"`
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

const historyKeep = 200

var history HistoryStore

// SetHistoryStore installs the persistence backend (called from router).
func SetHistoryStore(s HistoryStore) { history = s }

// recordMessage appends a message to the user's transcript, keeping at most
// historyKeep rows per user. Failures are logged but never break the chat.
func recordMessage(user, role, content string, cards []HistoryCard) {
	if history == nil {
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
	if _, err := history.Append(user, role, content, cardsJSON); err != nil {
		log.Printf("ai history append: %v", err)
		return
	}
	if err := history.Trim(user, historyKeep); err != nil {
		log.Printf("ai history trim: %v", err)
	}
}

// recordCardUpdate rewrites the status/output of the newest card matching
// cardID, so approve/reject outcomes survive page reloads.
func recordCardUpdate(user, cardID, status, output string) {
	if history == nil {
		return
	}
	rows, err := history.ListRaw(user, historyKeep)
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

// listHistory loads and parses the user's transcript, oldest first.
func listHistory(user string, limit int) ([]HistoryMsg, error) {
	rows, err := history.ListRaw(user, limit)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryMsg, 0, len(rows))
	for _, r := range rows {
		m := HistoryMsg{ID: r.ID, Role: r.Role, Text: r.Content, Time: r.Time}
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

// HistoryHandler: GET /api/ai/history — the user's transcript, oldest first.
func HistoryHandler(c *gin.Context) {
	user := c.MustGet("username").(string)
	if history == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []HistoryMsg{}})
		return
	}
	msgs, err := listHistory(user, historyKeep)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "读取历史失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": msgs})
}

// HistoryClearHandler: POST /api/ai/history/clear — wipes the user's saved
// transcript (used together with 新对话).
func HistoryClearHandler(c *gin.Context) {
	user := c.MustGet("username").(string)
	if history == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
		return
	}
	if err := history.Clear(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "清除历史失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}
