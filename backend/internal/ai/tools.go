package ai

import (
	"fmt"
	"strings"
	"time"

	"opsweb/internal/collect"
)

// toolSpec is one assistant-callable capability.
type toolSpec struct {
	Name        string
	Description string
	Params      map[string]Param
	Level       safetyLevel
	Exec        func(args map[string]any) (string, error)
}

// Param describes one tool argument.
type Param struct {
	Type        string
	Description string
	Required    bool
}

func pStr(desc string, required bool) Param {
	return Param{Type: "string", Description: desc, Required: required}
}

func pInt(desc string) Param {
	return Param{Type: "integer", Description: desc}
}

// toolRegistry: name -> spec, built once.
var toolRegistry = map[string]toolSpec{}

func register(specs ...toolSpec) {
	for _, s := range specs {
		toolRegistry[s.Name] = s
	}
}

func argStr(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func argInt(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return def
}

func init() {
	register(
		toolSpec{
			Name:        "get_overview",
			Description: "获取服务器总览：主机名、系统、CPU/内存/负载、容器数量统计",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				o := collect.CoreOverviewCached()
				return fmt.Sprintf("主机: %s (%s %s)\nCPU: %s %d核, 当前 %.1f%%\n内存: %.1f%% (used %dMB/total %dMB)\nSwap: %dMB used\n负载: %.2f %.2f %.2f\n容器: %d 中 %d 个运行中\n开机时长: %d 秒",
					o.Hostname, o.OS, o.Platform, o.CPUModel, o.CPUCores, o.CPUPercent,
					o.MemPercent, o.MemUsed/1024/1024, o.MemTotal/1024/1024, o.SwapUsed/1024/1024, //nolint
					o.Load1, o.Load5, o.Load15, o.ContainerCnt, o.RunningCnt, o.UptimeSec), nil
			},
		},
		toolSpec{
			Name:        "get_disks",
			Description: "获取磁盘分区使用率列表",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				list, err := collect.CoreDisks()
				if err != nil {
					return "", err
				}
				var b strings.Builder
				for _, d := range list {
					fmt.Fprintf(&b, "%s (%s) 挂载 %s: %.1f%% 已用 (%dMB free)\n", d.Device, d.FSType, d.Mountpoint, d.UsedPct, d.Free/1024/1024)
				}
				return b.String(), nil
			},
		},
		toolSpec{
			Name:        "get_load_history",
			Description: "获取最近 N 天 (1-30) 的 CPU/内存百分比历史采样",
			Params:      map[string]Param{"days": pInt("天数 1-30，默认 1")},
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				days := argInt(args, "days", 1)
				if days < 1 || days > 30 {
					days = 1
				}
				cpu, mem := collect.CoreLoadHistory(days)
				return summarizeSamples(cpu, mem), nil
			},
		},
		toolSpec{
			Name:        "get_containers",
			Description: "获取所有 Docker 容器列表（名称、镜像、状态）",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				list, err := collect.CoreContainers()
				if err != nil {
					return "", err
				}
				var b strings.Builder
				for _, c := range list {
					fmt.Fprintf(&b, "%s [%s] %s (%s)\n", c.Name, c.State, c.Image, c.Status)
				}
				return b.String(), nil
			},
		},
		toolSpec{
			Name:        "get_container_logs",
			Description: "查看指定容器的最近日志",
			Params: map[string]Param{
				"name": pStr("容器名", true),
				"tail": pInt("行数，默认 100"),
			},
			Level: levelRead,
			Exec: func(args map[string]any) (string, error) {
				out, err := collect.CoreContainerLogs(argStr(args, "name"), fmt.Sprintf("%d", argInt(args, "tail", 100)))
				if err != nil {
					return "", err
				}
				return truncate(out, 6000), nil
			},
		},
		toolSpec{
			Name:        "get_images",
			Description: "获取 Docker 镜像列表",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				list, err := collect.CoreImages()
				if err != nil {
					return "", err
				}
				var b strings.Builder
				for _, im := range list {
					fmt.Fprintf(&b, "%s:%s %s\n", im.Repository, im.Tag, im.Size)
				}
				return b.String(), nil
			},
		},
		toolSpec{
			Name:        "get_projects",
			Description: "获取业务项目列表（部署路径、容器构成、磁盘占用、最近部署）",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				var b strings.Builder
				for _, p := range collect.CoreProjects() {
					fmt.Fprintf(&b, "%s (%s) 容器: %s\n磁盘: %dMB 最近备份: %s\n", p.Name, p.Kind, strings.Join(p.Containers, ", "), p.DiskUsage, p.LastDeploy)
				}
				return b.String(), nil
			},
		},
		toolSpec{
			Name:        "get_deploy_status",
			Description: "查看指定项目最近一次部署的输出日志",
			Params:      map[string]Param{"name": pStr("项目名（见 get_projects）", true)},
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				return truncate(collect.CoreDeployStatus(argStr(args, "name")), 6000), nil
			},
		},
		toolSpec{
			Name:        "get_cicd_summary",
			Description: "获取 CI/CD 组件状态：GitLab、Runner、Nacos 是否在线",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				s := collect.CoreCICDSummary()
				return fmt.Sprintf("GitLab: %v (HTTP %s)\nRunner: %s %s\nNacos: %v", s.GitlabUp, s.GitlabDetail, s.RunnerState, s.RunnerDetail, s.NacosUp), nil
			},
		},
		toolSpec{
			Name:        "get_pipelines",
			Description: "获取最近的 CI/CD 构建任务结果",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				var b strings.Builder
				for _, j := range collect.CorePipelines() {
					fmt.Fprintf(&b, "#%d %s %s 用时%s 完成于 %s\n", j.JobID, j.Project, j.Status, j.Duration, j.FinishedAt)
				}
				return b.String(), nil
			},
		},
		toolSpec{
			Name:        "get_services",
			Description: "获取被关注的 systemd 服务状态",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				var b strings.Builder
				for _, sv := range collect.CoreServices() {
					fmt.Fprintf(&b, "%s [%s] %s\n", sv.Name, sv.Active, sv.Desc)
				}
				return b.String(), nil
			},
		},
		toolSpec{
			Name:        "get_alerts",
			Description: "获取当前活跃告警与最近告警事件",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				a := collect.CoreAlerts()
				return fmt.Sprintf("活跃告警: %v\n最近事件: %v", a["active"], a["recent"]), nil
			},
		},
		toolSpec{
			Name:        "get_processes",
			Description: "获取 CPU 占用最高的 50 个进程",
			Level:       levelRead,
			Exec: func(args map[string]any) (string, error) {
				var b strings.Builder
				for _, pr := range collect.CoreProcesses() {
					fmt.Fprintf(&b, "PID %d %s CPU %.1f%% MEM %.1f%% RSS %dMB\n", pr.Pid, pr.Name, pr.CPUPerc, pr.MemPerc, pr.RSS)
				}
				return b.String(), nil
			},
		},
		toolSpec{
			Name:        "get_journal",
			Description: "查询 systemd journal 日志",
			Params: map[string]Param{
				"unit":  pStr("服务 unit 名，如 nginx（可选）", false),
				"since": pStr("起始时间，如 \"2026-02-01 10:00:00\" 或 \"1 hour ago\"（可选）", false),
				"grep":  pStr("过滤关键字（可选）", false),
				"tail":  pInt("行数，默认 200"),
			},
			Level: levelRead,
			Exec: func(args map[string]any) (string, error) {
				data, err := collect.CoreJournal(argStr(args, "unit"), argStr(args, "since"), argStr(args, "grep"), fmt.Sprintf("%d", argInt(args, "tail", 200)))
				if err != nil {
					return "", err
				}
				lines, _ := data["lines"].([]string)
				return truncate(strings.Join(lines, "\n"), 6000), nil
			},
		},
		toolSpec{
			Name:        "get_log_file",
			Description: "读取白名单目录下日志文件的尾部内容",
			Params: map[string]Param{
				"path": pStr("文件绝对路径（限 /workspace/*/logs、/var/log）", true),
				"tail": pInt("行数，默认 200"),
			},
			Level: levelRead,
			Exec: func(args map[string]any) (string, error) {
				data, err := collect.CoreLogFile(argStr(args, "path"), fmt.Sprintf("%d", argInt(args, "tail", 200)))
				if err != nil {
					return "", err
				}
				lines, _ := data["lines"].([]string)
				return truncate(strings.Join(lines, "\n"), 6000), nil
			},
		},
		toolSpec{
			Name:        "container_action",
			Description: "对容器执行操作：start/stop/restart/kill/pause/unpause/remove。remove 强制删除容器，属于危险操作",
			Params: map[string]Param{
				"name":   pStr("容器名", true),
				"action": pStr("start|stop|restart|kill|pause|unpause|remove", true),
			},
			Level: levelWrite,
			Exec: func(args map[string]any) (string, error) {
				return collect.CoreContainerAction(argStr(args, "name"), argStr(args, "action"))
			},
		},
		toolSpec{
			Name:        "service_action",
			Description: "对 systemd 服务执行 start/stop/restart 操作",
			Params: map[string]Param{
				"name":   pStr("服务名（不带 .service 后缀）", true),
				"action": pStr("start|stop|restart", true),
			},
			Level: levelWrite,
			Exec: func(args map[string]any) (string, error) {
				return collect.CoreServiceAction(argStr(args, "name"), argStr(args, "action"))
			},
		},
		toolSpec{
			Name:        "deploy_project",
			Description: "触发指定项目的重新部署（构建+发布，最长 10 分钟）",
			Params:      map[string]Param{"name": pStr("项目名（见 get_projects）", true)},
			Level:       levelWrite,
			Exec: func(args map[string]any) (string, error) {
				return collect.CoreProjectDeploy(argStr(args, "name"))
			},
		},
		toolSpec{
			Name:        "run_shell",
			Description: "在服务器上执行一条 shell 命令。聊天时仅创建待批请求；用户批准后由审批接口调用本工具实际执行。命中黑名单的命令直接拒绝。输出最多截取 8000 字符",
			Params:      map[string]Param{"command": pStr("要执行的 shell 命令", true)},
			Level:       levelShell,
			Exec: func(args map[string]any) (string, error) {
				cmd := argStr(args, "command")
				// approval already granted by the user; blacklist stays a hard gate
				if _, blocked, _ := classifyShell(cmd); blocked != "" {
					return "", fmt.Errorf("已被安全策略硬拒绝（%s）", blocked)
				}
				if err := validShellBinary(cmd); err != nil {
					return "", err
				}
				out, err := runGuarded(cmd)
				if err != nil {
					return truncate(out, 4000) + "\n[错误] " + err.Error(), nil
				}
				return truncate(out, 8000), nil
			},
		},
	)
}

// summarizeSamples reduces history samples to a compact text summary.
func summarizeSamples(cpu, mem []collect.Sample) string {
	if len(cpu) == 0 {
		return "（无历史数据）"
	}
	avg, max := stats(cpu)
	avgM, maxM := stats(mem)
	first := time.Unix(cpu[0].T, 0).Format("01-02 15:04")
	last := time.Unix(cpu[len(cpu)-1].T, 0).Format("01-02 15:04")
	return fmt.Sprintf("采样区间 %s ~ %s（%d 点）\nCPU: 平均 %.1f%% 峰值 %.1f%%\n内存: 平均 %.1f%% 峰值 %.1f%%", first, last, len(cpu), avg, max, avgM, maxM)
}

func stats(s []collect.Sample) (avg, max float64) {
	var sum float64
	for _, v := range s {
		sum += v.V
		if v.V > max {
			max = v.V
		}
	}
	if len(s) > 0 {
		return sum / float64(len(s)), max
	}
	return 0, 0
}

// toolDefs converts the registry to OpenAI tool schemas.
func toolDefs() []chatTool {
	out := make([]chatTool, 0, len(toolRegistry))
	for _, s := range toolRegistry {
		props := map[string]any{}
		var req []string
		for name, p := range s.Params {
			props[name] = map[string]any{"type": p.Type, "description": p.Description}
			if p.Required {
				req = append(req, name)
			}
		}
		out = append(out, chatTool{
			Type: "function",
			Function: chatToolDefinition{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  map[string]any{"type": "object", "properties": props, "required": req},
			},
		})
	}
	return out
}
