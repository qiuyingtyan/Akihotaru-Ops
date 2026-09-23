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
	if strings.ContainsAny(name, " ;|&$`\\\"'") || strings.HasPrefix(name, "-") {
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

var projectRoots = []string{"/workspace"}

func ProjectsHandler(c *gin.Context) {
	ok(c, CoreProjects())
}

func CoreProjects() []Project {
	out, err := run(10*time.Second, "docker", "ps", "-a", "--format", "{{.Names}}\t{{.State}}\t{{.Status}}\t{{.Label \"com.docker.compose.project\"}}\t{{.Label \"com.docker.compose.project.working_dir\"}}")
	if err != nil {
		return []Project{}
	}

	projectMap := map[string]*Project{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		cn := f[0]
		state := f[1]
		status := f[2]
		composeProj := ""
		workDir := ""
		if len(f) >= 4 {
			composeProj = strings.TrimSpace(f[3])
		}
		if len(f) >= 5 {
			workDir = strings.TrimSpace(f[4])
		}

		projName := composeProj
		if projName == "" {
			projName = "系统独立容器"
		}

		p, exists := projectMap[projName]
		if !exists {
			p = &Project{
				Name:       projName,
				Kind:       "docker-compose",
				Path:       workDir,
				Containers: []string{},
			}
			if workDir != "" {
				p.DiskUsage = dirSizeMB(workDir)
				p.LastDeploy = lastBackupTime(workDir)
			}
			projectMap[projName] = p
		}
		p.Containers = append(p.Containers, cn+" ["+state+"|"+status+"]")
	}

	list := []Project{}
	for _, p := range projectMap {
		list = append(list, *p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

func dirSizeMB(path string) int64 {
	if path == "" {
		return 0
	}
	out, err := run(20*time.Second, "sh", "-c", "du -sm "+path+" 2>/dev/null | cut -f1")
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return n
}

func lastBackupTime(path string) string {
	if path == "" {
		return ""
	}
	out, _ := run(5*time.Second, "sh", "-c", "ls -1 "+path+"/backup 2>/dev/null | tail -1")
	return strings.TrimSpace(out)
}

var projectDeploy = map[string]string{}

var deployMu sync.Mutex
var deployBusy = false

func ProjectAction(c *gin.Context) (string, error) {
	return CoreProjectDeploy(c.Param("name"))
}

func CoreProjectDeploy(name string) (string, error) {
	script := projectDeploy[name]
	if script == "" {
		// 动态检测工作目录下的 docker-compose.yml 或 deploy.sh
		projects := CoreProjects()
		for _, p := range projects {
			if p.Name == name && p.Path != "" {
				if _, err := os.Stat(p.Path + "/docker-compose.yml"); err == nil {
					script = "compose:" + p.Path + "/docker-compose.yml"
					break
				}
				if _, err := os.Stat(p.Path + "/deploy.sh"); err == nil {
					script = p.Path + "/deploy.sh"
					break
				}
			}
		}
	}
	if script == "" {
		return "", fmt.Errorf("未找到项目 %s 的部署脚本或 compose 配置", name)
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
