package collect

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func readProcFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

type HostInfo struct {
	Hostname string
	OS       string
	Platform string
	Kernel   string
	Uptime   uint64
	BootTime uint64
}

func getHostInfo() HostInfo {
	var hi HostInfo
	hi.OS = "linux"
	hi.Hostname, _ = os.Hostname()
	if s := readProcFile("/proc/sys/kernel/hostname"); s != "" {
		hi.Hostname = strings.TrimSpace(s)
	}
	if s := readProcFile("/proc/version"); s != "" {
		f := strings.Fields(s)
		if len(f) >= 3 {
			hi.Kernel = f[2]
		}
	}
	if s := readProcFile("/etc/os-release"); s != "" {
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				hi.Platform = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			}
		}
	}
	if s := readProcFile("/proc/uptime"); s != "" {
		f := strings.Fields(s)
		if len(f) > 0 {
			up, _ := strconv.ParseFloat(f[0], 64)
			hi.Uptime = uint64(up)
			hi.BootTime = uint64(time.Now().Unix()) - hi.Uptime
		}
	}
	return hi
}

type MemStat struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Percent   float64 `json:"percent"`
}

func getMemStat() (MemStat, MemStat) {
	var vm, sw MemStat
	s := readProcFile("/proc/meminfo")
	var swapTotal, swapFree uint64
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		v *= 1024
		switch f[0] {
		case "MemTotal:":
			vm.Total = v
		case "MemAvailable:":
			vm.Available = v
		case "SwapTotal:":
			swapTotal = v
		case "SwapFree:":
			swapFree = v
		}
	}
	vm.Used = vm.Total - vm.Available
	if vm.Total > 0 {
		vm.Percent = float64(vm.Used) / float64(vm.Total) * 100
	}
	sw.Total = swapTotal
	sw.Used = swapTotal - swapFree
	if swapTotal > 0 {
		sw.Percent = float64(sw.Used) / float64(swapTotal) * 100
	}
	return vm, sw
}

type CPULoad struct {
	Cores    int     `json:"cores"`
	Model    string  `json:"model"`
	Percent  float64 `json:"percent"`
	Load1    float64 `json:"load1"`
	Load5    float64 `json:"load5"`
	Load15   float64 `json:"load15"`
	PerCore  []float64 `json:"perCore,omitempty"`
}

var (
	prevTotal uint64
	prevIdle  uint64
	prevMu    sync.Mutex
	prevOnce  sync.Once
)

type cpuTimes struct{ total, idle uint64 }

func readCpuTimes() map[string]cpuTimes {
	out := map[string]cpuTimes{}
	s := readProcFile("/proc/stat")
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		var total, idle uint64
		for i, v := range f[1:] {
			n, _ := strconv.ParseUint(v, 10, 64)
			total += n
			if i == 3 || i == 4 {
				idle += n
			}
		}
		out[f[0]] = cpuTimes{total, idle}
	}
	return out
}

func getCpuPercent() float64 {
	prevMu.Lock()
	defer prevMu.Unlock()
	cur := readCpuTimes()
	c, ok := cur["cpu"]
	if !ok {
		return 0
	}
	prevOnce.Do(func() {
		prevTotal, prevIdle = c.total, c.idle
		time.Sleep(200 * time.Millisecond)
		cur = readCpuTimes()
		c = cur["cpu"]
	})
	dt := c.total - prevTotal
	di := c.idle - prevIdle
	prevTotal, prevIdle = c.total, c.idle
	if dt == 0 {
		return 0
	}
	return float64(dt-di) / float64(dt) * 100
}

func getPerCorePercent() []float64 {
	// single-shot: compare against /proc/stat saved 300ms earlier
	t1 := readCpuTimes()
	time.Sleep(300 * time.Millisecond)
	t2 := readCpuTimes()
	var out []float64
	for i := 0; ; i++ {
		key := "cpu" + strconv.Itoa(i)
		a, ok1 := t1[key]
		b, ok2 := t2[key]
		if !ok1 || !ok2 {
			break
		}
		dt := b.total - a.total
		di := b.idle - a.idle
		if dt == 0 {
			out = append(out, 0)
		} else {
			out = append(out, float64(dt-di)/float64(dt)*100)
		}
	}
	return out
}

func getCpuModel() (string, int) {
	s := readProcFile("/proc/cpuinfo")
	model := ""
	cores := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "model name") {
			if model == "" {
				model = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			}
			cores++
		}
	}
	if cores == 0 {
		cores = 1
	}
	return model, cores
}

func getLoadAvg() (float64, float64, float64) {
	f := strings.Fields(readProcFile("/proc/loadavg"))
	if len(f) < 3 {
		return 0, 0, 0
	}
	l1, _ := strconv.ParseFloat(f[0], 64)
	l5, _ := strconv.ParseFloat(f[1], 64)
	l15, _ := strconv.ParseFloat(f[2], 64)
	return l1, l5, l15
}

type DiskInfo struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	FSType     string  `json:"fstype"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	UsedPct    float64 `json:"usedPct"`
}

type NetIOStat struct {
	Name        string  `json:"name"`
	BytesSent   uint64  `json:"bytesSent"`
	BytesRecv   uint64  `json:"bytesRecv"`
	PktsSent    uint64  `json:"pktsSent"`
	PktsRecv    uint64  `json:"pktsRecv"`
	RateRecvKBs float64 `json:"rateRecvKBs"`
	RateSentKBs float64 `json:"rateSentKBs"`
}

var lastNet = struct {
	mu   sync.Mutex
	snap map[string]NetIOStat
	t    time.Time
}{snap: map[string]NetIOStat{}, t: time.Now()}

func readNetIO() []NetIOStat {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	now := time.Now()
	lastNet.mu.Lock()
	defer lastNet.mu.Unlock()
	dt := now.Sub(lastNet.t).Seconds()
	if dt < 0.5 {
		dt = 1
	}
	var out []NetIOStat
	for _, line := range strings.Split(string(data), "\n")[2:] {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		f := strings.Fields(parts[1])
		if len(f) < 9 {
			continue
		}
		recv, _ := strconv.ParseUint(f[0], 10, 64)
		pktsRecv, _ := strconv.ParseUint(f[1], 10, 64)
		sent, _ := strconv.ParseUint(f[8], 10, 64)
		pktsSent, _ := strconv.ParseUint(f[9], 10, 64)
		s := NetIOStat{Name: name, BytesRecv: recv, PktsRecv: pktsRecv, BytesSent: sent, PktsSent: pktsSent}
		if prev, ok := lastNet.snap[name]; ok && dt > 0 {
			s.RateRecvKBs = float64(recv-prev.BytesRecv) / 1024 / dt
			s.RateSentKBs = float64(sent-prev.BytesSent) / 1024 / dt
		}
		out = append(out, s)
	}
	lastNet.snap = map[string]NetIOStat{}
	for _, s := range out {
		lastNet.snap[s.Name] = s
	}
	lastNet.t = now
	return out
}

type ProcProcess struct {
	Pid     int     `json:"pid"`
	Name    string  `json:"name"`
	CPUPerc float64 `json:"cpuPercent"`
	MemPerc float64 `json:"memPercent"`
	RSS     uint64  `json:"rssMB"`
}

var procCPULast = struct {
	mu   sync.Mutex
	m    map[int]uint64
	t    time.Time
}{m: map[int]uint64{}, t: time.Now()}

func listProcesses() []ProcProcess {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	totalMem, _ := getMemStat()
	now := time.Now()
	procCPULast.mu.Lock()
	defer procCPULast.mu.Unlock()
	dt := now.Sub(procCPULast.t).Seconds()
	if dt < 0.5 {
		dt = 1
	}
	hertz := 100.0
	if clk := getconf("CLK_TCK"); clk > 0 {
		hertz = clk
	}
	var out []ProcProcess
	newSnap := map[int]uint64{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat := readProcFile("/proc/" + e.Name() + "/stat")
		ut := strings.LastIndex(stat, ") ")
		if ut < 0 {
			continue
		}
		f := strings.Fields(stat[ut+2:])
		if len(f) < 22 {
			continue
		}
		utime, _ := strconv.ParseUint(f[11], 10, 64)
		stime, _ := strconv.ParseUint(f[12], 10, 64)
		totalTicks := utime + stime
		newSnap[pid] = totalTicks
		var cpuPct float64
		if prev, ok := procCPULast.m[pid]; ok {
			cpuPct = float64(totalTicks-prev) / hertz / dt * 100
		}
		rssKb, _ := strconv.ParseUint(f[21], 10, 64)
		memPct := 0.0
		if totalMem.Total > 0 {
			memPct = float64(rssKb*1024) / float64(totalMem.Total) * 100
		}
		name := ""
		if lp := strings.Index(stat, "("); lp > 0 && ut > lp {
			name = stat[lp+1 : ut]
		}
		out = append(out, ProcProcess{
			Pid: pid, Name: name, CPUPerc: cpuPct,
			MemPerc: memPct, RSS: rssKb / 1024,
		})
	}
	procCPULast.m = newSnap
	procCPULast.t = now
	return out
}

func getconf(name string) float64 {
	out, err := run(2*time.Second, "getconf", name)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(out), 64)
	return v
}
