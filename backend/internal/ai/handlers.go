package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// SettingsStore persists provider settings (pgsql-backed, key-value).
type SettingsStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

var settings SettingsStore

// SetSettingsStore installs the persistence backend (called from router).
func SetSettingsStore(s SettingsStore) { settings = s }

// LoadSettings overlays persisted settings onto the runtime config.
// Call once at startup after SetSettingsStore.
func LoadSettings() { loadSettings() }

const (
	setKeyAPIKey  = "ai_api_key"
	setKeyBaseURL = "ai_base_url"
	setKeyModel   = "ai_model"
)

// loadSettings overlays persisted settings on top of the env-derived config.
func loadSettings() {
	if settings == nil {
		return
	}
	c := GetConfig()
	keyVal, keyErr := settings.Get(setKeyAPIKey)
	if keyErr == nil && keyVal != "" {
		c.APIKey = keyVal
		if v, err := settings.Get(setKeyBaseURL); err == nil {
			c.BaseURL = v
		}
		if v, err := settings.Get(setKeyModel); err == nil {
			c.Model = v
		}
	}
	SetConfig(c)
}

// saveSettings persists the non-empty provider fields; empty fields mean
// "fall back to env/default" and delete the stored row.
func saveSettings(c Config) error {
	if settings == nil {
		return fmt.Errorf("设置存储不可用")
	}
	if err := settings.Set(setKeyAPIKey, c.APIKey); err != nil {
		return err
	}
	if c.BaseURL == "" {
		settings.Delete(setKeyBaseURL)
	} else if err := settings.Set(setKeyBaseURL, c.BaseURL); err != nil {
		return err
	}
	if c.Model == "" {
		settings.Delete(setKeyModel)
	} else if err := settings.Set(setKeyModel, c.Model); err != nil {
		return err
	}
	return nil
}

// SettingsHandler: GET /api/ai/settings — admin view of current provider
// config; the key is masked as sk-***last4.
func SettingsHandler(c *gin.Context) {
	if c.MustGet("username").(string) != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 1, "error": "仅管理员可查看 AI 设置"})
		return
	}
	c2 := GetConfig()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"enabled":   c2.APIKey != "",
		"keyMasked": maskKey(c2.APIKey),
		"hasKey":    c2.APIKey != "",
		"baseUrl":   c2.BaseURL,
		"model":     c2.Model,
	}})
}

// SettingsSaveHandler: POST /api/ai/settings {apiKey?, baseUrl?, model?}.
// Empty apiKey keeps the existing one; "-" clears the stored key.
func SettingsSaveHandler(c *gin.Context) {
	if c.MustGet("username").(string) != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 1, "error": "仅管理员可修改 AI 设置"})
		return
	}
	var req struct {
		APIKey  string `json:"apiKey"`
		BaseURL string `json:"baseUrl"`
		Model   string `json:"model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	cur := GetConfig()
	next := cur
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.APIKey == "-" {
		next.APIKey = ""
	} else if req.APIKey != "" {
		next.APIKey = req.APIKey
	}
	next.BaseURL = strings.TrimSpace(req.BaseURL)
	next.Model = strings.TrimSpace(req.Model)
	if next.APIKey == "" {
		SetConfig(next)
		if settings != nil {
			settings.Delete(setKeyAPIKey)
			settings.Delete(setKeyBaseURL)
			settings.Delete(setKeyModel)
		}
		auditf(c, "ai/settings", "clear", "OK")
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"enabled": false}})
		return
	}
	if err := saveSettings(next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "保存设置失败: " + err.Error()})
		return
	}
	SetConfig(next)
	auditf(c, "ai/settings", "update model="+next.Model+" base="+next.BaseURL, "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"enabled": true, "model": next.Model, "baseUrl": next.BaseURL}})
}

// SettingsTestHandler: POST /api/ai/settings/test {apiKey?, baseUrl?, model?}
// — probes the endpoint with the given or current settings.
func SettingsTestHandler(c *gin.Context) {
	if c.MustGet("username").(string) != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 1, "error": "仅管理员可测试 AI 连接"})
		return
	}
	var req struct {
		APIKey  string `json:"apiKey"`
		BaseURL string `json:"baseUrl"`
		Model   string `json:"model"`
	}
	_ = c.ShouldBindJSON(&req)
	override := Config{
		APIKey:  strings.TrimSpace(req.APIKey),
		BaseURL: strings.TrimSpace(req.BaseURL),
		Model:   strings.TrimSpace(req.Model),
	}
	if err := TestConnectivity(override); err != nil {
		auditf(c, "ai/settings/test", "probe", "FAIL "+err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": err.Error()})
		return
	}
	auditf(c, "ai/settings/test", "probe", "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "***"
	}
	return k[:5] + "****" + k[len(k)-4:]
}

// AuditSink lets the ai package write to the panel's audit trail without
// importing internal/api (avoids an import cycle).
type AuditSink func(ip, user, target, action, result string)

var audit AuditSink

// SetAudit installs the audit sink (called from router setup).
func SetAudit(fn AuditSink) { audit = fn }

func auditf(c *gin.Context, target, action, result string) {
	if audit != nil {
		user := ""
		if v, ok := c.Get("username"); ok {
			user, _ = v.(string)
		}
		audit(c.ClientIP(), user, target, action, result)
	}
}

// systemPrompt anchors the assistant's identity and safety contract.
const systemPrompt = `你是 pf3090 服务器运维面板的 AI 运维助手，帮助用户查看和管理这台 Linux 服务器（运行 Docker 容器与 systemd 服务，承载多个业务项目）。

行为准则：
1. 回答使用简体中文，简洁专业，关键数值用加粗。
2. 优先使用提供的工具获取真实数据，不要凭空猜测服务器状态；工具结果就是事实，如实转述。
3. 需要执行操作时选择最合适的工具；容器/服务/部署类操作会弹给用户确认，shell 危险命令需要用户批准，你无法绕过确认。
4. 当你的 shell 命令被标记为需批准时，等用户批准后结果会自动返回，不要重复调用。
5. 对危险命令（删除、重启、停止服务、批量操作等）在调用前先向用户说明影响范围。
6. 只回答与这台服务器运维相关的问题，其他话题礼貌拒绝。
7. 工具返回的错误信息要如实告知用户，并给出排查建议。`

// ChatRequest is the /ai/chat payload.
type ChatRequest struct {
	Message string `json:"message"`
}

// chatLoop runs at most maxToolRounds of tool-calling, returning the final
// assistant text plus any pending approvals created along the way.
func chatLoop(c *gin.Context, user, userMsg string) (string, []pendingAction, error) {
	s := getSession(user)
	s.append(chatMessage{Role: "user", Content: userMsg})

	msgs := append([]chatMessage{{Role: "system", Content: systemPrompt}}, s.snapshot()...)
	tools := toolDefs()
	var created []pendingAction
	toolRound := 0

	for toolRound < 8 {
		resp, err := chat(c.Request.Context(), msgs, tools)
		if err != nil {
			return "", created, err
		}
		msg := resp.Choices[0].Message
		if len(msg.ToolCalls) == 0 {
			s.append(chatMessage{Role: "assistant", Content: msg.Content})
			return msg.Content, created, nil
		}

		// persist assistant tool-call turn into session
		s.append(chatMessage{Role: "assistant", ToolCalls: msg.ToolCalls, Content: msg.Content})
		msgs = append(msgs, msg)

		for _, tc := range msg.ToolCalls {
			result, pending := executeToolCall(c, user, tc)
			if pending != nil {
				created = append(created, *pending)
			}
			msgs = append(msgs, chatMessage{
				Role: "tool", Content: result, ToolCallID: tc.ID, Name: tc.Function.Name,
			})
			s.append(chatMessage{
				Role: "tool", Content: result, ToolCallID: tc.ID, Name: tc.Function.Name,
			})
		}
		toolRound++
	}
	return "（已达单轮工具调用上限，请继续提问）", created, nil
}

// executeToolCall runs one tool call honoring the safety flow. When the
// call needs approval it registers a pending request and returns nil.
func executeToolCall(c *gin.Context, user string, tc chatToolCall) (result string, pending *pendingAction) {
	name := tc.Function.Name
	argsRaw := strings.TrimSpace(tc.Function.Arguments)

	spec, ok := toolRegistry[name]
	if !ok {
		auditf(c, "ai/"+name, "call", "FAIL unknown tool")
		return fmt.Sprintf("错误: 未知工具 %s", name), nil
	}

	var args map[string]any
	if argsRaw != "" {
		if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
			return fmt.Sprintf("错误: 参数解析失败 %v", err), nil
		}
	}

	switch spec.Level {
	case levelRead:
		out, err := spec.Exec(args)
		if err != nil {
			auditf(c, "ai/"+name, truncate(argsRaw, 80), "FAIL "+err.Error())
			return "错误: " + err.Error(), nil
		}
		auditf(c, "ai/"+name, truncate(argsRaw, 80), "OK")
		return out, nil

	case levelWrite:
		display := describeToolCall(name, args)
		pa := newPending(user, name, argsRaw, display, levelWrite, nil)
		auditf(c, "ai/"+name, truncate(argsRaw, 80), "PENDING approval")
		return fmt.Sprintf("该操作需要用户批准。已创建批准请求 id=%s，请告知用户在界面上确认，等待结果返回。", pa.ID), &pa

	default: // levelShell
		cmd, _ := args["command"].(string)
		level, blocked, hints := classifyShell(cmd)
		if blocked != "" {
			auditf(c, "ai/shell", truncate(cmd, 120), "BLOCKED "+blocked)
			return fmt.Sprintf("命令已被安全策略硬拒绝（%s）。请向用户说明原因，不要尝试变体绕过。", blocked), nil
		}
		if err := validShellBinary(cmd); err != nil {
			return "错误: " + err.Error(), nil
		}
		pa := newPending(user, "run_shell", argsRaw, cmd, level, hints)
		auditf(c, "ai/shell", truncate(cmd, 120), "PENDING approval level="+level.String())
		return fmt.Sprintf("已创建批准请求 id=%s（风险点: %s）。请告知用户在界面上确认，等待结果返回。", pa.ID, strings.Join(hints, "、")), &pa
	}
}

// describeToolCall renders a human-readable one-liner for approve cards.
func describeToolCall(name string, args map[string]any) string {
	switch name {
	case "container_action":
		return fmt.Sprintf("docker %s %s", argStr(args, "action"), argStr(args, "name"))
	case "service_action":
		return fmt.Sprintf("systemctl %s %s", argStr(args, "action"), argStr(args, "name"))
	case "deploy_project":
		return fmt.Sprintf("重新部署项目「%s」", argStr(args, "name"))
	}
	b, _ := json.Marshal(args)
	return name + " " + string(b)
}

// ── handlers ────────────────────────────────────────────────────────

// StatusHandler reports whether the assistant is configured.
func StatusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"enabled": Enabled(), "model": cfg.Model}})
}

// ChatHandler: POST /api/ai/chat { message }.
func ChatHandler(c *gin.Context) {
	if !Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 1, "error": "AI 功能未配置"})
		return
	}
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	user := c.MustGet("username").(string)
	auditf(c, "ai/chat", truncate(req.Message, 100), "ASK")

	reply, pendings, err := chatLoop(c, user, req.Message)
	if err != nil {
		auditf(c, "ai/chat", truncate(req.Message, 100), "FAIL "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": err.Error()})
		return
	}
	recordMessage(user, "user", req.Message, nil)
	var histCards []HistoryCard
	for _, pa := range pendings {
		histCards = append(histCards, HistoryCard{
			ID: pa.ID, Tool: pa.Tool, Command: pa.Command, Level: pa.Level,
			RiskHints: pa.RiskHints, Status: "",
		})
	}
	recordMessage(user, "assistant", reply, histCards)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"reply": reply, "pending": pendings}})
}

// ApprovedHandler: POST /api/ai/approve { id } — runs the approved action.
func ApprovedHandler(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	user := c.MustGet("username").(string)
	pa, err := takePending(user, req.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": err.Error()})
		return
	}

	spec, ok := toolRegistry[pa.Tool]
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "工具已不存在"})
		return
	}
	var args map[string]any
	if pa.Args != "" {
		json.Unmarshal([]byte(pa.Args), &args)
	}

	// run with a hard timeout; shell timeouts are shape-based
	done := make(chan struct{})
	var out string
	var execErr error
	go func() {
		defer close(done)
		out, execErr = spec.Exec(args)
	}()
	select {
	case <-done:
	case <-time.After(execTimeoutFor(pa)):
		auditf(c, "ai/"+pa.Tool, truncate(pa.Command, 120), "FAIL timeout")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "执行超时"})
		return
	}

	result := out
	if execErr != nil {
		result = out + "\n[错误] " + execErr.Error()
	}
	auditf(c, "ai/"+pa.Tool, truncate(pa.Command, 120), "APPROVED-EXEC "+bool2str(execErr == nil))
	recordCardUpdate(user, pa.ID, "已执行", truncate(result, 12000))

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"tool":    pa.Tool,
		"command": pa.Command,
		"output":  truncate(result, 12000),
		"ok":      execErr == nil,
	}})
}

// RejectHandler: POST /api/ai/reject { id }.
func RejectHandler(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	user := c.MustGet("username").(string)
	pa, err := takePending(user, req.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": err.Error()})
		return
	}
	auditf(c, "ai/"+pa.Tool, truncate(pa.Command, 120), "REJECTED")
	recordCardUpdate(user, pa.ID, "已拒绝", "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

// ResetHandler: POST /api/ai/reset — clear the user's conversation.
func ResetHandler(c *gin.Context) {
	user := c.MustGet("username").(string)
	resetSession(user)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

// execTimeoutFor returns the approval-execution hard timeout for a pending

func execTimeoutFor(pa pendingAction) time.Duration {
	if pa.Tool == "run_shell" {
		var args map[string]any
		json.Unmarshal([]byte(pa.Args), &args)
		if cmd, _ := args["command"].(string); cmd != "" {
			return shellExecTimeout(cmd) + 30*time.Second
		}
	}
	if pa.Tool == "deploy_project" {
		return 11 * time.Minute
	}
	return 2 * time.Minute
}

func bool2str(b bool) string {
	if b {
		return "OK"
	}
	return "FAIL"
}
