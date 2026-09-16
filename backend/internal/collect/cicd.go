package collect

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Service struct {
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Active  string `json:"active"`
	Sub     string `json:"sub"`
	Enabled string `json:"enabled"`
}

var interestingServiceKeywords = []string{
	"frpc", "keepalived", "docker", "containerd", "gitlab", "runner", "nacos",
	"pg-auto-promote", "nginx", "mysql", "postgres", "redis", "rabbitmq", "prometheus", "grafana",
}

func ServicesHandler(c *gin.Context) {
	out2, err := run(10*time.Second, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	if err != nil {
		fail(c, err.Error())
		return
	}
	var list []Service
	for _, line := range strings.Split(out2, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ".service")
		desc := strings.Join(fields[4:], " ")
		match := false
		for _, kw := range interestingServiceKeywords {
			if strings.Contains(name, kw) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		state := "unknown"
		if strings.Contains(line, " active ") {
			state = "active"
		} else if strings.Contains(line, " failed ") {
			state = "failed"
		} else if strings.Contains(line, " inactive ") {
			state = "inactive"
		}
		list = append(list, Service{Name: name, Desc: desc, Active: state, Sub: fields[3]})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	ok(c, list)
}

func ServiceAction(c *gin.Context) (string, error) {
	name := c.Param("name")
	action := c.Param("action")
	switch action {
	case "start", "stop", "restart", "status":
	default:
		return "", fmt.Errorf("unsupported action: %s", action)
	}
	// sanitize name
	if strings.ContainsAny(name, " ;|&$`\\\"'") {
		return "", fmt.Errorf("invalid service name")
	}
	out, err := run(30*time.Second, "sudo", "-n", "systemctl", action, name+".service")
	if err != nil {
		if action == "status" && out != "" {
			return out, nil
		}
		return "", fmt.Errorf("%s: %s", err, out)
	}
	return out, nil
}

func RunnerLogsHandler(c *gin.Context) {
	tail := c.DefaultQuery("tail", "100")
	out, _ := run(15*time.Second, "docker", "logs", "--tail", tail, "baq-gitlab-runner")
	ok(c, out)
}

type CICDSummary struct {
	GitlabUp     bool     `json:"gitlabUp"`
	GitlabDetail string   `json:"gitlabDetail"`
	RunnerState  string   `json:"runnerState"`
	RunnerDetail string   `json:"runnerDetail"`
	NacosUp      bool     `json:"nacosUp"`
	CIJobs       []CIJob  `json:"ciJobs"`
	DeployHooks  []string `json:"deployHooks"`
}

type CIJob struct {
	Name       string `json:"name"`
	ConfigFile string `json:"configFile"`
	Stages     string `json:"stages"`
}

func CICDSummaryHandler(c *gin.Context) {
	s := CICDSummary{CIJobs: []CIJob{}, DeployHooks: []string{}}
	var wg syncWaitGroup

	wg.Go(func() {
		out, err := run(5*time.Second, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://127.0.0.1:9980/users/sign_in")
		s.GitlabUp = err == nil && strings.TrimSpace(out) == "200"
		s.GitlabDetail = strings.TrimSpace(out)
	})
	wg.Go(func() {
		out, _ := run(5*time.Second, "docker", "inspect", "baq-gitlab-runner", "--format", "{{.State.Status}} {{.State.Health.Status}}")
		f := strings.Fields(out)
		if len(f) > 0 {
			s.RunnerState = f[0]
			if len(f) > 1 {
				s.RunnerDetail = f[1]
			}
		}
	})
	wg.Go(func() {
		out, err := run(5*time.Second, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://127.0.0.1:8848/nacos/")
		s.NacosUp = err == nil && strings.TrimSpace(out) == "200"
	})
	wg.Go(func() {
		for _, root := range []string{"/workspace/baq-test", "/workspace/szx-test"} {
			entries, _ := os.ReadDir(root + "/.ci")
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), ".yaml") {
					data, _ := os.ReadFile(root + "/.ci/" + e.Name())
					s.CIJobs = append(s.CIJobs, CIJob{Name: e.Name(), ConfigFile: root + "/.ci/" + e.Name(), Stages: firstLines(string(data), 20)})
				}
			}
			if hooks := findScripts(root + "/.deploy"); len(hooks) > 0 {
				s.DeployHooks = append(s.DeployHooks, hooks...)
			}
			if hooks := findScripts(root + "/.ci"); len(hooks) > 0 {
				s.DeployHooks = append(s.DeployHooks, hooks...)
			}
		}
		s.DeployHooks = append(s.DeployHooks, findScripts("/workspace/YangQingDe")...)
	})
	wg.Wait()
	ok(c, s)
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func findScripts(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".sh") || strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), ".yaml")) {
			out = append(out, dir+"/"+e.Name())
		}
	}
	return out
}
