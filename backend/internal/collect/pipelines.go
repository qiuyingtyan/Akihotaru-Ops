package collect

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
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
	FailReason string `json:"failReason,omitempty"`
}

const gitlabLocalURL = "http://127.0.0.1:9980"

var (
	reSucceeded  = regexp.MustCompile(`Job succeeded`)
	reFailed     = regexp.MustCompile(`Job failed`)
	reANSI       = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	reField      = regexp.MustCompile(`(duration_s|job|job-status|project_full_path|failure_reason)\s*=\s*("[^"]*"|\S+)`)
	reTimestamp  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z`)
	reLogrusMsg  = regexp.MustCompile(`\bmsg\s*=\s*"([^"]*)"`)
	reLogrusErr  = regexp.MustCompile(`\berror\s*=\s*"([^"]*)"`)
	reTrailKV    = regexp.MustCompile(`\s+[A-Za-z_][A-Za-z0-9_-]*\s*=\s*\S+\s*$`)
	reJobID      = regexp.MustCompile(`\bjob\s*=\s*(\d+)\b`)
	reLogrusTime = regexp.MustCompile(`^time\s*=\s*"[^"]*"\s*`)
)

var gitlabHTTP = &http.Client{
	Timeout: 3 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if strings.Contains(req.URL.Path, "sign_in") {
			return http.ErrUseLastResponse
		}
		if len(via) >= 2 {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func fetchRunnerJobsFromLog() []PipelineJob {
	runner := FindRunnerContainer()
	out, err := run(10*time.Second, "docker", "logs", "--timestamps", "--tail", "3000", runner)
	if err != nil && out == "" {
		return nil
	}
	jobs := parseRunnerLog(out)
	enrichFailedJobs(jobs)
	return jobs
}

type jobAcc struct {
	job      PipelineJob
	failEnum string
	failMsgs []string
	lineIdx  int
}

func parseRunnerLog(logText string) []PipelineJob {
	rawLines := strings.Split(logText, "\n")
	byID := map[int64]*jobAcc{}
	var order []int64

	ensure := func(id int64) *jobAcc {
		if a, ok := byID[id]; ok {
			return a
		}
		a := &jobAcc{job: PipelineJob{JobID: id}, lineIdx: -1}
		byID[id] = a
		order = append(order, id)
		return a
	}

	type orphan struct {
		idx int
		msg string
	}
	var orphans []orphan

	for i, rawLine := range rawLines {
		line := reANSI.ReplaceAllString(rawLine, "")
		if strings.TrimSpace(line) == "" {
			continue
		}
		jobID := parseJobID(line)
		isSucc := reSucceeded.MatchString(line)
		isFail := reFailed.MatchString(line)
		msg := failMessageFromLine(line)

		if jobID == 0 {
			if msg != "" && isFail {
				orphans = append(orphans, orphan{idx: i, msg: msg})
			}
			continue
		}

		a := ensure(jobID)
		if isSucc || isFail {
			if isFail {
				a.job.Status = "failed"
			} else if a.job.Status != "failed" {
				a.job.Status = "success"
			}
			a.lineIdx = i
			applyRunnerFields(&a.job, &a.failEnum, line)
			if m := reTimestamp.FindString(line); m != "" {
				if t, err := time.Parse(time.RFC3339Nano, m); err == nil {
					a.job.FinishedAt = t.Local().Format("01-02 15:04:05")
				}
			}
		}
		if msg != "" {
			a.failMsgs = append(a.failMsgs, msg)
		}
	}

	for _, o := range orphans {
		var best *jobAcc
		bestDist := 25
		for _, a := range byID {
			if a.job.Status != "failed" || a.lineIdx < 0 {
				continue
			}
			d := o.idx - a.lineIdx
			if d < 0 {
				d = -d
			}
			if d < bestDist {
				bestDist = d
				best = a
			}
		}
		if best != nil {
			best.failMsgs = append(best.failMsgs, o.msg)
		}
	}

	var jobs []PipelineJob
	for _, id := range order {
		a := byID[id]
		if a.job.Status == "" {
			continue
		}
		if a.job.Status == "failed" {
			a.job.FailReason = composeFailReason(a.failEnum, pickBestMsg(a.failMsgs))
		}
		jobs = append(jobs, a.job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].JobID > jobs[j].JobID })
	if len(jobs) > 30 {
		jobs = jobs[:30]
	}
	return jobs
}

func applyRunnerFields(job *PipelineJob, failEnum *string, line string) {
	for _, m := range reField.FindAllStringSubmatch(line, -1) {
		val := strings.Trim(m[2], `"`)
		switch m[1] {
		case "job":
			if job.JobID == 0 {
				job.JobID, _ = strconv.ParseInt(val, 10, 64)
			}
		case "project_full_path":
			if val != "" {
				job.Project = val
			}
		case "duration_s":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				job.Duration = strconv.FormatFloat(f, 'f', 0, 64) + "s"
			} else {
				job.Duration = val
			}
		case "failure_reason":
			if val != "" {
				*failEnum = val
			}
		}
	}
}

func parseJobID(line string) int64 {
	m := reJobID.FindStringSubmatch(line)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	return n
}

func failMessageFromLine(line string) string {
	clean := reANSI.ReplaceAllString(line, "")
	if m := reLogrusMsg.FindStringSubmatch(clean); len(m) > 1 {
		msg := strings.TrimSpace(m[1])
		low := strings.ToLower(msg)
		if strings.Contains(low, "fail") || strings.Contains(low, "error") || strings.Contains(low, "timeout") {
			if e := reLogrusErr.FindStringSubmatch(clean); len(e) > 1 {
				extra := strings.TrimSpace(e[1])
				if extra != "" && !strings.Contains(msg, extra) {
					msg = msg + ": " + extra
				}
			}
			return trimReason(msg)
		}
	}
	if e := reLogrusErr.FindStringSubmatch(clean); len(e) > 1 {
		extra := strings.TrimSpace(e[1])
		if extra != "" {
			return trimReason("Job failed: " + extra)
		}
	}

	stripped := stripRunnerMeta(clean)
	stripped = strings.TrimPrefix(stripped, "WARNING: ")
	stripped = strings.TrimPrefix(stripped, "ERROR: ")
	low := strings.ToLower(stripped)
	idx := strings.Index(stripped, "Job failed")
	if idx >= 0 {
		return trimReason(stripped[idx:])
	}
	if strings.Contains(low, "panic") || strings.Contains(low, "fatal") ||
		strings.Contains(low, "timeout") || strings.HasPrefix(low, "error") {
		return trimReason(stripped)
	}
	return ""
}

func stripRunnerMeta(line string) string {
	line = reTimestamp.ReplaceAllString(line, "")
	line = reLogrusTime.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)
	for {
		next := reTrailKV.ReplaceAllString(line, "")
		if next == line {
			break
		}
		line = next
	}
	return strings.TrimSpace(strings.Trim(line, `"'`))
}

func trimReason(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

func composeFailReason(enum, msg string) string {
	tr := translateFailureReason(enum)
	msg = strings.TrimSpace(msg)
	if strings.EqualFold(msg, "Job failed") || strings.EqualFold(msg, "Job failed:") {
		msg = ""
	}
	switch {
	case tr != "" && msg != "":
		if strings.Contains(msg, tr) {
			return msg
		}
		return tr + "：" + msg
	case tr != "":
		return tr
	case msg != "":
		return msg
	default:
		return "Job failed（未记录详细原因）"
	}
}

func translateFailureReason(r string) string {
	r = strings.Trim(strings.TrimSpace(r), `"`)
	switch r {
	case "script_failure":
		return "脚本执行失败"
	case "runner_system_failure":
		return "Runner 系统故障"
	case "job_execution_timeout":
		return "任务执行超时"
	case "stuck_or_timeout_failure":
		return "任务卡住或超时"
	case "unknown_failure":
		return "未知失败"
	case "canceled", "cancelled":
		return "已取消"
	case "archived_failure":
		return "任务已归档失败"
	case "unmet_prerequisites":
		return "前置条件不满足"
	case "scheduler_failure":
		return "调度失败"
	case "data_integrity_failure":
		return "数据完整性失败"
	case "forward_deployment_failure":
		return "部署转发失败"
	case "api_failure":
		return "GitLab API 失败"
	case "missing_dependency_failure":
		return "缺少依赖"
	case "runner_unsupported":
		return "Runner 不支持该任务"
	case "no_matching_runner":
		return "没有匹配的 Runner"
	default:
		return r
	}
}

func pickBestMsg(msgs []string) string {
	best := ""
	bestScore := -1
	for _, m := range msgs {
		s := reasonScore(m)
		if s > bestScore {
			best, bestScore = m, s
		}
	}
	return best
}

func reasonScore(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	low := strings.ToLower(s)
	if low == "job failed" || low == "job failed:" {
		return 1
	}
	score := len(s)
	if strings.Contains(low, "ci_fail_reason") {
		score += 80
	}
	if strings.Count(s, "\n") > 0 {
		score += 60
	}
	if strings.Contains(low, "system failure") || strings.Contains(low, "timeout") {
		score += 25
	}
	if strings.Contains(low, "exit code") || strings.Contains(low, "exit status") {
		score += 15
	}
	return score
}

func gitlabAuthBlocked(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden ||
		code == http.StatusFound || code == http.StatusMovedPermanently ||
		code == http.StatusSeeOther || code == http.StatusTemporaryRedirect ||
		code == http.StatusPermanentRedirect
}

func enrichFailedJobs(jobs []PipelineJob) {
	var idxs []int
	for i, j := range jobs {
		if j.Status == "failed" && j.JobID != 0 {
			idxs = append(idxs, i)
		}
	}
	if len(idxs) == 0 {
		return
	}
	if len(idxs) > 8 {
		idxs = idxs[:8]
	}

	token := strings.TrimSpace(os.Getenv("OPSWEB_GITLAB_TOKEN"))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	reason, code := fetchGitLabJobTrace(ctx, jobs[idxs[0]], token)
	if gitlabAuthBlocked(code) {
		return
	}
	if reason != "" {
		jobs[idxs[0]].FailReason = pickBetterReason(jobs[idxs[0]].FailReason, reason)
	}

	rest := idxs[1:]
	if len(rest) == 0 {
		return
	}
	sem := make(chan struct{}, 4)
	var mu sync.Mutex
	var wg syncWaitGroup
	for _, i := range rest {
		i := i
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			r, _ := fetchGitLabJobTrace(ctx, jobs[i], token)
			if r != "" {
				mu.Lock()
				jobs[i].FailReason = pickBetterReason(jobs[i].FailReason, r)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
}

func pickBetterReason(old, neu string) string {
	if neu == "" {
		return old
	}
	if old == "" || reasonScore(neu) >= reasonScore(old) {
		return neu
	}
	return old
}

func fetchGitLabJobTrace(ctx context.Context, job PipelineJob, token string) (string, int) {
	if job.Project == "" {
		return "", 0
	}
	apiURL := gitlabLocalURL + "/api/v4/projects/" + url.PathEscape(job.Project) + "/jobs/" + strconv.FormatInt(job.JobID, 10) + "/trace"
	body, code, err := gitlabGet(ctx, apiURL, token)
	if err == nil && code == 200 && body != "" && !looksLikeHTML(body) {
		if r := extractFailReasonFromTrace(body); r != "" {
			return r, code
		}
	}
	if gitlabAuthBlocked(code) && token != "" {
		return "", code
	}
	rawURL := gitlabLocalURL + "/" + encodeProjectPath(job.Project) + "/-/jobs/" + strconv.FormatInt(job.JobID, 10) + "/raw"
	body, code, err = gitlabGet(ctx, rawURL, token)
	if err == nil && code == 200 && body != "" && !looksLikeHTML(body) {
		if r := extractFailReasonFromTrace(body); r != "" {
			return r, code
		}
	}
	return "", code
}

func encodeProjectPath(project string) string {
	parts := strings.Split(project, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func gitlabGet(ctx context.Context, rawURL, token string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept", "text/plain, application/json;q=0.9, */*;q=0.1")
	if token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
	}
	resp, err := gitlabHTTP.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	return readTail(io.LimitReader(resp.Body, 8<<20), 512<<10), resp.StatusCode, nil
}

func readTail(r io.Reader, n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, 0, n)
	tmp := make([]byte, 32*1024)
	for {
		k, err := r.Read(tmp)
		if k > 0 {
			buf = append(buf, tmp[:k]...)
			if len(buf) > n {
				buf = append([]byte{}, buf[len(buf)-n:]...)
			}
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}

func looksLikeHTML(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	n := len(t)
	if n > 80 {
		n = 80
	}
	head := strings.ToLower(t[:n])
	return strings.HasPrefix(head, "<!doctype") || strings.HasPrefix(head, "<html") || strings.Contains(head, "<head")
}

func extractFailReasonFromTrace(trace string) string {
	trace = reANSI.ReplaceAllString(trace, "")
	trace = strings.ReplaceAll(trace, "\r\n", "\n")
	trace = strings.ReplaceAll(trace, "\r", "\n")

	var marker string
	for _, line := range strings.Split(trace, "\n") {
		if i := strings.Index(line, "CI_FAIL_REASON:"); i >= 0 {
			marker = strings.TrimSpace(line[i:])
		}
	}

	script := sliceTraceSection(trace, "step_script")
	if script == "" {
		script = sliceTraceSection(trace, "script")
	}
	if script == "" {
		script = trace
	}

	lines := usefulTraceLines(script)
	if len(lines) == 0 {
		lines = usefulTraceLines(trace)
	}
	if len(lines) == 0 && marker == "" {
		return ""
	}

	lastCmd := 0
	for i, ln := range lines {
		if strings.HasPrefix(ln, "$ ") && !strings.Contains(ln, "CI_JOB_STATUS") && !strings.Contains(ln, "CI_FAIL_REASON") {
			lastCmd = i
		}
	}
	chunk := lines[lastCmd:]
	if len(chunk) > 18 {
		chunk = chunk[len(chunk)-18:]
	}

	var b strings.Builder
	if marker != "" {
		b.WriteString(marker)
		if len(chunk) > 0 {
			b.WriteByte('\n')
		}
	}
	b.WriteString(strings.Join(chunk, "\n"))
	if errLine := lastJobFailedLine(trace); errLine != "" && !strings.Contains(b.String(), errLine) {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(errLine)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 1500 {
		out = "…" + out[len(out)-1499:]
	}
	return out
}

func sliceTraceSection(trace, name string) string {
	tag := ":" + name
	lines := strings.Split(trace, "\n")
	start, end := -1, -1
	for i, ln := range lines {
		if strings.Contains(ln, "section_start:") && strings.Contains(ln, tag) {
			start = i + 1
			end = -1
		}
		if start >= 0 && strings.Contains(ln, "section_end:") && strings.Contains(ln, tag) {
			end = i
		}
	}
	if start < 0 {
		return ""
	}
	if end < start {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

func usefulTraceLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || isTraceNoise(ln) {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func isTraceNoise(s string) bool {
	if strings.Contains(s, "section_start:") || strings.Contains(s, "section_end:") {
		return true
	}
	prefixes := []string{
		"Running with gitlab-runner",
		"Preparing the \"",
		"Preparing environment",
		"Using Docker executor",
		"Using Kubernetes executor",
		"Pulling docker image",
		"Using docker image",
		"Getting source from Git repository",
		"Skipping Git submodules",
		"Restoring cache",
		"Saving cache",
		"Downloading artifacts",
		"Uploading artifacts",
		"Cleaning up project directory and file based variables",
		"Job succeeded",
		"Executing \"step_script\" stage",
		"Executing \"prepare_script\" stage",
		"Executing \"get_sources\" stage",
		"Executing \"after_script\" stage",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func lastJobFailedLine(trace string) string {
	var last string
	for _, ln := range strings.Split(trace, "\n") {
		ln = strings.TrimSpace(reANSI.ReplaceAllString(ln, ""))
		if strings.Contains(ln, "Job failed") {
			last = stripRunnerMeta(strings.TrimPrefix(strings.TrimPrefix(ln, "WARNING: "), "ERROR: "))
		}
	}
	return last
}

var pipelineCache = struct {
	sync.Mutex
	jobs []PipelineJob
	t    time.Time
}{}

// PipelinesHandler returns recent CI/CD job results (cached 60s).
func PipelinesHandler(c *gin.Context) {
	ok(c, CorePipelines())
}

func CorePipelines() []PipelineJob {
	pipelineCache.Lock()
	if pipelineCache.jobs != nil && time.Since(pipelineCache.t) <= time.Minute {
		jobs := pipelineCache.jobs
		pipelineCache.Unlock()
		return jobs
	}
	pipelineCache.Unlock()

	jobs := fetchRunnerJobsFromLog()
	if jobs == nil {
		jobs = []PipelineJob{}
	}

	pipelineCache.Lock()
	pipelineCache.jobs = jobs
	pipelineCache.t = time.Now()
	pipelineCache.Unlock()
	return jobs
}
