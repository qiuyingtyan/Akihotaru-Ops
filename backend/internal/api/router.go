package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"opsweb/internal/ai"
	"opsweb/internal/collect"
	"opsweb/internal/web"
)

// Version is injected via ldflags at build time.
var Version = "dev"

// SessionToken is the credential issued by /api/login and accepted
// in place of a static token for all API calls.
var SessionToken = ""

var auditMu sync.Mutex

// auditLog appends an operation record to local file and pgsql.
func auditLog(c *gin.Context, target, action, result string) {
	auditMu.Lock()
	defer auditMu.Unlock()
	if f, err := os.OpenFile("/workspace/opsweb/audit.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
		fmt.Fprintf(f, "%s ip=%s %s %s result=%s\n",
			time.Now().Format("2006-01-02 15:04:05"),
			c.ClientIP(), target, action, result)
		f.Close()
	}
	log.Printf("AUDIT ip=%s %s %s result=%s", c.ClientIP(), target, action, result)
	if db != nil {
		dbAuditLog(c.ClientIP(), target, action, result)
	}
}

// actionHandler wraps a mutating operation with audit logging.
func actionHandler(kind string, h func(c *gin.Context) (string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := h(c)
		if err != nil {
			auditLog(c, kind+"/"+c.Param("name"), c.Param("action"), "FAIL "+err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": err.Error()})
			return
		}
		auditLog(c, kind+"/"+c.Param("name"), c.Param("action"), "OK")
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
	}
}

// sessionAuth extracts the session token from either the
// Authorization header or a token query param (SSE EventSource can't set headers).
func sessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		t := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if t == "" {
			t = c.Query("token")
		}
		user, ok := dbSessionUser(t)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Set("username", user)
		c.Next()
	}
}

// NewRouter builds the gin engine with auth and all routes.
func NewRouter() *gin.Engine {
	collect.SetMetricsDB(db)
	collect.StartSampler(MetricsIngest)
	collect.StartAlertChecker()

	// AI assistant: OpenAI-compatible endpoint via env config + audit sink
	ai.SetConfig(ai.Config{
		APIKey:  os.Getenv("OPSWEB_AI_KEY"),
		BaseURL: os.Getenv("OPSWEB_AI_BASE_URL"),
		Model:   os.Getenv("OPSWEB_AI_MODEL"),
	})
	ai.SetAudit(func(ip, user, target, action, result string) {
		auditMu.Lock()
		defer auditMu.Unlock()
		if f, err := os.OpenFile("/workspace/opsweb/audit.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
			fmt.Fprintf(f, "%s ip=%s user=%s %s %s result=%s\n",
				time.Now().Format("2006-01-02 15:04:05"), ip, user, target, action, result)
			f.Close()
		}
		log.Printf("AUDIT ip=%s user=%s %s %s result=%s", ip, user, target, action, result)
		if db != nil {
			dbAuditLog(ip, target, action, result+" user="+user)
		}
	})
	ai.SetSettingsStore(pgSettings{})
	ai.SetHistoryStore(pgHistory{})
	ai.LoadSettings()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// session auth via Authorization header or ?token= (SSE)
	apiGroup := r.Group("/api")
	apiGroup.Use(sessionAuth())

	// login is the only unauthenticated endpoint
	r.POST("/api/login", loginHandler)
	// health endpoint for CI verify / monitoring (no auth)
	r.GET("/api/health", func(c *gin.Context) {
		dbOK := DBHealthy()
		code := http.StatusOK
		if !dbOK {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{"code": 0, "data": gin.H{"status": "ok", "db": dbOK}})
	})

	apiGroup.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"msg": "pong"}) })
	apiGroup.POST("/logout", logoutHandler)
	apiGroup.GET("/version", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": 0, "data": gin.H{"version": Version}})
	})

	// system overview
	apiGroup.GET("/overview", collect.CachedOverviewHandler)
	apiGroup.GET("/cpu", collect.CPUHandler)
	apiGroup.GET("/mem", collect.MemHandler)
	apiGroup.GET("/disk", collect.DiskHandler)
	apiGroup.GET("/net", collect.NetHandler)
	apiGroup.GET("/load/history", collect.LoadHistoryHandler)

	// docker
	apiGroup.GET("/docker/containers", collect.ContainersHandler)
	apiGroup.POST("/docker/containers/:name/:action",
		actionHandler("docker", collect.ContainerAction))
	apiGroup.GET("/docker/images", collect.ImagesHandler)
	apiGroup.GET("/docker/container/:name/logs", collect.ContainerLogsHandler)

	// projects (compose stacks / workspace apps)
	apiGroup.GET("/projects", collect.ProjectsHandler)
	apiGroup.GET("/projects/:name/deploy-status", collect.DeployStatusHandler)
	apiGroup.POST("/projects/:name/:action",
		actionHandler("project", collect.ProjectAction))

	// CI/CD
	apiGroup.GET("/cicd/summary", collect.CICDSummaryHandler)
	apiGroup.GET("/cicd/pipelines", collect.PipelinesHandler)
	apiGroup.GET("/cicd/runner/logs", collect.RunnerLogsHandler)

	// systemd services
	apiGroup.GET("/services", collect.ServicesHandler)
	apiGroup.POST("/services/:name/:action",
		actionHandler("service", collect.ServiceAction))

	// alerts
	apiGroup.GET("/alerts", collect.AlertsHandler)

	// process list
	apiGroup.GET("/processes", collect.ProcessesHandler)

	// generic log tail + journal + directory browser + live follow (SSE)
	apiGroup.GET("/logs/file", collect.LogFileHandler)
	apiGroup.GET("/logs/list", collect.LogListHandler)
	apiGroup.GET("/logs/journal", collect.JournalHandler)
	apiGroup.GET("/logs/follow", collect.LogFollowHandler)
	apiGroup.GET("/logs/journal/follow", collect.JournalFollowHandler)

	// account management (pgsql-backed)
	apiGroup.POST("/account/password", changePassHandler)
	apiGroup.GET("/audit", auditListHandler)

	// AI assistant (all logged-in users; safety flow inside)
	apiGroup.GET("/ai/status", ai.StatusHandler)
	apiGroup.POST("/ai/chat", ai.ChatHandler)
	apiGroup.POST("/ai/approve", ai.ApprovedHandler)
	apiGroup.POST("/ai/reject", ai.RejectHandler)
	apiGroup.POST("/ai/reset", ai.ResetHandler)
	apiGroup.GET("/ai/settings", ai.SettingsHandler)
	apiGroup.POST("/ai/settings", ai.SettingsSaveHandler)
	apiGroup.POST("/ai/settings/test", ai.SettingsTestHandler)
	apiGroup.GET("/ai/history", ai.HistoryHandler)
	apiGroup.POST("/ai/history/clear", ai.HistoryClearHandler)
	usersGroup := apiGroup.Group("/users", adminOnly)
	usersGroup.GET("", usersListHandler)
	usersGroup.POST("", userCreateHandler)
	usersGroup.DELETE("/:name", userDeleteHandler)
	usersGroup.POST("/:name/password", userResetPassHandler)

	web.RegisterStatic(r)
	return r
}

// pgSettings adapts the pgsql-backed settings table to ai.SettingsStore.
type pgSettings struct{}

func (pgSettings) Get(key string) (string, error)      { return dbGetSetting(key) }
func (pgSettings) Set(key, value string) error         { return dbSetSetting(key, value) }
func (pgSettings) Delete(key string) error             { return dbDeleteSetting(key) }

// pgHistory adapts the pgsql-backed chat history table to ai.HistoryStore.
type pgHistory struct{}

func (pgHistory) Append(user string, conv int64, role, content, cards string) (int64, error) {
	return dbHistAppend(user, conv, role, content, cards)
}

func (pgHistory) UpdateCards(user string, id int64, cards string) error {
	return dbHistUpdateCards(user, id, cards)
}

func (pgHistory) TrimConvs(user string, keep int) error { return dbHistTrimConvs(user, keep) }

func (pgHistory) Convs(user string) ([]ai.ConvInfo, error) {
	rows, err := dbHistConvs(user)
	if err != nil {
		return nil, err
	}
	out := make([]ai.ConvInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, ai.ConvInfo{
			ConvID: r["convId"].(int64), Title: r["title"].(string),
			LastTime: r["lastTime"].(string), Msgs: int(r["msgs"].(int64)),
		})
	}
	return out, nil
}

func (pgHistory) ClearConv(user string, conv int64) error { return dbHistClearConv(user, conv) }

func (pgHistory) ListRaw(user string, limit int) ([]ai.HistoryRow, error) {
	rows, err := dbHistList(user, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ai.HistoryRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ai.HistoryRow{
			ID: r["id"].(int64), Conv: r["convId"].(int64), Role: r["role"].(string),
			Content: r["content"].(string), Cards: r["cards"].(string), Time: r["time"].(string),
		})
	}
	return out, nil
}

func (pgHistory) Clear(user string) error { return dbHistClear(user) }
