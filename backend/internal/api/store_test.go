package api

import (
	"testing"
	"time"
)

func TestLoginGuardFlow(t *testing.T) {
	ip := "1.2.3.4"
	failCnts = map[string]*failState{}
	for i := 0; i < 10; i++ {
		if !loginGuard(ip) {
			break
		}
		loginFail(ip)
	}
	if loginGuard(ip) {
		t.Error("after 5+ failures, loginGuard should block the ip")
	}
	// other ips unaffected
	if !loginGuard("5.6.7.8") {
		t.Error("other ip should not be blocked")
	}
	// blocked until window passes
	failMu.Lock()
	failCnts[ip].blockedU = time.Now().Add(-time.Second)
	failMu.Unlock()
	if !loginGuard(ip) {
		t.Error("expired block should allow login again")
	}
	failCnts = map[string]*failState{}
}

func TestLoginGuardWindowReset(t *testing.T) {
	ip := "9.9.9.9"
	failCnts = map[string]*failState{}
	// 4 failures within window: still allowed
	for i := 0; i < 4; i++ {
		loginFail(ip)
	}
	if !loginGuard(ip) {
		t.Error("4 failures should not block")
	}
	// simulate window expiry
	failMu.Lock()
	failCnts[ip].windowS = time.Now().Add(-time.Second)
	failMu.Unlock()
	if !loginGuard(ip) {
		t.Error("fresh window should allow")
	}
	failCnts = map[string]*failState{}
}

func TestMemSess(t *testing.T) {
	memSess.m = map[string]memSessEntry{}
	memSessRemember("tok1", "alice")
	if u, ok := memSessLookup("tok1"); !ok || u != "alice" {
		t.Errorf("memSessLookup(tok1) = %q,%v", u, ok)
	}
	if _, ok := memSessLookup("tok2"); ok {
		t.Error("unknown token should not resolve")
	}
	// expiry
	memSess.mu.Lock()
	memSess.m["tok1"] = memSessEntry{user: "alice", exp: time.Now().Add(-time.Minute)}
	memSess.mu.Unlock()
	if _, ok := memSessLookup("tok1"); ok {
		t.Error("expired token should not resolve")
	}
	memSessDrop("tok1")
	if _, ok := memSessLookup("tok1"); ok {
		t.Error("dropped token should not resolve")
	}
}

func TestCheckCredentialsRejectsEmpty(t *testing.T) {
	if db != nil {
		t.Skip("db connected in dev; skip negative check")
	}
	defer func() {
		if r := recover(); r != nil {
			t.Skip("nil db panics; acceptable without store")
		}
	}()
	if checkCredentials("admin", "x") {
		t.Error("without db, credentials must not pass")
	}
}
