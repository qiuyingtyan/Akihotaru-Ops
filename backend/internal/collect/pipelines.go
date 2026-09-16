package collect

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type PipelineJob struct {
	JobID      int64  `json:"jobId"`
	Project    string `json:"project"`
	Status     string `json:"status"`
	Duration   string `json:"duration"`
	FinishedAt string `json:"finishedAt"`
}

var (
	reSucceeded = regexp.MustCompile(`Job succeeded`)
	reFailed    = regexp.MustCompile(`Job failed`)
	reANSI      = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	reField     = regexp.MustCompile(`(duration_s|job|job-status|project_full_path)=([^ ]+)`)
	reTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z`)
)

// parseRunnerJournal scans runner log files / container logs for job result lines.
func fetchRunnerJobsFromLog() []PipelineJob {
	out, err := run(15*time.Second, "docker", "logs", "--timestamps", "--tail", "20000", "baq-gitlab-runner")
	if err != nil && out == "" {
		return nil
	}
	return parseRunnerLog(out)
}

func parseRunnerLog(logText string) []PipelineJob {
	var jobs []PipelineJob
	seen := map[int64]bool{}
	for _, rawLine := range strings.Split(logText, "\n") {
		line := reANSI.ReplaceAllString(rawLine, "")
		isSucc := reSucceeded.MatchString(line)
		isFail := reFailed.MatchString(line)
		if !isSucc && !isFail {
			continue
		}
		status := "success"
		if isFail {
			status = "failed"
		}
		var jobID int64
		project, duration := "", ""
		for _, m := range reField.FindAllStringSubmatch(line, -1) {
			switch m[1] {
			case "job":
				jobID, _ = strconv.ParseInt(m[2], 10, 64)
			case "project_full_path":
				project = m[2]
			case "duration_s":
				d := m[2]
				if f, err := strconv.ParseFloat(d, 64); err == nil {
					d = strconv.FormatFloat(f, 'f', 0, 64) + "s"
				}
				duration = d
			}
		}
		if jobID == 0 || seen[jobID] {
			continue
		}
		seen[jobID] = true
		finished := ""
		// docker logs --timestamps prefix: 2026-09-15T06:37:58.792034474Z
		if m := reTimestamp.FindString(line); m != "" {
			if t, err := time.Parse(time.RFC3339Nano, m); err == nil {
				finished = t.Local().Format("01-02 15:04:05")
			}
		}
		jobs = append(jobs, PipelineJob{
			JobID: jobID, Project: project, Status: status,
			Duration: duration, FinishedAt: finished,
		})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].JobID > jobs[j].JobID })
	if len(jobs) > 30 {
		jobs = jobs[:30]
	}
	return jobs
}

var pipelineCache = struct {
	sync.Mutex
	jobs []PipelineJob
	t    time.Time
}{}

// PipelinesHandler returns recent CI/CD job results (cached 60s).
func PipelinesHandler(c *gin.Context) {
	pipelineCache.Lock()
	defer pipelineCache.Unlock()
	if pipelineCache.jobs == nil || time.Since(pipelineCache.t) > time.Minute {
		pipelineCache.jobs = fetchRunnerJobsFromLog()
		pipelineCache.t = time.Now()
	}
	if pipelineCache.jobs == nil {
		pipelineCache.jobs = []PipelineJob{}
	}
	ok(c, pipelineCache.jobs)
}
