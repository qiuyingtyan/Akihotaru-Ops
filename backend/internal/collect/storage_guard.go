package collect

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type LargeFileInfo struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	SizeFormatted string `json:"sizeFormatted"`
	ModTime       string `json:"modTime"`
	IsDir         bool   `json:"isDir"`
}

type StorageOverview struct {
	Disks             []DiskInfo      `json:"disks"`
	WarningThreshold  float64         `json:"warningThreshold"`
	CriticalThreshold float64         `json:"criticalThreshold"`
	AutoCleanEnabled  bool            `json:"autoCleanEnabled"`
	TopFiles          []LargeFileInfo `json:"topFiles"`
}

var (
	storageMu          sync.Mutex
	warningThreshold   = 85.0
	criticalThreshold  = 90.0
	autoCleanEnabled   = true
	cachedLargeFiles   []LargeFileInfo
	lastLargeFilesScan time.Time
)

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), units[exp])
}

func CoreScanLargeFiles(rootDir string, limit int) []LargeFileInfo {
	if limit <= 0 {
		limit = 20
	}
	targets := []string{"/workspace", "/var/log"}
	if rootDir != "" {
		targets = []string{rootDir}
	}

	storageMu.Lock()
	if rootDir == "" && len(cachedLargeFiles) > 0 && time.Since(lastLargeFilesScan) < 45*time.Second {
		res := make([]LargeFileInfo, len(cachedLargeFiles))
		copy(res, cachedLargeFiles)
		storageMu.Unlock()
		return res
	}
	storageMu.Unlock()

	var files []LargeFileInfo
	for _, root := range targets {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if name == ".git" || name == "node_modules" || name == "proc" || name == "sys" || name == ".local" {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			size := info.Size()
			if size >= 10*1024*1024 {
				files = append(files, LargeFileInfo{
					Path:          p,
					Name:          name,
					Size:          size,
					SizeFormatted: formatFileSize(size),
					ModTime:       info.ModTime().Format("2006-01-02 15:04:05"),
					IsDir:         false,
				})
			}
			return nil
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Size > files[j].Size
	})

	if len(files) > limit {
		files = files[:limit]
	}

	if rootDir == "" {
		storageMu.Lock()
		cachedLargeFiles = files
		lastLargeFilesScan = time.Now()
		storageMu.Unlock()
	}

	return files
}

func CoreTruncateLogFile(targetPath string) error {
	cleanPath := filepath.Clean(targetPath)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path")
	}

	disallowedPrefixes := []string{"/bin", "/sbin", "/usr", "/lib", "/etc", "/dev", "/sys", "/proc", "/boot"}
	for _, dp := range disallowedPrefixes {
		if strings.HasPrefix(cleanPath, dp) {
			return fmt.Errorf("protected system path cannot be truncated")
		}
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cannot truncate directory")
	}

	err = os.Truncate(cleanPath, 0)
	if err != nil {
		out, runErr := run(5*time.Second, "truncate", "-s", "0", cleanPath)
		if runErr != nil {
			sudoOut, sudoErr := run(5*time.Second, "sudo", "-n", "truncate", "-s", "0", cleanPath)
			if sudoErr != nil {
				return fmt.Errorf("truncate error: %v (%s), sudo error: %v (%s)", runErr, out, sudoErr, sudoOut)
			}
		}
	}

	storageMu.Lock()
	cachedLargeFiles = nil
	storageMu.Unlock()

	return nil
}

func CoreCleanOldArchives(days int) (int, int64, error) {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	cleanedCount := 0
	var freedBytes int64

	roots := []string{"/workspace", "/var/log", "/tmp"}
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == ".local" {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			name := strings.ToLower(d.Name())
			isArchive := strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".zip") ||
				strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".bak") ||
				(strings.HasPrefix(p, "/tmp/") && info.ModTime().Before(cutoff))

			if isArchive && info.ModTime().Before(cutoff) {
				sz := info.Size()
				if remErr := os.Remove(p); remErr == nil {
					cleanedCount++
					freedBytes += sz
				} else {
					_, _ = run(5*time.Second, "sudo", "-n", "rm", "-f", p)
				}
			}
			return nil
		})
	}

	storageMu.Lock()
	cachedLargeFiles = nil
	storageMu.Unlock()

	return cleanedCount, freedBytes, nil
}

func CoreRenderLogrotateRules(services []AppService) string {
	var paths []string
	seen := map[string]bool{}
	for _, s := range services {
		lp := strings.TrimSpace(s.LogPath)
		if lp != "" && !seen[lp] {
			paths = append(paths, lp)
			seen[lp] = true
		}
	}

	if len(paths) == 0 {
		paths = []string{"/workspace/**/*.log", "/workspace/**/*.out"}
	}

	header := strings.Join(paths, " ")
	return fmt.Sprintf(`%s {
    daily
    rotate 7
    size 50M
    missingok
    notifempty
    compress
    delaycompress
    copytruncate
}
`, header)
}

func CoreDeployLogrotate() (string, error) {
	services := GetRawServices()
	content := strings.ReplaceAll(CoreRenderLogrotateRules(services), "\r\n", "\n")
	destPath := "/etc/logrotate.d/opsweb-services"

	if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
		tmpPath := "/tmp/opsweb-services.lr"
		if writeErr := os.WriteFile(tmpPath, []byte(content), 0644); writeErr != nil {
			return "", writeErr
		}
		out, cpErr := run(10*time.Second, "sudo", "-n", "cp", tmpPath, destPath)
		_ = os.Remove(tmpPath)
		if cpErr != nil {
			return "", fmt.Errorf("deploy logrotate error: %v (%s)", cpErr, out)
		}
		_, _ = run(10*time.Second, "sudo", "-n", "chmod", "644", destPath)
	}

	return "logrotate rule deployed successfully to " + destPath, nil
}

func StorageOverviewHandler(c *gin.Context) {
	disks, _ := CoreDisks()
	topFiles := CoreScanLargeFiles("", 20)

	storageMu.Lock()
	warn := warningThreshold
	crit := criticalThreshold
	auto := autoCleanEnabled
	storageMu.Unlock()

	res := StorageOverview{
		Disks:             disks,
		WarningThreshold:  warn,
		CriticalThreshold: crit,
		AutoCleanEnabled:  auto,
		TopFiles:          topFiles,
	}
	ok(c, res)
}

func StorageLargeFilesHandler(c *gin.Context) {
	dir := c.Query("path")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	ok(c, CoreScanLargeFiles(dir, limit))
}

type TruncateReq struct {
	Path string `json:"path"`
}

func StorageTruncateHandler(c *gin.Context) {
	var req TruncateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err.Error())
		return
	}
	if err := CoreTruncateLogFile(req.Path); err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, "truncated")
}

type CleanArchivesReq struct {
	Days int `json:"days"`
}

func StorageCleanArchivesHandler(c *gin.Context) {
	var req CleanArchivesReq
	_ = c.ShouldBindJSON(&req)
	cnt, freed, err := CoreCleanOldArchives(req.Days)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, gin.H{
		"cleanedCount": cnt,
		"freedBytes":   freed,
		"freedText":    formatFileSize(freed),
	})
}

func StorageDeployLogrotateHandler(c *gin.Context) {
	msg, err := CoreDeployLogrotate()
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, msg)
}

type StorageSettingsReq struct {
	WarningThreshold  float64 `json:"warningThreshold"`
	CriticalThreshold float64 `json:"criticalThreshold"`
	AutoCleanEnabled  bool    `json:"autoCleanEnabled"`
}

func StorageSaveSettingsHandler(c *gin.Context) {
	var req StorageSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err.Error())
		return
	}
	storageMu.Lock()
	if req.WarningThreshold > 0 {
		warningThreshold = req.WarningThreshold
	}
	if req.CriticalThreshold > 0 {
		criticalThreshold = req.CriticalThreshold
	}
	autoCleanEnabled = req.AutoCleanEnabled
	storageMu.Unlock()
	ok(c, "saved")
}
