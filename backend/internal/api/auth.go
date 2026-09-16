package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionTTL = 7 * 24 * time.Hour

var (
	authUser string
	authPass string

	sessMu   sync.Mutex
	sessions = map[string]time.Time{}

	failMu   sync.Mutex
	failCnts = map[string]*failState{}
)

type failState struct {
	count    int
	blockedU time.Time
	windowS  time.Time
}

func initAuth(user, pass string) {
	authUser, authPass = user, pass
}

func newSession() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)
	sessMu.Lock()
	sessions[t] = time.Now().Add(sessionTTL)
	sessMu.Unlock()
	return t, nil
}

func validSession(t string) bool {
	if t == "" {
		return false
	}
	sessMu.Lock()
	defer sessMu.Unlock()
	exp, ok := sessions[t]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(sessions, t)
		return false
	}
	return true
}

func dropSession(t string) {
	sessMu.Lock()
	delete(sessions, t)
	sessMu.Unlock()
}

func checkCredentials(user, pass string) bool {
	u := subtle.ConstantTimeCompare([]byte(user), []byte(authUser)) == 1
	p := subtle.ConstantTimeCompare([]byte(pass), []byte(authPass)) == 1
	return u && p
}

// loginGuard implements simple brute-force throttling per IP.
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

	tok, err := newSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "error": "会话创建失败"})
		return
	}
	auditLog(c, "auth/login", req.Username, "OK")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"token": tok, "expiresIn": int(sessionTTL.Seconds())}})
}

func logoutHandler(c *gin.Context) {
	t := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	dropSession(t)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}
