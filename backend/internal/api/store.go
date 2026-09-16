package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
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
	if err = d.Ping(); err != nil {
		return err
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
CREATE INDEX IF NOT EXISTS idx_ops_sessions_exp ON ops_sessions (expires_at);`
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
	return nil
}

func cleanupLoop() {
	for range time.Tick(1 * time.Hour) {
		db.Exec(`DELETE FROM ops_sessions WHERE expires_at < now()`)
		db.Exec(`DELETE FROM ops_audit WHERE time < now() - interval '90 days'`)
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
	return tok, exp, nil
}

func dbValidSession(tok string) bool {
	if tok == "" {
		return false
	}
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM ops_sessions WHERE token = $1 AND expires_at > now()`, tok).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// dbSessionUser returns the username owning a valid session token.
func dbSessionUser(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	var un string
	if err := db.QueryRow(
		`SELECT username FROM ops_sessions WHERE token = $1 AND expires_at > now()`, tok).Scan(&un); err != nil {
		return "", false
	}
	return un, true
}

func dbDropSession(tok string) {
	if tok != "" {
		db.Exec(`DELETE FROM ops_sessions WHERE token = $1`, tok)
	}
}

// ── audit log (now in pgsql) ───────────────────────────────────────

func dbAuditLog(ip, target, action, result string) {
	db.Exec(`INSERT INTO ops_audit (ip, target, action, result) VALUES ($1, $2, $3, $4)`,
		ip, target, action, result)
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
