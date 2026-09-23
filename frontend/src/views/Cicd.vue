<template>
  <div>
    <h2 class="page-title">CI/CD 状态 ♪</h2>
    <div v-if="err" class="error-msg">(｡•́︿•̀｡) {{ err }}</div>
    <template v-else-if="s">
      <div class="grid grid-3">
        <div class="card">
          <div class="stat-label">GitLab (127.0.0.1:9980)</div>
          <div class="stat-value">
            <span class="badge" :class="s.gitlabUp ? 'green' : 'red'">{{ s.gitlabUp ? '正常' : '异常' }}</span>
          </div>
          <div class="muted mt">HTTP: {{ s.gitlabDetail }}</div>
        </div>
        <div class="card">
          <div class="stat-label">GitLab Runner (baq-gitlab-runner)</div>
          <div class="stat-value">
            <span class="badge" :class="s.runnerState==='running' ? 'green' : 'red'">{{ s.runnerState || '未知' }}</span>
          </div>
          <div class="muted mt">{{ s.runnerDetail }}</div>
        </div>
        <div class="card">
          <div class="stat-label">Nacos (127.0.0.1:8848)</div>
          <div class="stat-value">
            <span class="badge" :class="s.nacosUp ? 'green' : 'red'">{{ s.nacosUp ? '正常' : '异常' }}</span>
          </div>
        </div>
      </div>

      <div class="card mt">
        <div class="stat-label">部署脚本</div>
        <div v-if="s.deployHooks.length">
          <div v-for="h in s.deployHooks" :key="h" style="padding:4px 0;font-family:monospace;font-size:13px">📜 {{ h }}</div>
        </div>
        <div v-else class="muted mt">未发现部署脚本</div>
      </div>

      <div class="card mt" v-for="j in s.ciJobs" :key="j.configFile">
        <div style="display:flex;justify-content:space-between">
          <strong>{{ j.name }}</strong>
          <span class="muted">{{ j.configFile }}</span>
        </div>
        <pre class="log mt">{{ j.stages }}</pre>
      </div>

      <div class="card mt">
        <div class="stat-label">最近构建记录（Runner 执行历史，最新 30 条）</div>
        <div class="muted mt" style="font-size:12px">失败记录可悬停查看原因，点击即可复制</div>
        <div v-if="pipelines.length">
          <table class="mt">
            <thead><tr><th>Job</th><th>项目</th><th>结果</th><th>耗时</th><th>完成时间</th></tr></thead>
            <tbody>
              <tr
                v-for="j in pipelines"
                :key="j.jobId"
                :class="{ 'ci-failed-row': j.status !== 'success' }"
                @mouseenter="j.status !== 'success' && showFailTip($event, j)"
                @mousemove="j.status !== 'success' && tip.show && placeTip($event)"
                @mouseleave="hideFailTip"
                @click="j.status !== 'success' && copyFailReason(j)"
              >
                <td>#{{ j.jobId }}</td>
                <td>{{ j.project }}</td>
                <td>
                  <span class="badge" :class="j.status==='success' ? 'green' : 'red'">{{ j.status === 'success' ? '✧ 成功' : '× 失败' }}</span>
                </td>
                <td>{{ j.duration || '-' }}</td>
                <td class="muted">{{ j.finishedAt || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="muted mt loading-tip">(っ˘ω˘ς) 暂无构建记录</div>
      </div>
      <Teleport to="body">
        <div
          v-if="tip.show && tip.job"
          class="ci-fail-tip"
          :style="tipStyle"
          @mouseenter="keepFailTip"
          @mouseleave="hideFailTip"
        >
          <div class="ci-fail-tip-hd">
            <span>Job #{{ tip.job.jobId }} 失败原因</span>
            <button class="btn" @click.stop="copyFailReason(tip.job)">复制</button>
          </div>
          <pre class="ci-fail-tip-body">{{ failText(tip.job) }}</pre>
        </div>
      </Teleport>

      <div class="card mt">
        <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px">
          <div class="stat-label">GitLab Runner 日志</div>
          <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
            <div class="log-btn-group">
              <button class="log-mode-btn" :class="{ active: viewMode === 'card' }" @click="viewMode = 'card'">📋 结构化</button>
              <button class="log-mode-btn" :class="{ active: viewMode === 'raw' }" @click="viewMode = 'raw'">💻 原生终端</button>
            </div>
            <div class="log-btn-group">
              <button class="log-filter-btn" :class="{ active: filterType === 'all' }" @click="filterType = 'all'">全部</button>
              <button class="log-filter-btn" :class="{ active: filterType === 'jobs' }" @click="filterType = 'jobs'">⭐ 关键事件</button>
              <button class="log-filter-btn" :class="{ active: filterType === 'errors' }" @click="filterType = 'errors'">🔴 错误告警</button>
            </div>
            <div class="log-search-wrap">
              <span style="font-size:12px;opacity:0.7">🔍</span>
              <input v-model="logKw" placeholder="过滤关键字 / JobID..." spellcheck="false" />
              <button v-if="logKw" class="log-clear-btn" @click="logKw = ''">✕</button>
            </div>
            <select v-model="tailN" @change="loadRunnerLogs" class="log-select">
              <option :value="50">50 行</option>
              <option :value="100">100 行</option>
              <option :value="200">200 行</option>
              <option :value="500">500 行</option>
            </select>
            <button class="btn" style="padding:4px 10px;font-size:12px" @click="loadRunnerLogs">🔄 刷新</button>
            <button class="btn" style="padding:4px 10px;font-size:12px" @click="copyAllRunnerLogs">{{ logCopied ? '✓ 已复制' : '📋 复制' }}</button>
          </div>
        </div>

        <div v-if="viewMode === 'card'" class="runner-card-wrap mt">
          <div v-if="loadingLogs" class="muted loading-tip">(๑>ᴗ<๑) 正在读取 Runner 日志…</div>
          <div v-else-if="!filteredParsedLogs.length" class="muted loading-tip">(っ˘ω˘ς) 未匹配到相关日志条目</div>
          <div v-else class="runner-log-list">
            <div
              v-for="(item, idx) in filteredParsedLogs"
              :key="idx"
              class="runner-log-item"
              :class="'level-' + item.level"
            >
              <div class="runner-item-main">
                <span class="runner-item-time">{{ item.time || '-' }}</span>
                <span class="runner-level-tag" :class="'tag-' + item.level">{{ item.badgeText }}</span>
                <span v-if="item.jobId" class="runner-item-chip job-chip">Job #{{ item.jobId }}</span>
                <span v-if="item.project" class="runner-item-chip proj-chip">{{ item.project }}</span>
                <span v-if="item.duration" class="runner-item-chip dur-chip">⏱ {{ item.duration }}</span>
                <span class="runner-item-head" v-html="highlightKw(item.headline)"></span>
              </div>
              <div v-if="item.details" class="runner-item-meta">
                <span v-html="highlightKw(item.details)"></span>
              </div>
            </div>
          </div>
        </div>

        <div v-else class="runner-raw-wrap mt">
          <div class="runner-raw-box">
            <div v-if="loadingLogs" class="muted" style="padding:16px;text-align:center">(๑>ᴗ<๑) 正在读取终端日志…</div>
            <div v-else-if="!filteredRawLines.length" class="muted" style="padding:16px;text-align:center">(っ˘ω˘ς) 未匹配到相关终端日志</div>
            <div
              v-for="(line, idx) in filteredRawLines"
              :key="idx"
              class="runner-raw-line"
              :class="getTermLineClass(line)"
              v-html="highlightKw(line)"
            ></div>
          </div>
        </div>
      </div>
    </template>
    <div v-else class="muted loading-tip">(๑•̀ㅂ•́)و✧ 加载中...</div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { toast } from '../ui.js'

const s = ref(null)
const err = ref('')
const runnerLogs = ref('')
const tailN = ref(100)
const pipelines = ref([])
const tip = ref({ show: false, x: 0, y: 0, job: null })
let hideTimer

const viewMode = ref('card')
const filterType = ref('all')
const logKw = ref('')
const loadingLogs = ref(false)
const logCopied = ref(false)

const tipStyle = computed(() => ({
  left: tip.value.x + 'px',
  top: tip.value.y + 'px'
}))

function failText(j) {
  if (!j) return ''
  return (j.failReason && String(j.failReason).trim()) || '未记录到失败原因（可检查 Runner 日志或配置 OPSWEB_GITLAB_TOKEN 拉取 Job Trace）'
}

function placeTip(e) {
  const pad = 14
  const w = 420
  const h = 260
  let x = e.clientX + 16
  let y = e.clientY + 16
  if (x + w > window.innerWidth - pad) x = Math.max(pad, e.clientX - w - 12)
  if (y + h > window.innerHeight - pad) y = Math.max(pad, window.innerHeight - h - pad)
  tip.value.x = x
  tip.value.y = y
}

function showFailTip(e, j) {
  clearTimeout(hideTimer)
  placeTip(e)
  tip.value.job = j
  tip.value.show = true
}

function keepFailTip() {
  clearTimeout(hideTimer)
}

function hideFailTip() {
  clearTimeout(hideTimer)
  hideTimer = setTimeout(() => { tip.value.show = false }, 120)
}

async function copyFailReason(j) {
  const text = failText(j)
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    toast('失败原因已复制 ♡', 'success')
  } catch (e) {
    toast('复制失败: ' + (e && e.message ? e.message : '未知错误'), 'error')
  }
}

function clampTail() {
  const v = Math.floor(Number(tailN.value))
  if (!Number.isFinite(v) || v < 1) { tailN.value = 1; return 1 }
  const c = Math.min(v, 5000)
  tailN.value = c
  return c
}

async function loadRunnerLogs() {
  loadingLogs.value = true
  try {
    runnerLogs.value = await api(`/cicd/runner/logs?tail=${clampTail()}`)
  } catch (e) {
    runnerLogs.value = '获取失败: ' + e.message
  } finally {
    loadingLogs.value = false
  }
}

function cleanAnsi(text) {
  if (!text) return ''
  return text
    .replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '')
    .replace(/[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]/g, '')
}

function parseLogLine(rawLine) {
  const line = cleanAnsi(rawLine).trim()
  if (!line) return null

  let timeStr = ''
  let rest = line
  const tsMatch = line.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+/)
  if (tsMatch) {
    const d = new Date(tsMatch[1])
    if (!isNaN(d.getTime())) {
      const pad = n => String(n).padStart(2, '0')
      timeStr = `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
    } else {
      timeStr = tsMatch[1].slice(5, 19).replace('T', ' ')
    }
    rest = line.slice(tsMatch[0].length).trim()
  }

  const jobM = rest.match(/\bjob=(\d+)\b/)
  const jobId = jobM ? jobM[1] : ''

  const projM = rest.match(/\bproject_full_path=([^\s]+)/)
  const project = projM ? projM[1] : ''

  const durM = rest.match(/\bduration_s=([0-9.]+)/)
  const duration = durM ? (parseFloat(durM[1]).toFixed(1) + 's') : ''

  let level = 'info'
  let badgeText = 'INFO'
  const isErr = /\b(error|fatal|panic)\b/i.test(rest) || (/failed/i.test(rest) && !/Checking for jobs/i.test(rest))
  const isWarn = /\b(warning|warn)\b/i.test(rest)
  const isSucc = /job succeeded/i.test(rest)

  if (isSucc) {
    level = 'success'
    badgeText = '成功'
  } else if (isErr) {
    level = 'error'
    badgeText = '失败'
  } else if (isWarn) {
    level = 'warn'
    badgeText = '警告'
  } else if (/received/i.test(rest)) {
    level = 'received'
    badgeText = '分发'
  } else if (/checking for jobs/i.test(rest)) {
    level = 'poll'
    badgeText = '轮询'
  } else if (/submitting|appending trace|updating job/i.test(rest)) {
    level = 'trace'
    badgeText = '同步'
  }

  const kvStart = rest.search(/\s+[a-zA-Z_][a-zA-Z0-9_-]*=/)
  let headline = kvStart > 0 ? rest.slice(0, kvStart).trim() : rest.trim()
  headline = headline.replace(/\s{2,}/g, ' ')
  let details = kvStart > 0 ? rest.slice(kvStart).trim().replace(/\s{2,}/g, ' ') : ''

  return {
    raw: line,
    time: timeStr,
    level,
    badgeText,
    headline,
    jobId,
    project,
    duration,
    details,
    isKey: isSucc || isErr || isWarn || (jobId !== '')
  }
}

const parsedLogs = computed(() => {
  if (!runnerLogs.value) return []
  return runnerLogs.value
    .split('\n')
    .map(parseLogLine)
    .filter(Boolean)
})

const filteredParsedLogs = computed(() => {
  const kw = logKw.value.trim().toLowerCase()
  return parsedLogs.value.filter(item => {
    if (filterType.value === 'jobs' && !item.isKey) return false
    if (filterType.value === 'errors' && item.level !== 'error' && item.level !== 'warn') return false
    if (kw) {
      const match = item.raw.toLowerCase().includes(kw) ||
                    item.headline.toLowerCase().includes(kw) ||
                    item.project.toLowerCase().includes(kw) ||
                    item.jobId.includes(kw)
      if (!match) return false
    }
    return true
  })
})

const rawLines = computed(() => {
  if (!runnerLogs.value) return []
  return runnerLogs.value
    .split('\n')
    .map(cleanAnsi)
    .filter(l => l.trim() !== '')
})

const filteredRawLines = computed(() => {
  const kw = logKw.value.trim().toLowerCase()
  return rawLines.value.filter(line => {
    if (filterType.value === 'jobs') {
      if (!/\b(job=\d+|succeeded|failed|error|warning)\b/i.test(line)) return false
    }
    if (filterType.value === 'errors') {
      if (!/\b(failed|error|warning|fatal)\b/i.test(line)) return false
    }
    if (kw && !line.toLowerCase().includes(kw)) return false
    return true
  })
})

const escMap = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }
function escHtml(s) {
  return String(s).replace(/[&<>"]/g, c => escMap[c])
}

function highlightKw(str) {
  if (!str) return ''
  const safe = escHtml(str)
  if (!logKw.value.trim()) return safe
  try {
    const k = escHtml(logKw.value.trim()).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    return safe.replace(new RegExp(k, 'gi'), m => `<mark class="kw-hit">${m}</mark>`)
  } catch {
    return safe
  }
}

function getTermLineClass(line) {
  if (/\b(error|fatal|panic)\b/i.test(line) || (/failed/i.test(line) && !/checking for jobs/i.test(line))) return 'term-err'
  if (/\b(warning|warn)\b/i.test(line)) return 'term-warn'
  if (/job succeeded/i.test(line)) return 'term-succ'
  if (/\bjob=\d+\b/i.test(line)) return 'term-job'
  return ''
}

async function copyAllRunnerLogs() {
  const text = runnerLogs.value || ''
  if (!text) return
  const done = () => {
    logCopied.value = true
    toast('Runner 日志已复制', 'success')
    setTimeout(() => { logCopied.value = false }, 1500)
  }
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      done()
    } catch {
      fallbackCopy(text, done)
    }
  } else {
    fallbackCopy(text, done)
  }
}

function fallbackCopy(text, done) {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  try { document.execCommand('copy'); done() } catch { /* ignore */ }
  ta.remove()
}

onMounted(async () => {
  try {
    s.value = await api('/cicd/summary')
    loadRunnerLogs()
    pipelines.value = await api('/cicd/pipelines')
  } catch (e) { err.value = '加载失败: ' + e.message }
})

onUnmounted(() => clearTimeout(hideTimer))
</script>

<style scoped>
.ci-failed-row { cursor: pointer; }
.ci-failed-row:hover td { background: rgba(251, 122, 158, 0.12); }

.log-btn-group {
  display: inline-flex;
  border-radius: 999px;
  background: var(--panel2);
  border: 1px solid var(--border);
  padding: 2px;
  gap: 2px;
}
.log-mode-btn, .log-filter-btn {
  background: transparent;
  border: none;
  border-radius: 999px;
  padding: 3px 10px;
  font-size: 12px;
  color: var(--muted);
  cursor: pointer;
  transition: all 0.2s ease;
}
.log-mode-btn.active, .log-filter-btn.active {
  background: var(--panel-solid);
  color: var(--accent-deep);
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}
.log-search-wrap {
  display: flex;
  align-items: center;
  gap: 4px;
  background: var(--panel2);
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 2px 8px;
}
.log-search-wrap input {
  border: none;
  background: transparent;
  outline: none;
  font-size: 12px;
  color: var(--text);
}
.log-clear-btn {
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--muted);
  font-size: 11px;
}
.log-select {
  background: var(--panel2);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text);
  font-size: 12px;
  padding: 2px 6px;
}

.runner-card-wrap {
  max-height: 520px;
  overflow-y: auto;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--panel-solid);
  padding: 8px;
}
.runner-log-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.runner-log-item {
  display: flex;
  flex-direction: column;
  padding: 7px 10px;
  border-radius: 8px;
  border-left: 3px solid rgba(200, 200, 200, 0.3);
  background: rgba(255, 255, 255, 0.5);
  font-size: 12px;
  transition: background 0.15s ease;
}
.runner-log-item:hover {
  background: rgba(255, 255, 255, 0.95);
}
.runner-log-item.level-success {
  border-left-color: #3bb273;
  background: rgba(59, 178, 115, 0.05);
}
.runner-log-item.level-error {
  border-left-color: #e8537f;
  background: rgba(232, 83, 127, 0.08);
}
.runner-log-item.level-warn {
  border-left-color: #f1a23a;
  background: rgba(241, 162, 58, 0.08);
}
.runner-log-item.level-received {
  border-left-color: #9254de;
  background: rgba(146, 84, 222, 0.05);
}
.runner-log-item.level-trace, .runner-log-item.level-poll {
  opacity: 0.85;
}

.runner-item-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  line-height: 1.4;
}
.runner-item-time {
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 11px;
  color: var(--muted);
  white-space: nowrap;
}
.runner-level-tag {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 4px;
  text-transform: uppercase;
  white-space: nowrap;
}
.tag-success { background: #d7f5e3; color: #1e7e46; }
.tag-error { background: #fed7e2; color: #b81d48; }
.tag-warn { background: #feecd2; color: #9c5409; }
.tag-received { background: #efe2fe; color: #531dab; }
.tag-poll { background: #e6f4ff; color: #0958d9; }
.tag-trace, .tag-info { background: #f0f0f0; color: #595959; }

.runner-item-chip {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 4px;
  white-space: nowrap;
  font-family: Consolas, "JetBrains Mono", monospace;
}
.job-chip { background: #e8d7fa; color: #5b21b6; font-weight: 600; }
.proj-chip { background: #fde8e8; color: #991b1b; }
.dur-chip { background: #fef3c7; color: #92400e; font-weight: 600; }

.runner-item-head {
  font-weight: 600;
  color: var(--text);
  word-break: break-word;
}
.runner-item-meta {
  margin-top: 4px;
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 11px;
  color: #887e96;
  white-space: pre-wrap;
  word-break: break-word;
  padding: 3px 6px;
  background: rgba(0, 0, 0, 0.03);
  border-radius: 4px;
}

.runner-raw-wrap {
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid rgba(201, 168, 245, 0.35);
}
.runner-raw-box {
  background: linear-gradient(160deg, #322846, #231c33);
  padding: 12px;
  max-height: 520px;
  overflow: auto;
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 12px;
  line-height: 1.6;
  color: #f3d9ff;
  white-space: pre;
  box-shadow: inset 0 2px 10px rgba(0, 0, 0, 0.25);
}
.runner-raw-line {
  display: block;
  white-space: pre;
  padding: 0 4px;
}
.runner-raw-line:hover {
  background: rgba(255, 255, 255, 0.06);
}
.term-err { color: #ff9db1; font-weight: 600; }
.term-warn { color: #ffd479; }
.term-succ { color: #86efac; font-weight: 600; }
.term-job { color: #d8b4fe; }

:deep(.kw-hit) {
  background: rgba(255, 214, 102, 0.9);
  color: #3f2a00;
  border-radius: 2px;
  padding: 0 2px;
}
</style>

<style>
.ci-fail-tip {
  position: fixed;
  z-index: 1200;
  width: 420px;
  max-width: calc(100vw - 28px);
  max-height: 260px;
  display: flex;
  flex-direction: column;
  background: var(--panel-solid);
  border: 1px solid rgba(251, 122, 158, 0.45);
  border-radius: 14px;
  box-shadow: var(--shadow);
  overflow: hidden;
}
.ci-fail-tip-hd {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  font-size: 12px;
  font-weight: 600;
  color: #e8537f;
  background: rgba(251, 122, 158, 0.08);
  border-bottom: 1px solid rgba(251, 122, 158, 0.2);
}
.ci-fail-tip-body {
  margin: 0;
  padding: 10px 12px;
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-all;
  overflow: auto;
  color: var(--text);
  background: #fffafc;
  user-select: text;
}
</style>
