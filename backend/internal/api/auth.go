package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionTTL = 7 * 24 * time.Hour

func checkCredentials(user, pass string) bool {
	return dbVerifyUser(user, pass)
}

// loginGuard implements simple brute-force throttling per IP.
var (
	failMu   sync.Mutex
	failCnts = map[string]*failState{}
)

type failState struct {
	count    int
	blockedU time.Time
	windowS  time.Time
}

func loginGuard(ip string) bool {
	failMu.Lock()
	defer failMu.Unlock()
	st, ok := failCnts[ip]
	if !ok {
		return true
	}
	if time.Now().Before(st.blockedU) {
		return false
	}
	if time.Now().After(st.windowS) {
		delete(failCnts, ip)
	}
	return true
}

func loginFail(ip string) {
	failMu.Lock()
	defer failMu.Unlock()
	st, ok := failCnts[ip]
	if !ok || time.Now().After(st.windowS) {
		failCnts[ip] = &failState{count: 1, windowS: time.Now().Add(10 * time.Minute)}
		return
	}
	st.count++
	if st.count >= 5 {
		st.blockedU = time.Now().Add(5 * time.Minute)
		st.count = 0
	}
}

func loginHandler(c *gin.Context) {
	ip := c.ClientIP()
	if !loginGuard(ip) {
		time.Sleep(1 * time.Second)
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 1, "error": "尝试次数过多，请 5 分钟后再试"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}

	if !checkCredentials(req.Username, req.Password) {
		loginFail(ip)
		auditLog(c, "auth/login", req.Username, "FAIL")
		time.Sleep(600 * time.Millisecond)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 1, "error": "用户名或密码错误"})
		return
	}

	failMu.Lock()
	delete(failCnts, ip)
	failMu.Unlock()

	tok, _, err := dbNewSession(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "会话创建失败"})
		return
	}
	auditLog(c, "auth/login", req.Username, "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"token": tok, "expiresIn": int(sessionTTL.Seconds())}})
}

func logoutHandler(c *gin.Context) {
	t := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	dbDropSession(t)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

// ── user management (admin only, stored in pgsql) ──────────────────

func changePassHandler(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	user := c.MustGet("username").(string)
	if err := dbChangePassword(user, req.OldPassword, req.NewPassword); err != nil {
		auditLog(c, "auth/change-pass", user, "FAIL")
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": err.Error()})
		return
	}
	auditLog(c, "auth/change-pass", user, "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

func adminOnly(c *gin.Context) {
	if c.MustGet("username").(string) != "admin" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 1, "error": "仅管理员可操作"})
		return
	}
	c.Next()
}

func usersListHandler(c *gin.Context) {
	users, err := dbListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": users})
}

func userCreateHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	if err := dbCreateUser(req.Username, req.Password, "admin"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": err.Error()})
		return
	}
	auditLog(c, "user/create", req.Username, "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

func userDeleteHandler(c *gin.Context) {
	name := c.Param("name")
	if err := dbDeleteUser(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": err.Error()})
		return
	}
	auditLog(c, "user/delete", name, "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

func userResetPassHandler(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": "参数错误"})
		return
	}
	name := c.Param("name")
	if err := dbUpdatePassword(name, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "error": err.Error()})
		return
	}
	auditLog(c, "user/reset-pass", name, "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

func auditListHandler(c *gin.Context) {
	items, err := dbRecentAudit(200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}
