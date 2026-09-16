package collect

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var allowedLogRoots = []string{
	"/workspace/baq-test/logs", "/workspace/szx-test/logs",
	"/workspace/baq-test/.deploy", "/workspace/szx-test/.deploy",
}

const maxLogReadBytes = 8 << 20 // read at most 8MB from the end of file

// LogFileHandler tails a whitelisted log file.
func LogFileHandler(c *gin.Context) {
	path := c.Query("path")
	tail := c.DefaultQuery("tail", "200")
	if path == "" {
		fail(c, "path required")
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		fail(c, "invalid path")
		return
	}
	clean := filepath.Clean(abs)
	allowed := false
	for _, root := range allowedLogRoots {
		if strings.HasPrefix(clean, root+"/") && !strings.Contains(clean, "..") {
			allowed = true
			break
		}
	}
	if !allowed {
		fail(c, "path not allowed")
		return
	}
	n, _ := strconv.Atoi(tail)
	if n <= 0 || n > 5000 {
		n = 200
	}
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
	readSize := st.Size()
	if readSize > maxLogReadBytes {
		f.Seek(-maxLogReadBytes, io.SeekEnd)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxLogReadBytes))
	if err != nil {
		fail(c, err.Error())
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	ok(c, gin.H{
		"path":  clean,
		"lines": lines,
		"size":  st.Size(),
		"mtime": st.ModTime().Format(time.DateTime),
	})
}
