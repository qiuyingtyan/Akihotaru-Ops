package collect

import (
	"bufio"
	"compress/gzip"
	"fmt"
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
	data, err := CoreJournal(c.Query("unit"), c.Query("since"), c.Query("grep"), c.DefaultQuery("tail", "200"))
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, data)
}

func CoreJournal(unit, since, grep, tail string) (gin.H, error) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, fmt.Errorf("journalctl 不可用")
	}
	args := []string{"--no-pager", "-q", "-o", "short-iso"}
	if unit != "" {
		if !validUnitName(unit) {
			return nil, fmt.Errorf("非法的 unit 名称")
		}
		args = append(args, "-u", unit)
	}
	if since != "" {
		args = append(args, "--since", since)
	}
	if grep != "" {
		args = append(args, "--grep", grep)
	}
	args = append(args, "--reverse")

	n := clampTail(tail)

	// fetch a generous window then trim to n lines
	cmd := exec.Command("journalctl", args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl 执行失败: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > n {
		// --reverse gives newest first; keep newest n then restore chrono order
		lines = lines[:n]
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	return gin.H{"lines": lines, "count": len(lines)}, nil
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
	data, err := CoreLogFile(c.Query("path"), c.DefaultQuery("tail", "200"))
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, data)
}

func CoreLogFile(path, tail string) (gin.H, error) {
	if path == "" {
		return nil, fmt.Errorf("path required")
	}
	clean, err := resolveLogPath(path)
	if err != nil {
		return nil, err
	}
	n := clampTail(tail)

	f, err := os.Open(clean)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, fmt.Errorf("这是一个目录，请选择具体文件（可用目录浏览接口）")
	}

	var lines []string
	var size int64 = st.Size()
	if strings.HasSuffix(clean, ".gz") {
		// gz: whole-file decompress but cap at 64MB decompressed
		gz, gerr := gzip.NewReader(io.LimitReader(f, maxLogReadBytes))
		if gerr != nil {
			return nil, gerr
		}
		defer gz.Close()
		lines = tailLines(gz, n)
	} else {
		// plain file: only read the tail (last 8MB max)
		readFrom := int64(0)
		if size > maxLogReadBytes {
			readFrom = size - maxLogReadBytes
			if _, err := f.Seek(readFrom, io.SeekStart); err != nil {
				return nil, err
			}
			// skip to next newline to avoid a partial first line
			br := bufio.NewReader(f)
			if _, err := br.ReadString('\n'); err != nil && err != io.EOF {
				return nil, err
			}
			lines = tailLines(br, n)
		} else {
			lines = tailLines(bufio.NewReader(f), n)
		}
	}
	return gin.H{
		"path":  clean,
		"lines": lines,
		"size":  size,
		"mtime": st.ModTime().Format("2006-01-02 15:04:05"),
	}, nil
}
