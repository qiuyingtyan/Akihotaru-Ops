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
        <div style="display:flex;justify-content:space-between;align-items:center">
          <div class="stat-label">Runner 日志（最近 {{ tailN }} 行）</div>
          <span>
            <input v-model.number="tailN" style="width:60px;background:var(--panel2);border:1.5px solid var(--border);color:var(--text);border-radius:10px;padding:4px 8px" />
            <button class="btn" @click="loadRunnerLogs">刷新</button>
          </span>
        </div>
        <pre class="log mt">{{ runnerLogs || '（暂无）' }}</pre>
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
  try { runnerLogs.value = await api(`/cicd/runner/logs?tail=${clampTail()}`) }
  catch (e) { runnerLogs.value = '获取失败: ' + e.message }
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
