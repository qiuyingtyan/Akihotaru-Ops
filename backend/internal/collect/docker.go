package collect

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Container struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	State     string   `json:"state"`
	Status    string   `json:"status"`
	Ports     []string `json:"ports"`
	CreatedAt string   `json:"createdAt"`
	ID        string   `json:"id"`
}

func ContainersHandler(c *gin.Context) {
	list, err := CoreContainers()
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, list)
}

func CoreContainers() ([]Container, error) {
	out, err := run(10*time.Second, "docker", "ps", "-a", "--no-trunc",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.State}}\t{{.Status}}\t{{.Ports}}\t{{.CreatedAt}}")
	if err != nil {
		return nil, err
	}
	list := []Container{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 7 {
			continue
		}
		ports := []string{}
		if pf := strings.TrimSpace(f[5]); pf != "" {
			ports = strings.Split(pf, ", ")
		}
		list = append(list, Container{
			ID: f[0][:12], Name: f[1], Image: f[2], State: f[3],
			Status: f[4], Ports: ports, CreatedAt: f[6],
		})
	}
	return list, nil
}

func ContainerAction(c *gin.Context) (string, error) {
	return CoreContainerAction(c.Param("name"), c.Param("action"))
}

func CoreContainerAction(name, action string) (string, error) {
	var args []string
	switch action {
	case "start", "stop", "restart", "kill", "pause", "unpause":
		args = []string{action, name}
	case "remove":
		args = []string{"rm", "-f", name}
	default:
		return "", fmt.Errorf("unsupported action: %s", action)
	}
	if strings.ContainsAny(name, " ;|&$`\\\"'") {
		return "", fmt.Errorf("invalid container name")
	}
	out, err := run(30*time.Second, "docker", args...)
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, out)
	}
	return out, nil
}

func ContainerLogsHandler(c *gin.Context) {
	out, err := CoreContainerLogs(c.Param("name"), c.DefaultQuery("tail", "200"))
	if err != nil && out == "" {
		fail(c, err.Error())
		return
	}
	ok(c, out)
}

func CoreContainerLogs(name, tail string) (string, error) {
	return run(15*time.Second, "docker", "logs", "--tail", tail, name)
}

type Image struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}

func ImagesHandler(c *gin.Context) {
	list, err := CoreImages()
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, list)
}

func CoreImages() ([]Image, error) {
	out, err := run(15*time.Second, "docker", "images", "--format",
		"{{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}")
	if err != nil {
		return nil, err
	}
	list := []Image{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		list = append(list, Image{Repository: f[0], Tag: f[1], Size: f[2], Created: f[3]})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Repository < list[j].Repository })
	return list, nil
}

type Project struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Kind       string   `json:"kind"`
	Containers []string `json:"containers"`
	DiskUsage  int64    `json:"diskUsageMB"`
	LastDeploy string   `json:"lastDeploy,omitempty"`
}

var projectRoots = []string{"/workspace/baq-test", "/workspace/szx-test", "/workspace/ljgw", "/workspace/YangQingDe"}

var knownProjects = map[string][]string{
	"办案区(baq-test)":    {"baq-main-service", "baq-video-service", "baq-receiver-service", "baq-live", "baq-zlm", "baq-test-nginx", "baq-gitlab-runner", "mysql-baq", "redis-baq", "pgsql-baq", "rabbitmq", "gitlab", "nacos"},
	"三中心(szx-test)":    {"szx-glzx-gateway", "szx-glzx-ag", "szx-glzx-baq", "szx-glzx-sacw", "szx-glzx-clean", "szx-glzx-converge", "szx-agzx-service", "szx-agzx-cabinet", "szx-sacw-service", "szx-test-nginx"},
	"vocedu平台":          {"vocedu-gateway", "vocedu-user-service", "vocedu-student-web", "vocedu-admin-web", "vocedu-mysql", "vocedu-redis", "vocedu-minio"},
	"VirtualLedgerMart": {"vlm-api", "vlm-web", "vlm-postgres"},
}

func ProjectsHandler(c *gin.Context) {
	ok(c, CoreProjects())
}

func CoreProjects() []Project {
	out, err := run(10*time.Second, "docker", "ps", "-a", "--format", "{{.Names}}\t{{.State}}\t{{.Status}}")
	if err != nil {
		return []Project{}
	}
	stateMap := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) >= 2 {
			stateMap[f[0]] = f[1] + "|" + strings.Join(f[2:], "\t")
		}
	}
	list := []Project{}
	for name, cons := range knownProjects {
		p := Project{Name: name, Kind: "docker-compose", Containers: []string{}}
		switch name {
		case "办案区(baq-test)":
			p.Path = "/workspace/baq-test"
		case "三中心(szx-test)":
			p.Path = "/workspace/szx-test"
		case "vocedu平台":
			p.Path = "/workspace/ljgw/vocedu_integrated_platform"
		case "VirtualLedgerMart":
			p.Path = "/workspace/ljgw/Virtual_Ledger_Mart"
		}
		running := 0
		for _, cn := range cons {
			p.Containers = append(p.Containers, cn+" ["+stateMap[cn]+"]")
			if strings.HasPrefix(stateMap[cn], "running") || strings.HasPrefix(stateMap[cn], "healthy") {
				running++
			}
		}
		p.DiskUsage = dirSizeMB(p.Path)
		p.LastDeploy = lastBackupTime(p.Path)
		_ = running
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

func dirSizeMB(path string) int64 {
	out, err := run(20*time.Second, "sh", "-c", "du -sm "+path+" 2>/dev/null | cut -f1")
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return n
}

func lastBackupTime(path string) string {
	out, _ := run(5*time.Second, "sh", "-c", "ls -1 "+path+"/backup 2>/dev/null | tail -1")
	return strings.TrimSpace(out)
}

var projectDeploy = map[string]string{
	"办案区(baq-test)":    "/workspace/baq-test/.deploy/deploy-remote.sh",
	"三中心(szx-test)":    "/workspace/szx-test/.deploy/deploy-remote.sh",
	"vocedu平台":          "compose:/workspace/ljgw/vocedu_integrated_platform/docker-compose.yml",
	"VirtualLedgerMart": "compose:/workspace/ljgw/Virtual_Ledger_Mart/docker-compose.yml",
}

var deployMu sync.Mutex
var deployBusy = false

func ProjectAction(c *gin.Context) (string, error) {
	return CoreProjectDeploy(c.Param("name"))
}

func CoreProjectDeploy(name string) (string, error) {
	script, ok := projectDeploy[name]
	if !ok {
		return "", fmt.Errorf("no deploy method for project: %s", name)
	}
	deployMu.Lock()
	if deployBusy {
		deployMu.Unlock()
		return "", fmt.Errorf("另一个部署任务正在进行中")
	}
	deployBusy = true
	deployMu.Unlock()
	defer func() {
		deployMu.Lock()
		deployBusy = false
		deployMu.Unlock()
	}()

	go collectDeployOutput(name)
	if strings.HasPrefix(script, "compose:") {
		file := strings.TrimPrefix(script, "compose:")
		if strings.ContainsAny(file, " ;|&$`\\\"'") {
			return "", fmt.Errorf("invalid compose file")
		}
		out, err := run(10*time.Minute, "docker", "compose", "-f", file, "up", "-d", "--build")
		finishDeployOutput(name, out, err)
		return out, err
	}
	if _, err := os.Stat(script); err != nil {
		return "", fmt.Errorf("deploy script not found: %s", script)
	}
	out, err := run(10*time.Minute, "bash", script)
	finishDeployOutput(name, out, err)
	return out, err
}

var deployState = struct {
	sync.Mutex
	outputs map[string]string
}{outputs: map[string]string{}}

func collectDeployOutput(name string) {
	deployState.Lock()
	deployState.outputs[name] = "部署开始: " + time.Now().Format("01-02 15:04:05") + "\n"
	deployState.Unlock()
}

func finishDeployOutput(name, out string, err error) {
	status := "成功"
	if err != nil {
		status = "失败: " + err.Error()
	}
	deployState.Lock()
	deployState.outputs[name] += out + "\n部署" + status + "\n"
	deployState.Unlock()
}

func DeployStatusHandler(c *gin.Context) {
	ok(c, CoreDeployStatus(c.Param("name")))
}

func CoreDeployStatus(name string) string {
	deployState.Lock()
	defer deployState.Unlock()
	out := deployState.outputs[name]
	if out == "" {
		out = "（无进行中或最近的部署记录）"
	}
	return out
}
