package collect

import (
	"strings"
	"testing"
)

func TestResolveLogPathAllowed(t *testing.T) {
	cases := []string{
		"/var/log",
		"/var/log/syslog",
		"/var/log/nginx/access.log",
		"/workspace/baq-test/logs/main/info.log",
		"/workspace/szx-test/.deploy/x",
	}
	for _, p := range cases {
		if _, err := resolveLogPath(p); err != nil {
			t.Errorf("resolveLogPath(%q) = %v, want allowed", p, err)
		}
	}
}

func TestResolveLogPathDenied(t *testing.T) {
	cases := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/root/.ssh/id_rsa",
		"/workspace/other/file",
		"/var/logx/evil",
		"",
		"relative/path",
	}
	for _, p := range cases {
		if got, err := resolveLogPath(p); err == nil {
			t.Errorf("resolveLogPath(%q) = %q, want denied", p, got)
		}
	}
}

// 路径穿越：.. 必须被拒绝（即使最终解析落在白名单内也不允许）
func TestResolveLogPathTraversal(t *testing.T) {
	cases := []string{
		"/var/log/../../etc/passwd",
		"/var/log/../shadow",
		"/workspace/baq-test/logs/../../../etc/passwd",
		"/var/log/sub/../../../etc/passwd",
	}
	for _, p := range cases {
		if got, err := resolveLogPath(p); err == nil {
			t.Errorf("resolveLogPath(%q) = %q, want traversal denied", p, got)
		}
	}
}

func TestValidUnitName(t *testing.T) {
	for _, ok := range []string{"nginx.service", "ssh", "user@1000.service", "systemd-journald.service", "a-b_c.service"} {
		if !validUnitName(ok) {
			t.Errorf("validUnitName(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"a;reboot", "a b", "a$HOME", "a|b", "a&b", "../etc", "a`id`", string(make([]byte, 200))} {
		if validUnitName(bad) {
			t.Errorf("validUnitName(%q) = true, want false", bad)
		}
	}
}

func TestClampTail(t *testing.T) {
	if clampTail("0") != 200 || clampTail("-5") != 200 || clampTail("abc") != 200 {
		t.Error("clampTail should fall back to 200 for invalid input")
	}
	if clampTail("50") != 50 {
		t.Error("clampTail(50) should be 50")
	}
	if clampTail("99999") != 200 {
		t.Error("clampTail should cap at 200 for absurd values")
	}
}

func TestEvalRule(t *testing.T) {
	r := AlertRule{Op: ">", Value: 90}
	if !evalRule(95, r) || evalRule(85, r) {
		t.Error("evalRule > failed")
	}
	r2 := AlertRule{Op: "<", Value: 10}
	if !evalRule(5, r2) || evalRule(15, r2) {
		t.Error("evalRule < failed")
	}
	if evalRule(5, AlertRule{Op: "=", Value: 5}) {
		t.Error("unknown op should be false")
	}
}

func TestTailLines(t *testing.T) {
	in := "l1\nl2\nl3\nl4\nl5"
	got := tailLines(strings.NewReader(in), 3)
	if len(got) != 3 || got[0] != "l3" || got[2] != "l5" {
		t.Errorf("tailLines = %v, want last 3", got)
	}
	if got := tailLines(strings.NewReader(""), 3); len(got) != 0 {
		t.Errorf("tailLines(empty) = %v", got)
	}
}
