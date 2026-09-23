package collect

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type AppService struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	DisplayName   string  `json:"displayName"`
	GroupName     string  `json:"groupName"`
	Level         int     `json:"level"`
	WorkDir       string  `json:"workDir"`
	ExecStart     string  `json:"execStart"`
	ExecStop      string  `json:"execStop"`
	AfterDeps     string  `json:"afterDeps"`
	Port          int     `json:"port"`
	LogPath       string  `json:"logPath"`
	RestartPolicy string  `json:"restartPolicy"`
	ServiceType   string  `json:"serviceType"`
	AutoStart     bool    `json:"autoStart"`
	CreatedAt     string  `json:"createdAt,omitempty"`
	UpdatedAt     string  `json:"updatedAt,omitempty"`

	Status        string  `json:"status"`
	SubState      string  `json:"subState"`
	PID           int     `json:"pid"`
	PortListening bool    `json:"portListening"`
	CPUPerc       float64 `json:"cpuPercent"`
	RSSMB         uint64  `json:"rssMB"`
	Uptime        string  `json:"uptime"`
	UnitInstalled bool    `json:"unitInstalled"`
}

type RebootSelfCheckResult struct {
	UptimeSec       uint64       `json:"uptimeSec"`
	UptimeFormatted string       `json:"uptimeFormatted"`
	TotalCount      int          `json:"totalCount"`
	HealthyCount    int          `json:"healthyCount"`
	UnhealthyCount  int          `json:"unhealthyCount"`
	AllHealthy      bool         `json:"allHealthy"`
	LevelSummary    map[int]int  `json:"levelSummary"`
	Services        []AppService `json:"services"`
}

var (
	appServicesMu sync.RWMutex
	appServiceDB  *sql.DB
)

const localServicesFile = "/workspace/opsweb/app_services.json"

func SetAppServiceDB(d *sql.DB) {
	appServicesMu.Lock()
	defer appServicesMu.Unlock()
	appServiceDB = d
}

func loadServicesFromLocal() ([]AppService, error) {
	data, err := os.ReadFile(localServicesFile)
	if err != nil {
		return nil, err
	}
	var list []AppService
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveServicesToLocal(list []AppService) {
	_ = os.MkdirAll(filepath.Dir(localServicesFile), 0755)
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(localServicesFile+".tmp", data, 0644)
		_ = os.Rename(localServicesFile+".tmp", localServicesFile)
	}
}

func queryServicesDB() ([]AppService, error) {
	if appServiceDB == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	rows, err := appServiceDB.Query(`
		SELECT id, name, display_name, group_name, level, work_dir, exec_start,
		       exec_stop, after_deps, port, log_path, restart_policy, service_type, auto_start,
		       to_char(created_at, 'YYYY-MM-DD HH24:MI:SS'),
		       to_char(updated_at, 'YYYY-MM-DD HH24:MI:SS')
		FROM ops_app_services ORDER BY level ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AppService
	for rows.Next() {
		var s AppService
		if err := rows.Scan(&s.ID, &s.Name, &s.DisplayName, &s.GroupName, &s.Level,
			&s.WorkDir, &s.ExecStart, &s.ExecStop, &s.AfterDeps, &s.Port, &s.LogPath,
			&s.RestartPolicy, &s.ServiceType, &s.AutoStart, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		list = append(list, s)
	}
	return list, nil
}

func GetRawServices() []AppService {
	appServicesMu.RLock()
	list, err := queryServicesDB()
	appServicesMu.RUnlock()

	if err != nil || len(list) == 0 {
		if localList, localErr := loadServicesFromLocal(); localErr == nil && len(localList) > 0 {
			list = localList
		}
	} else {
		saveServicesToLocal(list)
	}
	return list
}

func isPortOpen(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 120*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func querySystemdServiceStatus(unitName string) (status string, subState string, pid int) {
	out, err := run(3*time.Second, "systemctl", "show", unitName, "--property=ActiveState,SubState,MainPID")
	if err != nil {
		return "unknown", "unknown", 0
	}
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "ActiveState=") {
			status = strings.TrimPrefix(l, "ActiveState=")
		} else if strings.HasPrefix(l, "SubState=") {
			subState = strings.TrimPrefix(l, "SubState=")
		} else if strings.HasPrefix(l, "MainPID=") {
			pid, _ = strconv.Atoi(strings.TrimPrefix(l, "MainPID="))
		}
	}
	return status, subState, pid
}

func inspectAppServiceRuntime(s *AppService, procs []ProcProcess) {
	unitName := fmt.Sprintf("ops-%s.service", s.Name)
	unitFile := fmt.Sprintf("/etc/systemd/system/%s", unitName)
	if _, err := os.Stat(unitFile); err == nil {
		s.UnitInstalled = true
	}

	st, sub, pid := querySystemdServiceStatus(unitName)
	s.Status = st
	s.SubState = sub
	s.PID = pid

	s.PortListening = isPortOpen(s.Port)

	if s.PID <= 0 && len(procs) > 0 {
		for _, p := range procs {
			if strings.Contains(p.Name, s.Name) || (s.WorkDir != "" && strings.Contains(p.Name, s.WorkDir)) {
				s.PID = p.Pid
				s.CPUPerc = p.CPUPerc
				s.RSSMB = p.RSS
				if s.Status == "" || s.Status == "unknown" || s.Status == "inactive" {
					s.Status = "active"
					s.SubState = "running"
				}
				break
			}
		}
	}

	if s.PID > 0 {
		for _, p := range procs {
			if p.Pid == s.PID {
				s.CPUPerc = p.CPUPerc
				s.RSSMB = p.RSS
				break
			}
		}
	}

	if s.Status == "" || s.Status == "unknown" {
		if s.PortListening {
			s.Status = "active"
			s.SubState = "running"
		} else {
			s.Status = "inactive"
			s.SubState = "dead"
		}
	}
}

func CoreGetAppServices() []AppService {
	raw := GetRawServices()
	procs := listProcesses()
	result := make([]AppService, len(raw))

	var wg sync.WaitGroup
	for i := range raw {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			svc := raw[idx]
			inspectAppServiceRuntime(&svc, procs)
			result[idx] = svc
		}(i)
	}
	wg.Wait()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Level != result[j].Level {
			return result[i].Level < result[j].Level
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func CoreCreateAppService(s AppService) (int64, error) {
	name := strings.TrimSpace(s.Name)
	if name == "" || strings.ContainsAny(name, " /\\:*?\"<>|;$`&'") {
		return 0, fmt.Errorf("invalid service identifier name")
	}
	if s.DisplayName == "" {
		s.DisplayName = name
	}
	if s.Level <= 0 {
		s.Level = 3
	}
	if s.RestartPolicy == "" {
		s.RestartPolicy = "always"
	}

	appServicesMu.Lock()
	var newID int64
	if appServiceDB != nil {
		err := appServiceDB.QueryRow(`
			INSERT INTO ops_app_services (
				name, display_name, group_name, level, work_dir, exec_start,
				exec_stop, after_deps, port, log_path, restart_policy, service_type, auto_start
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING id`,
			name, s.DisplayName, s.GroupName, s.Level, s.WorkDir, s.ExecStart,
			s.ExecStop, s.AfterDeps, s.Port, s.LogPath, s.RestartPolicy, s.ServiceType, s.AutoStart,
		).Scan(&newID)
		if err != nil {
			appServicesMu.Unlock()
			return 0, err
		}
	} else {
		newID = time.Now().UnixMilli()
	}

	s.ID = newID
	localList, _ := loadServicesFromLocal()
	localList = append(localList, s)
	saveServicesToLocal(localList)
	appServicesMu.Unlock()

	_ = generateAndInstallSystemdUnit(s)
	return newID, nil
}

func CoreUpdateAppService(id int64, s AppService) error {
	name := strings.TrimSpace(s.Name)
	if name == "" || strings.ContainsAny(name, " /\\:*?\"<>|;$`&'") {
		return fmt.Errorf("invalid service identifier name")
	}
	if s.DisplayName == "" {
		s.DisplayName = name
	}
	if s.Level <= 0 {
		s.Level = 3
	}
	if s.RestartPolicy == "" {
		s.RestartPolicy = "always"
	}

	appServicesMu.Lock()
	if appServiceDB != nil {
		_, err := appServiceDB.Exec(`
			UPDATE ops_app_services SET
				name = $1, display_name = $2, group_name = $3, level = $4,
				work_dir = $5, exec_start = $6, exec_stop = $7, after_deps = $8,
				port = $9, log_path = $10, restart_policy = $11, service_type = $12,
				auto_start = $13, updated_at = now()
			WHERE id = $14`,
			name, s.DisplayName, s.GroupName, s.Level, s.WorkDir, s.ExecStart,
			s.ExecStop, s.AfterDeps, s.Port, s.LogPath, s.RestartPolicy, s.ServiceType,
			s.AutoStart, id)
		if err != nil {
			appServicesMu.Unlock()
			return err
		}
	}

	localList, _ := loadServicesFromLocal()
	for i := range localList {
		if localList[i].ID == id {
			s.ID = id
			localList[i] = s
			break
		}
	}
	saveServicesToLocal(localList)
	appServicesMu.Unlock()

	_ = generateAndInstallSystemdUnit(s)
	return nil
}

func CoreDeleteAppService(id int64) error {
	raw := GetRawServices()
	var target AppService
	found := false
	for _, s := range raw {
		if s.ID == id {
			target = s
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("service id %d not found", id)
	}

	appServicesMu.Lock()
	if appServiceDB != nil {
		_, err := appServiceDB.Exec(`DELETE FROM ops_app_services WHERE id = $1`, id)
		if err != nil {
			appServicesMu.Unlock()
			return err
		}
	}

	localList, _ := loadServicesFromLocal()
	filtered := make([]AppService, 0, len(localList))
	for _, s := range localList {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	saveServicesToLocal(filtered)
	appServicesMu.Unlock()

	_ = removeSystemdUnit(target.Name)
	return nil
}

func resolveExecCmd(cmd, workDir string) string {
	c := strings.TrimSpace(cmd)
	if strings.HasPrefix(c, "./") && workDir != "" {
		abs := filepath.Join(workDir, strings.TrimPrefix(c, "./"))
		_ = os.Chmod(abs, 0755)
		return abs
	}
	if strings.HasSuffix(c, ".sh") && workDir != "" && !strings.HasPrefix(c, "/") {
		abs := filepath.Join(workDir, c)
		_ = os.Chmod(abs, 0755)
		return abs
	}
	return c
}

func renderSystemdUnit(s AppService) string {
	after := "network.target"
	if strings.TrimSpace(s.AfterDeps) != "" {
		after += " " + strings.TrimSpace(s.AfterDeps)
	}
	wants := ""
	if strings.TrimSpace(s.AfterDeps) != "" {
		wants = fmt.Sprintf("Wants=%s\n", strings.TrimSpace(s.AfterDeps))
	}

	workDir := s.WorkDir
	if workDir == "" {
		workDir = "/workspace"
	}

	execStart := resolveExecCmd(s.ExecStart, workDir)
	execStopLine := ""
	if strings.TrimSpace(s.ExecStop) != "" {
		execStopLine = fmt.Sprintf("ExecStop=%s\n", resolveExecCmd(s.ExecStop, workDir))
	}

	restart := s.RestartPolicy
	if restart == "" {
		restart = "always"
	}

	stype := s.ServiceType
	if stype == "" {
		if strings.Contains(s.ExecStart, ".sh") {
			stype = "forking"
		} else {
			stype = "simple"
		}
	}

	extraLines := ""
	if stype == "forking" {
		extraLines = "RemainAfterExit=yes\nTimeoutSec=60s\n"
	} else {
		extraLines = "KillMode=mixed\n"
	}

	return fmt.Sprintf(`[Unit]
Description=OpsWeb: %s
After=%s
%s
[Service]
Type=%s
%sWorkingDirectory=%s
ExecStart=%s
%sRestart=%s
RestartSec=5s
TimeoutStopSec=30s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, s.DisplayName, after, wants, stype, extraLines, workDir, execStart, execStopLine, restart)
}

func generateAndInstallSystemdUnit(s AppService) error {
	unitName := fmt.Sprintf("ops-%s.service", s.Name)
	content := renderSystemdUnit(s)
	destPath := fmt.Sprintf("/etc/systemd/system/%s", unitName)

	if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
		tmpPath := fmt.Sprintf("/tmp/%s", unitName)
		if writeErr := os.WriteFile(tmpPath, []byte(content), 0644); writeErr != nil {
			return writeErr
		}
		out, cpErr := run(10*time.Second, "sudo", "-n", "cp", tmpPath, destPath)
		_ = os.Remove(tmpPath)
		if cpErr != nil {
			return fmt.Errorf("install service unit error: %s (%s)", cpErr, out)
		}
	}

	_, _ = run(10*time.Second, "sudo", "-n", "systemctl", "daemon-reload")

	if s.AutoStart {
		_, _ = run(10*time.Second, "sudo", "-n", "systemctl", "enable", unitName)
	} else {
		_, _ = run(10*time.Second, "sudo", "-n", "systemctl", "disable", unitName)
	}
	return nil
}

func removeSystemdUnit(name string) error {
	unitName := fmt.Sprintf("ops-%s.service", name)
	_, _ = run(15*time.Second, "sudo", "-n", "systemctl", "stop", unitName)
	_, _ = run(10*time.Second, "sudo", "-n", "systemctl", "disable", unitName)
	destPath := fmt.Sprintf("/etc/systemd/system/%s", unitName)
	if err := os.Remove(destPath); err != nil {
		_, _ = run(10*time.Second, "sudo", "-n", "rm", "-f", destPath)
	}
	_, _ = run(10*time.Second, "sudo", "-n", "systemctl", "daemon-reload")
	return nil
}

func CoreAppServiceAction(idOrName string, action string) (string, error) {
	raw := GetRawServices()
	var target AppService
	found := false
	for _, s := range raw {
		if strconv.FormatInt(s.ID, 10) == idOrName || s.Name == idOrName {
			target = s
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("service %s not found", idOrName)
	}

	unitName := fmt.Sprintf("ops-%s.service", target.Name)
	unitFile := fmt.Sprintf("/etc/systemd/system/%s", unitName)

	if _, err := os.Stat(unitFile); err != nil {
		_ = generateAndInstallSystemdUnit(target)
	}

	switch action {
	case "start", "stop", "restart":
		out, err := run(30*time.Second, "sudo", "-n", "systemctl", action, unitName)
		if err != nil {
			if action == "start" && target.ExecStart != "" {
				wd := target.WorkDir
				if wd == "" {
					wd = "."
				}
				shOut, shErr := run(10*time.Second, "sh", "-c", fmt.Sprintf("cd %s && nohup %s >/dev/null 2>&1 &", wd, target.ExecStart))
				if shErr == nil {
					return "fallback started via shell", nil
				}
				return "", fmt.Errorf("systemctl %s failed: %v (%s), fallback shell failed: %v (%s)", action, err, out, shErr, shOut)
			}
			return "", fmt.Errorf("systemctl %s error: %v (%s)", action, err, out)
		}
		return fmt.Sprintf("%s ok", action), nil
	case "status":
		out, _ := run(10*time.Second, "systemctl", "status", unitName, "--no-pager")
		return out, nil
	default:
		return "", fmt.Errorf("unsupported action %s", action)
	}
}

func CoreSyncAllSystemdUnits() ([]string, error) {
	raw := GetRawServices()
	var synced []string
	var lastErr error
	for _, s := range raw {
		if err := generateAndInstallSystemdUnit(s); err == nil {
			synced = append(synced, s.Name)
		} else {
			lastErr = err
		}
	}
	if len(synced) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return synced, nil
}

func CoreBatchAction(action string) (map[string]any, error) {
	services := CoreGetAppServices()
	if action != "start" && action != "restart" && action != "stop" {
		return nil, fmt.Errorf("unsupported batch action: %s", action)
	}

	groups := map[int][]AppService{}
	for _, s := range services {
		lvl := s.Level
		if lvl <= 0 {
			lvl = 3
		}
		groups[lvl] = append(groups[lvl], s)
	}

	var levels []int
	for lvl := range groups {
		levels = append(levels, lvl)
	}
	if action == "stop" {
		sort.Sort(sort.Reverse(sort.IntSlice(levels)))
	} else {
		sort.Ints(levels)
	}

	results := []map[string]any{}
	for _, lvl := range levels {
		svcs := groups[lvl]
		for _, s := range svcs {
			out, err := CoreAppServiceAction(s.Name, action)
			res := map[string]any{
				"name":    s.Name,
				"level":   lvl,
				"success": err == nil,
				"output":  out,
			}
			if err != nil {
				res["error"] = err.Error()
			}
			results = append(results, res)
		}
		if action != "stop" {
			time.Sleep(1200 * time.Millisecond)
		}
	}

	return map[string]any{
		"action":  action,
		"results": results,
		"total":   len(results),
	}, nil
}

func CoreRebootSelfCheck() RebootSelfCheckResult {
	hi := getHostInfo()
	services := CoreGetAppServices()

	healthy := 0
	unhealthy := 0
	lvlSummary := map[int]int{}

	for _, s := range services {
		lvlSummary[s.Level]++
		isGood := false
		if s.Status == "active" {
			isGood = true
		} else if s.PortListening {
			isGood = true
		}
		if isGood {
			healthy++
		} else {
			unhealthy++
		}
	}

	upFmt := ""
	d := hi.Uptime / 86400
	h := (hi.Uptime % 86400) / 3600
	m := (hi.Uptime % 3600) / 60
	if d > 0 {
		upFmt = fmt.Sprintf("%d天 %d小时 %d分钟", d, h, m)
	} else if h > 0 {
		upFmt = fmt.Sprintf("%d小时 %d分钟", h, m)
	} else {
		upFmt = fmt.Sprintf("%d分钟", m)
	}

	return RebootSelfCheckResult{
		UptimeSec:       hi.Uptime,
		UptimeFormatted: upFmt,
		TotalCount:      len(services),
		HealthyCount:    healthy,
		UnhealthyCount:  unhealthy,
		AllHealthy:      unhealthy == 0,
		LevelSummary:    lvlSummary,
		Services:        services,
	}
}

func CoreExportProductionBundle() ([]byte, error) {
	services := GetRawServices()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	installScript := `#!/usr/bin/env bash
set -e
echo "=== Installing OpsWeb Production Units ==="
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
sudo cp "${DIR}"/units/*.service /etc/systemd/system/
sudo systemctl daemon-reload
`
	checkScript := `#!/usr/bin/env bash
echo "=== Production Health Check ==="
`

	for _, s := range services {
		unitName := fmt.Sprintf("ops-%s.service", s.Name)
		content := renderSystemdUnit(s)
		tarAddFile(tw, "units/"+unitName, []byte(content))

		if s.AutoStart {
			installScript += fmt.Sprintf("sudo systemctl enable %s\n", unitName)
			installScript += fmt.Sprintf("sudo systemctl restart %s\n", unitName)
		}
		checkScript += fmt.Sprintf("echo -n '%s: '\nsystemctl is-active %s\n", s.Name, unitName)
	}

	installScript += "echo '=== All Services Installed & Started ==='\n"
	checkScript += "echo '=== Health Check Complete ==='\n"

	tarAddFile(tw, "install-services.sh", []byte(installScript))
	tarAddFile(tw, "check-health.sh", []byte(checkScript))

	logrotateRule := CoreRenderLogrotateRules(services)
	tarAddFile(tw, "opsweb-logrotate.conf", []byte(logrotateRule))

	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes(), nil
}

func tarAddFile(tw *tar.Writer, name string, content []byte) {
	cleanContent := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	hdr := &tar.Header{
		Name:    name,
		Mode:    0755,
		Size:    int64(len(cleanContent)),
		ModTime: time.Now(),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(cleanContent)
}

func AppServicesListHandler(c *gin.Context) {
	ok(c, CoreGetAppServices())
}

func AppServiceCreateHandler(c *gin.Context) {
	var s AppService
	if err := c.ShouldBindJSON(&s); err != nil {
		fail(c, err.Error())
		return
	}
	id, err := CoreCreateAppService(s)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, gin.H{"id": id})
}

func AppServiceUpdateHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, "invalid id")
		return
	}
	var s AppService
	if err := c.ShouldBindJSON(&s); err != nil {
		fail(c, err.Error())
		return
	}
	if err := CoreUpdateAppService(id, s); err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, "updated")
}

func AppServiceDeleteHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, "invalid id")
		return
	}
	if err := CoreDeleteAppService(id); err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, "deleted")
}

func AppServiceActionHandler(c *gin.Context) {
	nameOrID := c.Param("name")
	action := c.Param("action")
	out, err := CoreAppServiceAction(nameOrID, action)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, out)
}

func AppServicesBatchActionHandler(c *gin.Context) {
	action := c.Param("action")
	res, err := CoreBatchAction(action)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, res)
}

func AppServicesSyncSystemdHandler(c *gin.Context) {
	synced, err := CoreSyncAllSystemdUnits()
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, gin.H{"synced": synced})
}

func AppServicesSelfCheckHandler(c *gin.Context) {
	ok(c, CoreRebootSelfCheck())
}

func AppServicesExportBundleHandler(c *gin.Context) {
	data, err := CoreExportProductionBundle()
	if err != nil {
		fail(c, err.Error())
		return
	}
	c.Header("Content-Disposition", "attachment; filename=production-ops-bundle.tar.gz")
	c.Data(http.StatusOK, "application/gzip", data)
}
