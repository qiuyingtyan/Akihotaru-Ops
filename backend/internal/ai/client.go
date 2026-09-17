// Package ai implements the LLM-backed ops assistant: an OpenAI-compatible
// chat client, a tool registry over the panel's own capabilities plus a
// guarded shell tool, and a per-user conversation store with approval flow.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config carries the LLM endpoint settings, read once at startup.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

var cfg Config

// SetConfig installs the provider config (called from router setup).
func SetConfig(c Config) { cfg = c }

// Enabled reports whether the assistant has an API key configured.
func Enabled() bool { return cfg.APIKey != "" }

// chatMessage is one entry of the OpenAI chat array.
type chatMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatTool struct {
	Type     string             `json:"type"`
	Function chatToolDefinition `json:"function"`
}

type chatToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

var httpClient = &http.Client{Timeout: 180 * time.Second}

// chat performs one round-trip against the OpenAI-compatible endpoint.
func chat(ctx context.Context, msgs []chatMessage, tools []chatTool) (*chatResponse, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("AI 功能未配置（缺少 OPSWEB_AI_KEY）")
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.deepseek.com"
	}
	body, err := json.Marshal(chatRequest{
		Model:       cfg.Model,
		Messages:    msgs,
		Tools:       tools,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(base, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AI 接口请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败: %w", err)
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("AI 响应解析失败 (HTTP %s): %s", resp.Status, truncate(string(data), 200))
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("AI 接口报错: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return nil, fmt.Errorf("AI 未返回任何内容")
	}
	return &cr, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
