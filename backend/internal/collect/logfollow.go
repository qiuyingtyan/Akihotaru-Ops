package collect

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	followInterval = 800 * time.Millisecond
	followMaxLines = 5000 // hard cap on buffered lines per client
)

// LogFollowHandler streams new lines of a whitelisted log file via SSE.
// Params: path, tail (initial history lines).
func LogFollowHandler(c *gin.Context) {
	path := c.Query("path")
	clean, err := resolveLogPath(path)
	if err != nil {
		fail(c, err.Error())
		return
	}
	f, err := os.Open(clean)
	if err != nil {
		fail(c, err.Error())
		return
	}
	f.Close()
	st, err := os.Stat(clean)
	if err != nil || st.IsDir() {
		fail(c, "不是可跟随的文件")
		return
	}
	isGz := strings.HasSuffix(clean, ".gz")

	n := clampTail(c.DefaultQuery("tail", "50"))

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(200)

	ctx := c.Request.Context()

	// send initial history then follow from EOF
	if !isGz {
		if lines := tailFile(clean, n); len(lines) > 0 {
			writeSSE(c, "history", strings.Join(lines, "\n"))
		}
	} else {
		if lines := tailGzFile(clean, n); len(lines) > 0 {
			writeSSE(c, "history", strings.Join(lines, "\n"))
		}
	}
	writeSSE(c, "ready", clean)
	c.Writer.Flush()

	if isGz {
		// gz files are rotated/compressed: poll for mtime change and re-read tail
		lastMod := st.ModTime()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			nst, err := os.Stat(clean)
			if err != nil {
				writeSSE(c, "error", "文件不存在或不可读")
				c.Writer.Flush()
				return
			}
			if nst.ModTime().After(lastMod) {
				lastMod = nst.ModTime()
				if lines := tailGzFile(clean, 20); len(lines) > 0 {
					writeSSE(c, "lines", strings.Join(lines, "\n"))
					c.Writer.Flush()
				}
			}
		}
	}

	// plain file: poll for size growth and stream new bytes
	offset := fileSize(clean)
	ticker := time.NewTicker(followInterval)
	defer ticker.Stop()
	truncWarned := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		nst, err := os.Stat(clean)
		if err != nil {
			writeSSE(c, "error", "文件不存在或不可读")
			c.Writer.Flush()
			return
		}
		size := nst.Size()
		if size < offset {
			// truncated (logrotate): restart from beginning
			offset = 0
			if !truncWarned {
				writeSSE(c, "note", "-- 日志被轮转截断，从头开始 --")
				truncWarned = true
			}
		}
		if size == offset {
			continue
		}
		f, err := os.Open(clean)
		if err != nil {
			return
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return
		}
		data, _ := io.ReadAll(io.LimitReader(f, 2<<20))
		f.Close()
		// align to last complete line; keep remainder as new offset
		text := string(data)
		if li := strings.LastIndexByte(text, '\n'); li >= 0 {
			text = text[:li+1]
			offset += int64(li + 1)
		} else {
			// no complete line yet
			continue
		}
		truncWarned = false
		if text != "" {
			writeSSE(c, "lines", strings.TrimSuffix(text, "\n"))
			c.Writer.Flush()
		}
	}
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func tailFile(p string, n int) []string {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	var r io.Reader = f
	if sz := st.Size(); sz > maxLogReadBytes {
		if _, err := f.Seek(sz-maxLogReadBytes, io.SeekStart); err != nil {
			return nil
		}
		br := bufio.NewReader(f)
		br.ReadString('\n')
		r = br
	}
	return tailLines(r, n)
}

func tailGzFile(p string, n int) []string {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	gz, err := gzip.NewReader(io.LimitReader(f, maxLogReadBytes))
	if err != nil {
		return nil
	}
	defer gz.Close()
	return tailLines(gz, n)
}

// JournalFollowHandler streams journalctl -f via SSE.
// Params: unit, grep. Starts from "now" so only new lines stream.
func JournalFollowHandler(c *gin.Context) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		fail(c, "journalctl 不可用")
		return
	}
	args := []string{"--no-pager", "-q", "-o", "short-iso", "-n", "0", "-f"}
	if unit := c.Query("unit"); unit != "" {
		if !validUnitName(unit) {
			fail(c, "非法的 unit 名称")
			return
		}
		args = append(args, "-u", unit)
	}
	if grep := c.Query("grep"); grep != "" {
		args = append(args, "--grep", grep)
	}
	cmd := exec.Command("journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fail(c, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		fail(c, err.Error())
		return
	}
	defer cmd.Process.Kill()
	go func() { <-c.Request.Context().Done(); cmd.Process.Kill() }()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(200)
	writeSSE(c, "ready", "journal")
	c.Writer.Flush()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var batch []string
	flush := func() {
		if len(batch) > 0 {
			writeSSE(c, "lines", strings.Join(batch, "\n"))
			c.Writer.Flush()
			batch = batch[:0]
		}
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			flush()
		default:
			if !sc.Scan() {
				flush()
				return
			}
			batch = append(batch, sc.Text())
			if len(batch) >= 200 {
				flush()
			}
		}
	}
}

func writeSSE(c *gin.Context, event, data string) {
	c.Writer.WriteString("event: " + event + "\n")
	for _, line := range strings.Split(data, "\n") {
		c.Writer.WriteString("data: " + line + "\n")
	}
	c.Writer.WriteString("\n")
}

// sendSSE is used by handlers that already wrote the SSE header.
func sendSSE(c *gin.Context, event, data string) {
	writeSSE(c, event, data)
	c.Writer.Flush()
}
