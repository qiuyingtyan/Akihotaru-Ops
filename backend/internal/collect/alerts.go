package collect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type AlertRule struct {
	Metric string  `json:"metric"`
	Op     string  `json:"op"`
	Value  float64 `json:"value"`
	Level  string  `json:"level"`
}

type AlertEvent struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Metric  string    `json:"metric"`
	Value   float64   `json:"value"`
	Detail  string    `json:"detail"`
	Active  bool      `json:"active"`
}

var defaultRules = []AlertRule{
	{Metric: "diskPercent", Op: ">", Value: 90, Level: "critical"},
	{Metric: "diskPercent", Op: ">", Value: 80, Level: "warning"},
	{Metric: "memPercent", Op: ">", Value: 90, Level: "critical"},
	{Metric: "memPercent", Op: ">", Value: 80, Level: "warning"},
	{Metric: "load1", Op: ">", Value: 16, Level: "warning"},
	{Metric: "containersUnhealthy", Op: ">", Value: 0, Level: "critical"},
	{Metric: "servicesFailed", Op: ">", Value: 0, Level: "critical"},
}

type alertsState struct {
	mu       sync.Mutex
	active   map[string]AlertEvent
	recent   []AlertEvent
	webhook  string
	interval time.Duration
	// webhook cooldown: metric -> last notify time
	lastNotify map[string]time.Time
}

var alerts = alertsState{
	active:     map[string]AlertEvent{},
	webhook:    os.Getenv("OPSWEB_WEBHOOK"),
	interval:   30 * time.Second,
	lastNotify: map[string]time.Time{},
}

// StartAlertChecker evaluates rules periodically and notifies via webhook.
func StartAlertChecker() {
	go func() {
		for {
			checkAlerts()
			time.Sleep(alerts.interval)
		}
	}()
}

func evalRule(v float64, r AlertRule) bool {
	switch r.Op {
	case ">":
		return v > r.Value
	case "<":
		return v < r.Value
	}
	return false
}

func checkAlerts() {
	type metricVal struct {
		key   string
		val   float64
		label string
	}
	var ms []metricVal

	vm, _ := getMemStat()
	l1, _, _ := getLoadAvg()
	for _, d := range readDiskUsage() {
		if d.Mountpoint == "/" || d.Mountpoint == "/workspace" {
			ms = append(ms, metricVal{"diskPercent:" + d.Mountpoint, d.UsedPct,
				"磁盘 " + d.Mountpoint + " 使用率"})
		}
	}
	ms = append(ms, metricVal{"memPercent", vm.Percent, "内存使用率"})
	ms = append(ms, metricVal{"load1", l1, "系统负载 load1"})
	ms = append(ms, metricVal{"containersUnhealthy", float64(countUnhealthyContainers()), "异常容器数"})
	ms = append(ms, metricVal{"servicesFailed", float64(countFailedServices()), "失败系统服务数"})

	var fired []AlertEvent
	alerts.mu.Lock()
	seen := map[string]bool{}
	for _, m := range ms {
		seen[m.key] = true
		for _, r := range defaultRules {
			if !strings.HasPrefix(m.key, r.Metric) {
				continue
			}
			if evalRule(m.val, r) {
				if prev, ok := alerts.active[m.key]; ok {
					prev.Value = m.val
					alerts.active[m.key] = prev
				} else {
					ev := AlertEvent{Time: time.Now(), Level: r.Level,
						Metric: m.key, Value: m.val, Detail: m.label, Active: true}
					alerts.active[m.key] = ev
					alerts.recent = append(alerts.recent, ev)
					fired = append(fired, ev)
				}
			} else if prev, ok := alerts.active[m.key]; ok {
				prev.Active = false
				prev.Time = time.Now()
				alerts.recent = append(alerts.recent, prev)
				delete(alerts.active, m.key)
				fired = append(fired, prev)
			}
		}
	}
	// drop actives whose metric vanished
	for k := range alerts.active {
		if !seen[k] {
			delete(alerts.active, k)
		}
	}
	if len(alerts.recent) > 200 {
		alerts.recent = alerts.recent[len(alerts.recent)-200:]
	}
	recentCp := make([]AlertEvent, len(alerts.recent))
	copy(recentCp, alerts.recent)
	activeCnt := len(alerts.active)
	alerts.mu.Unlock()

	if len(fired) > 0 && alerts.webhook != "" {
		// cooldown: only notify for metrics not notified in the last 5 minutes
		now := time.Now()
		var todo []AlertEvent
		for _, e := range fired {
			if last, ok := alerts.lastNotify[e.Metric]; !ok || now.Sub(last) >= 5*time.Minute {
				alerts.lastNotify[e.Metric] = now
				todo = append(todo, e)
			}
		}
		for k, last := range alerts.lastNotify {
			if now.Sub(last) > 30*time.Minute {
				delete(alerts.lastNotify, k)
			}
		}
		if len(todo) > 0 {
			go notifyWebhook(todo, activeCnt)
		}
	}
}

func countUnhealthyContainers() int {
	out, err := run(10*time.Second, "docker", "ps", "-a", "--format", "{{.State}} {{.Status}}")
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(out, "\n") {
		l := strings.ToLower(line)
		if strings.Contains(l, "unhealthy") || strings.Contains(l, "dead") || strings.Contains(l, "oom") {
			n++
		}
	}
	return n
}

func countFailedServices() int {
	out, err := run(10*time.Second, "systemctl", "list-units", "--type=service", "--state=failed", "--no-pager", "--no-legend")
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func readDiskUsage() []DiskInfo {
	out, err := run(10*time.Second, "df", "-B1", "--output=source,fstype,size,used,avail,pcent,target")
	if err != nil {
		return nil
	}
	var list []DiskInfo
	for i, line := range strings.Split(out, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		total, _ := parseInt(f[2])
		used, _ := parseInt(f[3])
		pct, _ := parseFloat(strings.TrimSuffix(f[5], "%"))
		list = append(list, DiskInfo{Device: f[0], FSType: f[1], Total: total,
			Used: used, Mountpoint: f[len(f)-1], UsedPct: pct})
	}
	return list
}

func parseInt(s string) (uint64, error) {
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func parseFloat(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(s, "%g", &v)
	return v, err
}

// notifyWebhook sends an alert summary to a wecom/dingtalk-style webhook.
func notifyWebhook(events []AlertEvent, activeCnt int) {
	var b strings.Builder
	for _, e := range events {
		state := "触发"
		if !e.Active {
			state = "恢复"
		}
		b.WriteString(fmt.Sprintf("- [%s] %s %s: 当前值 %.1f\n", state, e.Detail, e.Level, e.Value))
	}
	text := fmt.Sprintf("【pf3090 运维告警】当前活跃告警 %d 条\n%s", activeCnt, b.String())
	payload := map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(alerts.webhook, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("webhook notify failed: %v", err)
		return
	}
	resp.Body.Close()
}

func AlertsHandler(c *gin.Context) {
	alerts.mu.Lock()
	defer alerts.mu.Unlock()
	active := make([]AlertEvent, 0, len(alerts.active))
	for _, e := range alerts.active {
		active = append(active, e)
	}
	recent := alerts.recent
	if recent == nil {
		recent = []AlertEvent{}
	}
	ok(c, gin.H{"active": active, "recent": recent})
}
