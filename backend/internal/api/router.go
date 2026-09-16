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

	"opsweb/internal/collect"
	"opsweb/internal/web"
)

// Version is injected via ldflags at build time.
var Version = "dev"

// SessionToken is the credential issued by /api/login and accepted
// in place of a static token for all API calls.
var SessionToken = ""

var auditMu sync.Mutex

// auditLog appends an operation record to a local audit file.
func auditLog(c *gin.Context, target, action, result string) {
	auditMu.Lock()
	defer auditMu.Unlock()
	f, err := os.OpenFile("/workspace/opsweb/audit.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	line := fmt.Sprintf("%s ip=%s %s %s result=%s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		c.ClientIP(), target, action, result)
	f.WriteString(line)
	log.Printf("AUDIT ip=%s %s %s result=%s", c.ClientIP(), target, action, result)
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

// NewRouter builds the gin engine with auth and all routes.
func NewRouter(user, pass string) *gin.Engine {
	collect.StartSampler()
	collect.StartAlertChecker()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	initAuth(user, pass)

	// session auth via Authorization header
	apiGroup := r.Group("/api")
	apiGroup.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		auth := c.GetHeader("Authorization")
		t := strings.TrimPrefix(auth, "Bearer ")
		if t == "" || !validSession(t) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	})

	// login is the only unauthenticated endpoint
	r.POST("/api/login", loginHandler)

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

	// generic log tail
	apiGroup.GET("/logs/file", collect.LogFileHandler)

	web.RegisterStatic(r)
	return r
}
