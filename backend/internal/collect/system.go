package collect

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

func ok(c *gin.Context, data any)     { c.JSON(200, gin.H{"code": 0, "data": data}) }
func fail(c *gin.Context, msg string) { c.JSON(500, gin.H{"code": 1, "error": msg}) }

type Overview struct {
	Hostname     string    `json:"hostname"`
	OS           string    `json:"os"`
	Platform     string    `json:"platform"`
	Kernel       string    `json:"kernel"`
	UptimeSec    uint64    `json:"uptimeSec"`
	CPUCores     int       `json:"cpuCores"`
	CPUModel     string    `json:"cpuModel"`
	CPUPercent   float64   `json:"cpuPercent"`
	MemTotal     uint64    `json:"memTotal"`
	MemUsed      uint64    `json:"memUsed"`
	MemPercent   float64   `json:"memPercent"`
	SwapTotal    uint64    `json:"swapTotal"`
	SwapUsed     uint64    `json:"swapUsed"`
	Load1        float64   `json:"load1"`
	Load5        float64   `json:"load5"`
	Load15       float64   `json:"load15"`
	BootTime     uint64    `json:"bootTime"`
	ContainerCnt int       `json:"containerCount"`
	RunningCnt   int       `json:"runningCount"`
	Time         time.Time `json:"time"`
}

var overviewCache = struct {
	sync.Mutex
	o *Overview
	t time.Time
}{}

// CachedOverviewHandler serves the overview with a 3s TTL to avoid
// hammering docker/proc when many tabs poll simultaneously.
func CachedOverviewHandler(c *gin.Context) {
	overviewCache.Lock()
	defer overviewCache.Unlock()
	if overviewCache.o != nil && time.Since(overviewCache.t) < 3*time.Second {
		ok(c, overviewCache.o)
		return
	}
	o := buildOverview()
	overviewCache.o = &o
	overviewCache.t = time.Now()
	ok(c, o)
}

func OverviewHandler(c *gin.Context) {
	ok(c, buildOverview())
}

func buildOverview() Overview {
	o := Overview{Time: time.Now()}
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		hi := getHostInfo()
		o.Hostname = hi.Hostname
		o.OS = hi.OS
		o.Platform = hi.Platform
		o.Kernel = hi.Kernel
		o.UptimeSec = hi.Uptime
		o.BootTime = hi.BootTime
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.CPUModel, o.CPUCores = getCpuModel()
		o.CPUPercent = getCpuPercent()
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		vm, sw := getMemStat()
		o.MemTotal = vm.Total
		o.MemUsed = vm.Used
		o.MemPercent = vm.Percent
		o.SwapTotal = sw.Total
		o.SwapUsed = sw.Used
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.Load1, o.Load5, o.Load15 = getLoadAvg()
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		out, err := run(5*time.Second, "docker", "ps", "-a", "--format", "{{.State}}")
		if err == nil {
			for _, line := range strings.Fields(out) {
				o.ContainerCnt++
				if line == "running" || line == "healthy" || line == "restarting" {
					o.RunningCnt++
				}
			}
		}
	}()
	wg.Wait()
	return o
}

func DiskHandler(c *gin.Context) {
	out, err := run(10*time.Second, "df", "-B1", "--output=source,fstype,size,used,avail,pcent,target")
	if err != nil {
		fail(c, err.Error())
		return
	}
	var list []DiskInfo
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		total, _ := strconv.ParseUint(f[2], 10, 64)
		used, _ := strconv.ParseUint(f[3], 10, 64)
		avail, _ := strconv.ParseUint(f[4], 10, 64)
		pct, _ := strconv.ParseFloat(strings.TrimSuffix(f[5], "%"), 64)
		list = append(list, DiskInfo{
			Device: f[0], FSType: f[1], Total: total, Used: used, Free: avail,
			UsedPct: pct, Mountpoint: f[len(f)-1],
		})
	}
	ok(c, list)
}

func MemHandler(c *gin.Context) {
	vm, sw := getMemStat()
	ok(c, gin.H{"virtual": vm, "swap": sw})
}

func CPUHandler(c *gin.Context) {
	model, cores := getCpuModel()
	l1, l5, l15 := getLoadAvg()
	ok(c, gin.H{
		"model": model, "cores": cores,
		"perCore": getPerCorePercent(), "percent": getCpuPercent(),
		"load": gin.H{"load1": l1, "load5": l5, "load15": l15},
	})
}

func NetHandler(c *gin.Context) {
	ok(c, readNetIO())
}

type Sample struct {
	T int64   `json:"t"`
	V float64 `json:"v"`
}

const histFile = "/workspace/opsweb/history.json"

var loadHist = struct {
	mu    sync.Mutex
	cpu   []Sample
	mem   []Sample
	dirty bool
}{}

// StartSampler records cpu/mem percent once a minute for history charts.
// Samples are persisted to disk and survive restarts (retained 30 days).
func StartSampler(metricsIngest func(t int64, name string, v float64)) {
	loadHist.mu.Lock()
	loadSampleFromDisk()
	loadHist.mu.Unlock()

	go persistLoop()
	// prime baseline
	getCpuPercent()
	go func() {
		for {
			cp := getCpuPercent()
			vm, _ := getMemStat()
			now := time.Now()
			loadHist.mu.Lock()
			loadHist.cpu = append(loadHist.cpu, Sample{T: now.Unix(), V: cp})
			loadHist.mem = append(loadHist.mem, Sample{T: now.Unix(), V: vm.Percent})
			trimHistory(now)
			loadHist.dirty = true
			loadHist.mu.Unlock()
			if metricsIngest != nil {
				metricsIngest(now.Unix(), "cpu", cp)
				metricsIngest(now.Unix(), "mem", vm.Percent)
			}
			time.Sleep(time.Minute)
		}
	}()
}

func trimHistory(now time.Time) {
	cutoff := now.Add(-30 * 24 * time.Hour).Unix()
	for len(loadHist.cpu) > 0 && loadHist.cpu[0].T < cutoff {
		loadHist.cpu = loadHist.cpu[1:]
	}
	for len(loadHist.mem) > 0 && loadHist.mem[0].T < cutoff {
		loadHist.mem = loadHist.mem[1:]
	}
}

func loadSampleFromDisk() {
	data, err := os.ReadFile(histFile)
	if err != nil {
		return
	}
	var stored struct {
		Cpu []Sample `json:"cpu"`
		Mem []Sample `json:"mem"`
	}
	if json.Unmarshal(data, &stored) == nil {
		loadHist.cpu = stored.Cpu
		loadHist.mem = stored.Mem
		trimHistory(time.Now())
	}
}

func persistLoop() {
	for {
		time.Sleep(2 * time.Minute)
		loadHist.mu.Lock()
		if !loadHist.dirty {
			loadHist.mu.Unlock()
			continue
		}
		cpu := loadHist.cpu
		mem := loadHist.mem
		loadHist.dirty = false
		loadHist.mu.Unlock()
		data, _ := json.Marshal(struct {
			Cpu []Sample `json:"cpu"`
			Mem []Sample `json:"mem"`
		}{cpu, mem})
		tmp := histFile + ".tmp"
		if os.WriteFile(tmp, data, 0644) == nil {
			os.Rename(tmp, histFile)
		}
	}
}

func LoadHistoryHandler(c *gin.Context) {
	days := 1
	if d, err := strconv.Atoi(c.DefaultQuery("days", "1")); err == nil && d >= 1 && d <= 30 {
		days = d
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	loadHist.mu.Lock()
	var cpuCp, memCp []Sample
	for _, s := range loadHist.cpu {
		if s.T >= cutoff {
			cpuCp = append(cpuCp, s)
		}
	}
	for _, s := range loadHist.mem {
		if s.T >= cutoff {
			memCp = append(memCp, s)
		}
	}
	loadHist.mu.Unlock()

	// merge pgsql-stored metrics (authoritative once available)
	if dbCpu, dbMem := queryMetrics(cutoff); len(dbCpu) > 0 || len(dbMem) > 0 {
		cpuCp = mergeSamples(cpuCp, dbCpu)
		memCp = mergeSamples(memCp, dbMem)
	}
	if cpuCp == nil {
		cpuCp = []Sample{}
	}
	if memCp == nil {
		memCp = []Sample{}
	}
	ok(c, gin.H{"cpu": cpuCp, "mem": memCp})
}

// mergeSamples merges two sorted sample lists, pg wins on duplicate timestamps.
func mergeSamples(a, b []Sample) []Sample {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := make(map[int64]bool, len(b))
	for _, s := range b {
		seen[s.T] = true
	}
	out := make([]Sample, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s.T] {
			out = append(out, s)
		}
	}
	out = append(out, b...)
	return out
}

func ProcessesHandler(c *gin.Context) {
	procs := listProcesses()
	sort.Slice(procs, func(i, j int) bool { return procs[i].CPUPerc > procs[j].CPUPerc })
	if len(procs) > 50 {
		procs = procs[:50]
	}
	ok(c, procs)
}
