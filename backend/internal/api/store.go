package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

var (
	db *sql.DB

	errUnauthorized = errors.New("原密码错误")
	errWeakPass     = errors.New("新密码至少 6 位")
)

type userRow struct {
	id           int64
	username     string
	passwordHash string
	role         string
}

// InitStore opens the pgsql connection and ensures schema + seed admin.
// dsn example: postgres://opsweb:pw@127.0.0.1:5433/opsweb
func InitStore(dsn string, seedUser, seedPass string) error {
	d, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	d.SetMaxOpenConns(4)
	d.SetMaxIdleConns(2)
	d.SetConnMaxLifetime(30 * time.Minute)
	// pgsql-baq 容器可能比本服务晚就绪，重试一段时间而不是直接退出
	var pingErr error
	for i := 0; i < 30; i++ {
		pingErr = d.Ping()
		if pingErr == nil {
			break
		}
		log.Printf("pgsql not ready (%d/30): %v", i+1, pingErr)
		time.Sleep(2 * time.Second)
	}
	if pingErr != nil {
		d.Close()
		return fmt.Errorf("pgsql unreachable after retries: %w", pingErr)
	}
	db = d

	const schema = `
CREATE TABLE IF NOT EXISTS ops_users (
	id          SERIAL PRIMARY KEY,
	username    TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role        TEXT NOT NULL DEFAULT 'admin',
	created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS ops_audit (
	id         BIGSERIAL PRIMARY KEY,
	time       TIMESTAMPTZ NOT NULL DEFAULT now(),
	ip         TEXT NOT NULL,
	target     TEXT NOT NULL,
	action     TEXT NOT NULL,
	result     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ops_audit_time ON ops_audit (time DESC);
CREATE TABLE IF NOT EXISTS ops_sessions (
	token      TEXT PRIMARY KEY,
	username   TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ops_sessions_exp ON ops_sessions (expires_at);
CREATE TABLE IF NOT EXISTS ops_metrics (
	time   TIMESTAMPTZ NOT NULL,
	metric TEXT NOT NULL,
	value  DOUBLE PRECISION NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ops_metrics_time ON ops_metrics (time DESC);
CREATE TABLE IF NOT EXISTS ops_settings (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS ops_chat_history (
	id      BIGSERIAL PRIMARY KEY,
	username TEXT NOT NULL,
	conv_id  BIGINT NOT NULL DEFAULT 0,
	role     TEXT NOT NULL,
	content  TEXT NOT NULL,
	cards    TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ops_chat_history_user ON ops_chat_history (username, id);
ALTER TABLE ops_chat_history ADD COLUMN IF NOT EXISTS conv_id BIGINT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_ops_chat_history_conv ON ops_chat_history (username, conv_id, id);
CREATE TABLE IF NOT EXISTS ops_app_services (
	id            BIGSERIAL PRIMARY KEY,
	name          TEXT NOT NULL UNIQUE,
	display_name  TEXT NOT NULL,
	group_name    TEXT NOT NULL DEFAULT 'default',
	level         INT NOT NULL DEFAULT 3,
	work_dir      TEXT NOT NULL,
	exec_start    TEXT NOT NULL,
	exec_stop     TEXT NOT NULL DEFAULT '',
	after_deps    TEXT NOT NULL DEFAULT '',
	port          INT NOT NULL DEFAULT 0,
	log_path      TEXT NOT NULL DEFAULT '',
	restart_policy TEXT NOT NULL DEFAULT 'always',
	service_type  TEXT NOT NULL DEFAULT '',
	auto_start    BOOLEAN NOT NULL DEFAULT true,
	created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ops_app_services_level ON ops_app_services (level, id);
ALTER TABLE ops_app_services ADD COLUMN IF NOT EXISTS service_type TEXT NOT NULL DEFAULT '';`
	if _, err = db.Exec(schema); err != nil {
		return err
	}

	// seed default admin when no user exists
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM ops_users`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		h, herr := bcrypt.GenerateFromPassword([]byte(seedPass), bcrypt.DefaultCost)
		if herr != nil {
			return herr
		}
		if _, ierr := db.Exec(
			`INSERT INTO ops_users (username, password_hash, role) VALUES ($1, $2, 'admin')`,
			seedUser, string(h)); ierr != nil {
			return ierr
		}
		log.Printf("seeded initial user %q into pgsql", seedUser)
	}
	go cleanupLoop()
	go metricsStoreLoop()
	return nil
}

// ── metrics (cpu/mem history in pgsql, replaces file persistence) ──

type metricPoint struct {
	T      int64
	V      float64
	Metric string
}

var metricCh = make(chan metricPoint, 256)

// MetricsIngest queues a sampled point for async pgsql storage.
func MetricsIngest(t int64, name string, v float64) {
	select {
	case metricCh <- metricPoint{T: t, V: v, Metric: name}:
	default:
		// channel full: drop oldest samples rather than block the sampler
		select {
		case <-metricCh:
		default:
		}
	}
}

func metricsStoreLoop() {
	batch := make([]metricPoint, 0, 64)
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case p := <-metricCh:
			batch = append(batch, p)
			if len(batch) >= 64 {
				flushMetrics(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				flushMetrics(batch)
				batch = batch[:0]
			}
		}
	}
}

func flushMetrics(batch []metricPoint) {
	for _, p := range batch {
		db.Exec(`INSERT INTO ops_metrics (time, metric, value) VALUES (to_timestamp($1), $2, $3)`,
			float64(p.T), p.Metric, p.V)
	}
}

func cleanupLoop() {
	for range time.Tick(1 * time.Hour) {
		db.Exec(`DELETE FROM ops_sessions WHERE expires_at < now()`)
		db.Exec(`DELETE FROM ops_audit WHERE time < now() - interval '90 days'`)
		db.Exec(`DELETE FROM ops_metrics WHERE time < now() - interval '90 days'`)
		memSessCleanup()
	}
}

func closeStore() {
	if db != nil {
		db.Close()
	}
}

// ── user management ────────────────────────────────────────────────

func dbGetUser(username string) (*userRow, error) {
	row := db.QueryRow(`SELECT id, username, password_hash, role FROM ops_users WHERE username = $1`, username)
	var u userRow
	if err := row.Scan(&u.id, &u.username, &u.passwordHash, &u.role); err != nil {
		return nil, err
	}
	return &u, nil
}

func dbVerifyUser(username, password string) bool {
	u, err := dbGetUser(username)
	if err != nil {
		// burn comparable time even for unknown users
		bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0X0PYmVlSkC0kBpN0VzCXY1vV1S"), []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.passwordHash), []byte(password)) == nil
}

func dbChangePassword(username, oldPass, newPass string) error {
	if !dbVerifyUser(username, oldPass) {
		return errUnauthorized
	}
	if len(newPass) < 6 {
		return errWeakPass
	}
	return dbUpdatePassword(username, newPass)
}

func dbUpdatePassword(username, newPass string) error {
	h, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE ops_users SET password_hash = $2, updated_at = now() WHERE username = $1`, username, string(h))
	return err
}

func dbCreateUser(username, password, role string) error {
	if len(password) < 6 {
		return errWeakPass
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO ops_users (username, password_hash, role) VALUES ($1, $2, $3)`,
		strings.TrimSpace(username), string(h), role)
	return err
}

func dbDeleteUser(username string) error {
	_, err := db.Exec(`DELETE FROM ops_users WHERE username = $1 AND username <> 'admin'`, username)
	return err
}

func dbListUsers() ([]gin.H, error) {
	rows, err := db.Query(`SELECT username, role, to_char(created_at, 'YYYY-MM-DD HH24:MI') FROM ops_users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var un, role, created string
		if err := rows.Scan(&un, &role, &created); err != nil {
			return nil, err
		}
		out = append(out, gin.H{"username": un, "role": role, "createdAt": created})
	}
	return out, rows.Err()
}

// ── sessions (persisted in pgsql so restarts don't log users out) ──

var sessMu sync.Mutex

func dbNewSession(username string) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	tok := hex.EncodeToString(b)
	exp := time.Now().Add(sessionTTL)
	if _, err := db.Exec(
		`INSERT INTO ops_sessions (token, username, expires_at) VALUES ($1, $2, $3)`,
		tok, username, exp); err != nil {
		return "", time.Time{}, err
	}
	memSessRemember(tok, username)
	return tok, exp, nil
}

func dbValidSession(tok string) bool {
	_, ok := dbSessionUser(tok)
	return ok
}

// dbSessionUser returns the username owning a valid session token.
// 当 pgsql 短暂不可用时回退到内存副本，避免所有用户被登出。
func dbSessionUser(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	var un string
	err := db.QueryRow(
		`SELECT username FROM ops_sessions WHERE token = $1 AND expires_at > now()`, tok).Scan(&un)
	if err == nil {
		memSessRemember(tok, un)
		return un, true
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	// DB 故障：使用内存副本
	return memSessLookup(tok)
}

// ── in-memory session mirror (fallback when pgsql is down) ────────

type memSessEntry struct {
	user string
	exp  time.Time
}

var memSess = struct {
	mu sync.RWMutex
	m  map[string]memSessEntry
}{m: map[string]memSessEntry{}}

func memSessRemember(tok, user string) {
	memSess.mu.Lock()
	memSess.m[tok] = memSessEntry{user: user, exp: time.Now().Add(sessionTTL)}
	if len(memSess.m) > 4096 {
		memSess.mu.Unlock()
		memSessCleanup()
		return
	}
	memSess.mu.Unlock()
}

func memSessLookup(tok string) (string, bool) {
	memSess.mu.RLock()
	e, ok := memSess.m[tok]
	memSess.mu.RUnlock()
	if !ok || time.Now().After(e.exp) {
		return "", false
	}
	return e.user, true
}

func memSessCleanup() {
	now := time.Now()
	memSess.mu.Lock()
	for k, e := range memSess.m {
		if now.After(e.exp) {
			delete(memSess.m, k)
		}
	}
	memSess.mu.Unlock()
}

func memSessDrop(tok string) {
	memSess.mu.Lock()
	delete(memSess.m, tok)
	memSess.mu.Unlock()
}

// DBHealthy reports whether pgsql is reachable right now.
func DBHealthy() bool {
	if db == nil {
		return false
	}
	return db.Ping() == nil
}

func dbDropSession(tok string) {
	if tok != "" {
		db.Exec(`DELETE FROM ops_sessions WHERE token = $1`, tok)
		memSessDrop(tok)
	}
}

// ── audit log (now in pgsql) ───────────────────────────────────────

func dbAuditLog(ip, target, action, result string) {
	db.Exec(`INSERT INTO ops_audit (ip, target, action, result) VALUES ($1, $2, $3, $4)`,
		ip, target, action, result)
}

// ── settings (key-value, used for AI provider config) ───────────────

// dbGetSetting returns one settings value ("", nil) when missing.
func dbGetSetting(key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM ops_settings WHERE key = $1`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// dbSetSetting upserts one settings value.
func dbSetSetting(key, value string) error {
	_, err := db.Exec(`INSERT INTO ops_settings (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()`, key, value)
	return err
}

// dbDeleteSetting removes one settings value.
func dbDeleteSetting(key string) error {
	_, err := db.Exec(`DELETE FROM ops_settings WHERE key = $1`, key)
	return err
}

// ── chat history (per-user transcripts, grouped by conversation) ──

func dbHistAppend(user string, conv int64, role, content, cards string) (int64, error) {
	var id int64
	err := db.QueryRow(
		`INSERT INTO ops_chat_history (username, conv_id, role, content, cards) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		user, conv, role, content, cards).Scan(&id)
	return id, err
}

func dbHistUpdateCards(user string, id int64, cards string) error {
	_, err := db.Exec(`UPDATE ops_chat_history SET cards = $3 WHERE username = $1 AND id = $2`, user, id, cards)
	return err
}

// dbHistTrimConvs keeps only the newest keep conversations (by newest msg).
func dbHistTrimConvs(user string, keep int) error {
	_, err := db.Exec(`DELETE FROM ops_chat_history WHERE username = $1 AND conv_id > 0 AND conv_id IN (
		SELECT conv_id FROM ops_chat_history WHERE username = $1 AND conv_id > 0
		GROUP BY conv_id ORDER BY max(id) DESC OFFSET $2)`, user, keep)
	return err
}

func dbHistConvs(user string) ([]gin.H, error) {
	rows, err := db.Query(`SELECT c.conv_id, max(c.id),
			to_char(max(c.created_at), 'YYYY-MM-DD HH24:MI'), count(*),
			coalesce(max(t.title), '')
		FROM ops_chat_history c
		LEFT JOIN LATERAL (
			SELECT h2.content AS title FROM ops_chat_history h2
			WHERE h2.username = c.username AND h2.conv_id = c.conv_id AND h2.role = 'user'
			ORDER BY h2.id LIMIT 1
		) t ON true
		WHERE c.username = $1 AND c.conv_id > 0
		GROUP BY c.conv_id ORDER BY max(c.id) DESC`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var conv, maxID, cnt int64
		var lastTime, title string
		if err := rows.Scan(&conv, &maxID, &lastTime, &cnt, &title); err != nil {
			return nil, err
		}
		out = append(out, gin.H{"convId": conv, "lastId": maxID, "title": title, "lastTime": lastTime, "msgs": cnt})
	}
	return out, rows.Err()
}

func dbHistClearConv(user string, conv int64) error {
	_, err := db.Exec(`DELETE FROM ops_chat_history WHERE username = $1 AND conv_id = $2`, user, conv)
	return err
}

func dbHistClear(user string) error {
	_, err := db.Exec(`DELETE FROM ops_chat_history WHERE username = $1`, user)
	return err
}

func dbHistList(user string, limit int) ([]gin.H, error) {
	rows, err := db.Query(`SELECT id, conv_id, role, content, cards, to_char(created_at, 'YYYY-MM-DD HH24:MI') FROM ops_chat_history WHERE username = $1 ORDER BY id`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []gin.H
	for rows.Next() {
		var id, conv int64
		var role, content, cards, created string
		if err := rows.Scan(&id, &conv, &role, &content, &cards, &created); err != nil {
			return nil, err
		}
		all = append(all, gin.H{"id": id, "convId": conv, "role": role, "content": content, "cards": cards, "time": created})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

func dbRecentAudit(limit int) ([]gin.H, error) {
	rows, err := db.Query(`SELECT to_char(time, 'YYYY-MM-DD HH24:MI:SS'), ip, target, action, result
		FROM ops_audit ORDER BY time DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var t, ip, tg, ac, rs string
		if err := rows.Scan(&t, &ip, &tg, &ac, &rs); err != nil {
			return nil, err
		}
		out = append(out, gin.H{"time": t, "ip": ip, "target": tg, "action": ac, "result": rs})
	}
	return out, rows.Err()
}

// CloseStore closes the pgsql connection pool.
func CloseStore() {
	closeStore()
}
