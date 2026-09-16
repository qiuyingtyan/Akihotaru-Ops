package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// Init registers the embedded SPA serving on the router.
func Init() {}

// RegisterStatic mounts the embedded frontend onto the given router group.
func RegisterStatic(r *gin.Engine) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return
	}
	fileServer := http.StripPrefix("/", http.FileServer(http.FS(sub)))
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// serve real files; fall back to index.html for SPA routes only
		if f, err := sub.Open(strings.TrimPrefix(p, "/")); err == nil {
			f.Close()
			if strings.HasPrefix(p, "/assets/") {
				// hashed filenames are safe to cache forever
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		index, ierr := fs.ReadFile(sub, "index.html")
		if ierr != nil {
			c.String(http.StatusNotFound, "frontend not built")
			return
		}
		// only SPA routes (no file extension) get index.html;
		// stale asset requests must 404 instead of returning HTML
		if strings.HasSuffix(p, "/") || path.Ext(p) == "" {
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", index)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
}
