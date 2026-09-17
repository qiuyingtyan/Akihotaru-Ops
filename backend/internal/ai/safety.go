package ai

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Safety level of a tool call, drives the approval flow:
//   - levelRead:  harmless query, runs immediately
//   - levelWrite: mutating panel op, needs one-click user approval
//   - levelShell: arbitrary shell command, needs approval + AI risk report
//                 and is hard-blocked when it matches the blacklist
type safetyLevel int

const (
	levelRead  safetyLevel = iota
	levelWrite
	levelShell
)

func (l safetyLevel) String() string {
	switch l {
	case levelRead:
		return "read"
	case levelWrite:
		return "write"
	default:
		return "shell"
	}
}

// ── shell blacklist: hard-blocked, never executable, no override ────

var shellBlacklistPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+(-[a-zA-Z-]*[rf][a-zA-Z-]*\s+)*/(\s|$)`),            // rm -rf /
	regexp.MustCompile(`\brm\s+-[a-zA-Z-]*r[a-zA-Z-]*f|\brm\s+-[a-zA-Z-]*f[a-zA-Z-]*r`), // rm -fr/rf any order
	regexp.MustCompile(`>\s*/dev/sd[a-z]`),                                        // overwrite disk device
	regexp.MustCompile(`\bmkfs(\.\w+)?\b`),                                        // format filesystem
	regexp.MustCompile(`\bdd\s+[^\n]*of=/dev/`),                                   // dd to device
	regexp.MustCompile(`:\(\)\s*\{\s*:\|\:&\s*\}\s*;`),                            // fork bomb
	regexp.MustCompile(`\b(shutdown|reboot|halt|poweroff|init\s+[06])\b`),         // power control
	regexp.MustCompile(`\b(mkswap|fdisk|parted|wipefs|blockdev)\b`),               // partition tools
	regexp.MustCompile(`\bchmod\s+(-[a-zA-Z]*\s+)?777\s+/(etc|usr|bin|sbin|lib|boot)\b`),
	regexp.MustCompile(`\bchown\s+[^\n]*\s+/(etc|usr|bin|sbin|lib|boot)\b`),       // chown system dirs
	regexp.MustCompile(`\biptables\s+(-F|--flush)\b`),                             // flush firewall
	regexp.MustCompile(`\bufw\s+(disable|reset)\b`),                               // disable firewall
	regexp.MustCompile(`\bsystemctl\s+(disable|mask)\s+(sshd|firewalld|opsweb)\b`),
	regexp.MustCompile(`\buser(del|mod)\s+(-r\s+)?(root|pf3090|admin)\b`),         // touch core accounts
	regexp.MustCompile(`\bpasswd\s+(root|pf3090)\b`),
	regexp.MustCompile(`/etc/(passwd|shadow|sudoers)\b[^\n]*[><]`),                // overwrite auth files
	regexp.MustCompile(`(>|>>)\s*/etc/(passwd|shadow|sudoers)\b`),                 // redirect onto auth files
	regexp.MustCompile(`\bcurl\b[^\n]*\|\s*(ba)?sh\b`),                            // remote script pipe
	regexp.MustCompile(`\bwget\b[^\n]*\|\s*(ba)?sh\b`),
	regexp.MustCompile(`\b(docker|kubectl)\s+(system\s+prune[^-\n]*-[a-zA-Z]*a)`, ),
	regexp.MustCompile(`\btruncate\s+-s\s*0\s+/(var|etc|workspace)/\S*log`),       // truncate logs
	regexp.MustCompile(`\bhistory\s+-c\b`),                                        // clear history
	regexp.MustCompile(`\bcrontab\s+-r\b`),                                        // wipe crontab
	regexp.MustCompile(`\bkill(all)?\s+(-9\s+)?-?1\b`),                            // kill init/1
	regexp.MustCompile(`\bkill\s+-9\s+\$?\(?pgrep\b[^\n]*sshd`),                   // kill ssh daemons
}

// blockedShellReasons pairs human-readable reasons with patterns.
var blockedShellReasons = map[string]string{
	`\brm\s+(-[a-zA-Z-]*[rf][a-zA-Z-]*\s+)*/(\s|$)`: "递归删除根目录",
	`\brm\s+-[a-zA-Z-]*r[a-zA-Z-]*f|\brm\s+-[a-zA-Z-]*f[a-zA-Z-]*r`: "递归强制删除",
	`>\s*/dev/sd[a-z]`:                              "直接写磁盘设备",
	`\bmkfs(\.\w+)?\b`:                              "格式化文件系统",
	`\bdd\s+[^\n]*of=/dev/`:                         "dd 写入设备",
	`:\(\)\s*\{\s*:\|\:&\s*\}\s*;`:                  "fork 炸弹",
	`\b(shutdown|reboot|halt|poweroff|init\s+[06])\b`: "关机/重启服务器",
	`\b(mkswap|fdisk|parted|wipefs|blockdev)\b`:     "分区操作",
	`\bchmod\s+(-[a-zA-Z]*\s+)?777\s+/(etc|usr|bin|sbin|lib|boot)\b`: "对系统目录宽松授权",
	`\bchown\s+[^\n]*\s+/(etc|usr|bin|sbin|lib|boot)\b`:              "变更系统目录属主",
	`\biptables\s+(-F|--flush)\b`:                                    "清空防火墙规则",
	`\bufw\s+(disable|reset)\b`:                                      "关闭防火墙",
	`\bsystemctl\s+(disable|mask)\s+(sshd|firewalld|opsweb)\b`:       "禁用关键系统服务",
	`\buser(del|mod)\s+(-r\s+)?(root|pf3090|admin)\b`:                "操作核心账号",
	`\bpasswd\s+(root|pf3090)\b`:                                     "修改核心账号密码",
	`/etc/(passwd|shadow|sudoers)\b[^\n]*[><]`:                       "覆写认证文件",
	`(>|>>)\s*/etc/(passwd|shadow|sudoers)\b`:                       "覆写认证文件",
	`\b(docker|kubectl)\s+(system\s+prune[^-\n]*-[a-zA-Z]*a)`:        "全量清理容器/镜像",
	`\btruncate\s+-s\s*0\s+/(var|etc|workspace)/\S*log`:              "清空系统日志",
	`\bhistory\s+-c\b`:                                               "清除历史记录",
	`\bcrontab\s+-r\b`:                                               "清空计划任务",
	`\bkill(all)?\s+(-9\s+)?-?1\b`:                                   "杀死 1 号进程",
	`\bkill\s+-9\s+\$?\(?pgrep\b[^\n]*sshd`:                          "杀死 SSH 服务",
}

// dangerousHintRegexps mark commands that are allowed but must be flagged
// in the risk report and always require explicit approval.
var dangerousHintRegexps = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`\bdocker\s+(rm|rmi|stop|kill|prune)\b`), "停止/删除容器或镜像"},
	{regexp.MustCompile(`\bsystemctl\s+(stop|restart|disable)\b`), "停止/重启/禁用系统服务"},
	{regexp.MustCompile(`\b(kill|pkill|killall)\b`), "终止进程"},
	{regexp.MustCompile(`\brm\b`), "删除文件"},
	{regexp.MustCompile(`\b(mv|cp)\b[^\n]*\s/(etc|usr|var|workspace)/`), "改动系统/业务目录文件"},
	{regexp.MustCompile(`\bchmod|\bchown`), "变更权限/属主"},
	{regexp.MustCompile(`\b(git)\s+(push|reset|clean)\b`), "Git 写操作"},
	{regexp.MustCompile(`\b(npm|pip|apt|yum|dnf|apk|go)\s+(install|remove|purge)\b`), "安装/卸载软件包"},
	{regexp.MustCompile(`\btar\b[^\n]*\s/(etc|usr|workspace)/`), "解压覆盖系统/业务目录"},
	{regexp.MustCompile(`>\s*/(etc|usr|workspace)/`), "覆写系统/业务文件"},
	{regexp.MustCompile(`\bsudo\b`), "sudo 提权执行"},
	{regexp.MustCompile(`\b(curl|wget)\b`), "访问外部网络"},
	{regexp.MustCompile(`\b(bash|sh)\s+-c\b`), "嵌套 shell 执行"},
}

// sensitiveTargetRegexps: reading these targets leaks credentials or
// private data; such commands never auto-run even when read-only.
var sensitiveTargetRegexps = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`(^|\s)/etc/(shadow|gshadow)\b`), "读取账号口令文件"},
	{regexp.MustCompile(`(^|\s)/root/\.ssh/`), "读取 SSH 私钥目录"},
	{regexp.MustCompile(`(^|\s)/root/\.bash_history`), "读取 shell 历史"},
	{regexp.MustCompile(`(^|\s)[^\s]*id_(rsa|ed25519|ecdsa)(\.\w+)?$`), "读取 SSH 私钥文件"},
	{regexp.MustCompile(`\.(git-credentials|netrc|pem)\b|\.aws/(credentials|config)\b|\.kube/config\b`), "读取云凭证/私钥文件"},
	{regexp.MustCompile(`(^|\s)(env|printenv)(\s|$)`), "导出环境变量（可能含密钥）"},
}

// commandPathWhitelist: binaries the assistant may run without approval
// when the command is a pure read-only query.
var shellReadOnlyFirstWord = map[string]bool{
	"cat": true, "ls": true, "head": true, "tail": true, "grep": true,
	"df": true, "du": true, "free": true, "uptime": true, "who": true,
	"ps": true, "date": true, "hostname": true, "wc": true, "stat": true,
	"uname": true, "lscpu": true, "vmstat": true, "iostat": true,
	"ss": true, "journalctl": true, "printenv": true, "id": true,
	"groups": true, "whoami": true, "nvidia-smi": true, "echo": true,
	"sort": true, "uniq": true, "cut": true, "tr": true,
}

// classifyShell inspects a raw shell command and returns its safety level,
// a hard block reason ("" when allowed), and flagged risk hints.
func classifyShell(cmd string) (safetyLevel, string, []string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return levelShell, "空命令", nil
	}

	var hints []string
	for _, s := range sensitiveTargetRegexps {
		if s.re.MatchString(cmd) {
			hints = append(hints, s.reason)
		}
	}
	for _, d := range dangerousHintRegexps {
		if d.re.MatchString(cmd) {
			hints = append(hints, d.reason)
		}
	}

	// Pure read-only pipelines without sensitive targets or risky flags
	// auto-execute. Checked BEFORE the blacklist so query commands that
	// merely mention banned words (e.g. grep shutdown /var/log/x) work.
	if isReadOnlyPipeline(cmd) && len(hints) == 0 {
		return levelRead, "", nil
	}

	for _, re := range shellBlacklistPatterns {
		if re.MatchString(cmd) {
			return levelShell, "命令命中安全黑名单：" + reasonFor(re.String()), nil
		}
	}
	if len(hints) == 0 {
		return levelWrite, "", nil
	}
	return levelShell, "", hints
}

func reasonFor(pattern string) string {
	if r, ok := blockedShellReasons[pattern]; ok {
		return r
	}
	return "危险操作"
}

// isReadOnlyPipeline checks every shell segment for read-only whitelist.
func isReadOnlyPipeline(cmd string) bool {
	if strings.ContainsAny(cmd, "\n") {
		return false
	}
	// reject any redirection or scheduling operators outright
	if regexp.MustCompile(`(^|\s)(>|>>|<|&|;|\|\||&&|` + "`" + `|\$\()`).MatchString(cmd) {
		return false
	}
	segments := splitPipe(cmd)
	if len(segments) == 0 {
		return false
	}
	for _, seg := range segments {
		fields := strings.Fields(strings.TrimSpace(seg))
		if len(fields) == 0 {
			return false
		}
		if shellReadOnlyFirstWord[fields[0]] {
			continue
		}
		// docker/systemctl/git are read-only only for whitelisted subcommands
		if sub, ok := shellReadOnlySubcommands[fields[0]]; ok {
			if len(fields) >= 2 && sub[fields[1]] {
				continue
			}
			return false
		}
		return false
	}
	return true
}

// shellReadOnlySubcommands: binaries that are read-only only for certain
// subcommands; the first argument must be whitelisted.
var shellReadOnlySubcommands = map[string]map[string]bool{
	"docker": {
		"ps": true, "images": true, "logs": true, "stats": true,
		"top": true, "version": true, "info": true, "port": true, "diff": true,
	},
	"systemctl": {
		"status": true, "list-units": true, "list-unit-files": true, "is-active": true,
		"is-enabled": true, "is-failed": true, "show": true, "cat": true, "list-timers": true,
	},
	"git":  {"status": true, "log": true, "diff": true, "branch": true, "show": true, "remote": true},
	"ip":   {"addr": true, "link": true, "route": true, "neigh": true},
}

func splitPipe(cmd string) []string {
	parts := strings.Split(cmd, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// shellExecTimeout picks a timeout by command shape: deployments and
// package installs get more headroom than quick queries.
func shellExecTimeout(cmd string) time.Duration {
	switch {
	case regexp.MustCompile(`\b(npm|pip|apt|yum|dnf|apk|go)\s+(install|build|remove|purge)\b`).MatchString(cmd):
		return 10 * time.Minute
	case regexp.MustCompile(`\b(docker\s+compose|docker\s+build|git\s+push)\b`).MatchString(cmd):
		return 10 * time.Minute
	case regexp.MustCompile(`\b(tar|cp|mv|rsync)\b`).MatchString(cmd):
		return 5 * time.Minute
	default:
		return 60 * time.Second
	}
}

// validShellBinary ensures the first word is an existing executable so the
// AI cannot smuggle control characters into the command name.
func validShellBinary(cmd string) error {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return fmt.Errorf("空命令")
	}
	name := fields[0]
	if strings.ContainsAny(name, "/\\") && !strings.HasPrefix(name, "/") {
		return fmt.Errorf("非法的可执行文件路径")
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("命令不存在: %s", name)
	}
	return nil
}
