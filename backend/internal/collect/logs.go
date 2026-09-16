package collect

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var allowedLogRoots = []string{
	"/workspace/baq-test/logs", "/workspace/szx-test/logs",
	"/workspace/baq-test/.deploy", "/workspace/szx-test/.deploy",
}

const maxLogReadBytes = 8 << 20 // read at most 8MB from the end of file

// journal units that are hidden from the generic system log view by default
var noisyUnits = []string{"user@1000.service", "session-c1.scope", "session-1596.scope"}

// JournalHandler tails systemd journal via journalctl.
// params: unit, since, grep, tail
func JournalHandler(c *gin.Context) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		fail(c, "journalctl 不可用")
		return
	}
	args := []string{"--no-pager", "-q", "-o", "short-iso"}
	if unit := c.Query("unit"); unit != "" {
		if !validUnitName(unit) {
			fail(c, "非法的 unit 名称")
			return
		}
		args = append(args, "-u", unit)
	}
	if since := c.Query("since"); since != "" {
		args = append(args, "--since", since)
	}
	if grep := c.Query("grep"); grep != "" {
		args = append(args, "--grep", grep)
	}
	args = append(args, "--reverse")

	n := clampTail(c.DefaultQuery("tail", "200"))

	// fetch a generous window then trim to n lines
	cmd := exec.Command("journalctl", args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		fail(c, "journalctl 执行失败: "+err.Error())
		return
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > n {
		// --reverse gives newest first; keep newest n then restore chrono order
		lines = lines[:n]
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	ok(c, gin.H{"lines": lines, "count": len(lines)})
}

func validUnitName(u string) bool {
	if len(u) > 128 {
		return false
	}
	for _, r := range u {
		alphanumeric := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !alphanumeric && r != '.' && r != '-' && r != '_' && r != '@' {
			return false
		}
	}
	return true
}

func clampTail(s string) int {
	n, _ := strconv.Atoi(s)
	if n <= 0 || n > 5000 {
		n = 200
	}
	return n
}

// ── directory listing for the log browser ──────────────────────────

// LogListHandler lists files/dirs under an allowed root (one level or
// recursive=false only) so the frontend can browse to a file.
func LogListHandler(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		path = "/var/log"
	}
	dir, err := resolveLogPath(path)
	if err != nil {
		fail(c, err.Error())
		return
	}
	st, err := os.Stat(dir)
	if err != nil {
		fail(c, err.Error())
		return
	}
	if !st.IsDir() {
		fail(c, "不是目录")
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fail(c, err.Error())
		return
	}
	type item struct {
		Name  string `json:"name"`
		IsDir bool   `json:"isDir"`
		Size  int64  `json:"size"`
	}
	var dirs, files []item
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, ierr := e.Info()
		var size int64
		if ierr == nil {
			size = info.Size()
		}
		it := item{Name: name, IsDir: e.IsDir(), Size: size}
		if e.IsDir() {
			dirs = append(dirs, it)
		} else {
			files = append(files, it)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	ok(c, gin.H{"path": dir, "dirs": dirs, "files": files})
}

// resolveLogPath validates and cleans a user-supplied log path against
// the whitelist. Allows /var/log too.
func resolveLogPath(p string) (string, error) {
	if p == "" || !strings.HasPrefix(p, "/") {
		return "", errInvalidPath
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	allowedRoots := append([]string{}, allowedLogRoots...)
	allowedRoots = append(allowedRoots, "/var/log")
	for _, root := range allowedRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			if strings.Contains(clean[len(root):], "..") {
				continue
			}
			return clean, nil
		}
	}
	return "", errPathNotAllowed
}

var (
	errInvalidPath    = &errStr{"路径不合法"}
	errPathNotAllowed = &errStr{"路径不在允许范围内"}
)

type errStr struct{ s string }

func (e *errStr) Error() string { return e.s }

// ── file tail with gzip support (rewritten LogFileHandler) ─────────

func tailLines(r io.Reader, n int) []string {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[len(lines)-n:]
		}
	}
	return lines
}

// LogFileHandler tails a whitelisted log file (plain or .gz).
func LogFileHandler(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		fail(c, "path required")
		return
	}
	clean, err := resolveLogPath(path)
	if err != nil {
		fail(c, err.Error())
		return
	}
	n := clampTail(c.DefaultQuery("tail", "200"))

	f, err := os.Open(clean)
	if err != nil {
		fail(c, err.Error())
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		fail(c, err.Error())
		return
	}
	if st.IsDir() {
		fail(c, "这是一个目录，请选择具体文件（可用目录浏览接口）")
		return
	}

	var lines []string
	var size int64 = st.Size()
	if strings.HasSuffix(clean, ".gz") {
		// gz: whole-file decompress but cap at 64MB decompressed
		gz, gerr := gzip.NewReader(io.LimitReader(f, maxLogReadBytes))
		if gerr != nil {
			fail(c, gerr.Error())
			return
		}
		defer gz.Close()
		lines = tailLines(gz, n)
	} else {
		// plain file: only read the tail (last 8MB max)
		readFrom := int64(0)
		if size > maxLogReadBytes {
			readFrom = size - maxLogReadBytes
			if _, err := f.Seek(readFrom, io.SeekStart); err != nil {
				fail(c, err.Error())
				return
			}
			// skip to next newline to avoid a partial first line
			br := bufio.NewReader(f)
			if _, err := br.ReadString('\n'); err != nil && err != io.EOF {
				fail(c, err.Error())
				return
			}
			lines = tailLines(br, n)
		} else {
			lines = tailLines(bufio.NewReader(f), n)
		}
	}
	ok(c, gin.H{
		"path":  clean,
		"lines": lines,
		"size":  size,
		"mtime": st.ModTime().Format("2006-01-02 15:04:05"),
	})
}
