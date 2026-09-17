<template>
  <div>
    <h2 class="page-title">服务器总览 ♪</h2>
    <div v-if="err" class="error-msg">(｡•́︿•̀｡) {{ err }}</div>
    <template v-else-if="o">
      <div class="grid grid-4">
        <div class="card">
          <div class="stat-label">CPU 使用率</div>
          <div class="stat-value" :style="{color: pctColor(o.cpuPercent)}">{{ o.cpuPercent.toFixed(1) }}%</div>
          <div class="progress"><div :style="{width: o.cpuPercent+'%', background: pctColor(o.cpuPercent)}"></div></div>
          <div class="muted mt">{{ o.cpuCores }} 核 · {{ o.load1.toFixed(2) }} / {{ o.load5.toFixed(2) }} / {{ o.load15.toFixed(2) }}</div>
        </div>
        <div class="card">
          <div class="stat-label">内存使用率</div>
          <div class="stat-value" :style="{color: pctColor(o.memPercent)}">{{ o.memPercent.toFixed(1) }}%</div>
          <div class="progress"><div :style="{width: o.memPercent+'%', background: pctColor(o.memPercent)}"></div></div>
          <div class="muted mt">{{ fmtBytes(o.memUsed) }} / {{ fmtBytes(o.memTotal) }}</div>
        </div>
        <div class="card">
          <div class="stat-label">容器运行</div>
          <div class="stat-value" :style="{color: 'var(--accent)'}">{{ o.runningCount }} / {{ o.containerCount }}</div>
          <div class="muted mt">Docker 容器 running / total</div>
        </div>
        <div class="card">
          <div class="stat-label">运行时长</div>
          <div class="stat-value">{{ fmtUptime(o.uptimeSec) }}</div>
          <div class="muted mt">{{ o.platform }}</div>
        </div>
      </div>

      <div class="grid grid-2 mt">
        <div class="card">
          <div class="stat-label">磁盘使用</div>
          <table class="mt">
            <thead><tr><th>挂载点</th><th>文件系统</th><th>容量</th><th>已用</th><th>使用率</th></tr></thead>
            <tbody>
              <tr v-for="d in disks" :key="d.mountpoint">
                <td>{{ d.mountpoint }}</td>
                <td class="muted">{{ d.device }}</td>
                <td>{{ fmtBytes(d.total) }}</td>
                <td>{{ fmtBytes(d.used) }}</td>
                <td><span class="badge" :class="d.usedPct>=90?'red':d.usedPct>=75?'yellow':'green'">{{ d.usedPct.toFixed(0) }}%</span></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="card">
          <div class="stat-label">网络 IO（实时速率）</div>
          <table class="mt">
            <thead><tr><th>网卡</th><th>接收速率</th><th>发送速率</th><th>累计收/发</th></tr></thead>
            <tbody>
              <tr v-for="n in activeNets" :key="n.name">
                <td>{{ n.name }}</td>
                <td>{{ n.rateRecvKBs.toFixed(1) }} KB/s</td>
                <td>{{ n.rateSentKBs.toFixed(1) }} KB/s</td>
                <td class="muted">{{ fmtBytes(n.bytesRecv) }} / {{ fmtBytes(n.bytesSent) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="card mt">
        <div style="display:flex;justify-content:space-between;align-items:center">
          <div class="stat-label">CPU / 内存 采样曲线（每分钟，重启不丢失）</div>
          <select v-model.number="days" @change="changeDays" style="background:#fff;border:1.5px solid var(--border);color:var(--text);border-radius:10px;padding:5px 10px">
            <option :value="1">最近 24 小时</option>
            <option :value="3">最近 3 天</option>
            <option :value="7">最近 7 天</option>
            <option :value="30">最近 30 天</option>
          </select>
        </div>
        <div v-if="hist.cpu.length" class="mt">
          <svg viewBox="0 0 800 180" style="width:100%;height:180px" @mousemove="histHover" @mouseleave="histHoverIdx = null">
            <g v-for="i in 4" :key="'g'+i">
              <line :x1="0" :y1="i * 34" :x2="800" :y2="i * 34" stroke="rgba(183,148,246,0.15)" stroke-width="1" />
              <text :x="4" :y="i * 34 - 4" font-size="10" fill="var(--muted)">{{ 100 - i * 25 }}%</text>
            </g>
            <g v-if="histSpan() > 86400">
              <line v-for="t in histTicks" :key="'t'+t" :x1="histX(t)" :y1="6" :x2="histX(t)" :y2="170" stroke="rgba(183,148,246,0.12)" stroke-width="1" />
              <text v-for="t in histTicks" :key="'tl'+t" :x="histX(t) + 4" :y="178" font-size="10" fill="var(--muted)">{{ histTickLabel(t) }}</text>
            </g>
            <polyline :points="histPath(hist.cpu)" fill="none" stroke="#ff7eb6" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
            <polyline :points="histPath(hist.mem)" fill="none" stroke="#b794f6" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
            <g v-if="histHoverIdx !== null && hist.cpu[histHoverIdx]">
              <line :x1="histX(hist.cpu[histHoverIdx].t)" y1="0" :x2="histX(hist.cpu[histHoverIdx].t)" y2="180" stroke="rgba(255,126,182,0.4)" stroke-dasharray="4 3" />
              <circle :cx="histX(hist.cpu[histHoverIdx].t)" :cy="histY(hist.cpu[histHoverIdx].v)" r="4" fill="#ff7eb6" />
              <circle v-if="hist.mem[histHoverIdx]" :cx="histX(hist.mem[histHoverIdx].t)" :cy="histY(hist.mem[histHoverIdx].v)" r="4" fill="#b794f6" />
            </g>
          </svg>
          <div class="muted">
            🌸 CPU%　💜 内存%
            <span v-if="histHoverIdx !== null && hist.cpu[histHoverIdx]" style="margin-left:12px">
              {{ histTimeLabel(hist.cpu[histHoverIdx].t) }}　
              CPU {{ hist.cpu[histHoverIdx].v.toFixed(1) }}% · 
              内存 {{ hist.mem[histHoverIdx] ? hist.mem[histHoverIdx].v.toFixed(1) : '-' }}%
            </span>
          </div>
        </div>
        <div v-else class="muted mt loading-tip">(っ˘ω˘ς) 采样数据累积中，稍后刷新就有曲线啦～</div>
      </div>

      <div class="card mt">
        <div class="stat-label">主机信息</div>
        <div class="grid grid-3 mt">
          <div>主机名: {{ o.hostname }}</div>
          <div>内核: {{ o.kernel }}</div>
          <div>CPU: {{ o.cpuModel }}</div>
        </div>
      </div>
    </template>
    <div v-else class="muted loading-tip">(๑•̀ㅂ•́)و✧ 拼命加载中...</div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api, fmtBytes, fmtUptime, pctColor, onVisible } from '../api.js'

const o = ref(null)
const disks = ref([])
const nets = ref([])
const hist = ref({ cpu: [], mem: [] })
const err = ref('')
const days = ref(1)
let timer
let offVisible

async function load() {
  try {
    const [ov, dk, nt] = await Promise.all([api('/overview'), api('/disk'), api('/net')])
    o.value = ov; disks.value = dk || []; nets.value = nt || []
    err.value = ''
  } catch (e) { err.value = '加载失败: ' + e.message }
}

async function loadHistory() {
  try {
    hist.value = await api(`/load/history?days=${days.value}`)
  } catch { /* 曲线加载失败不打断页面 */ }
}

function changeDays() { loadHistory() }

const histHoverIdx = ref(null)

function histSpan() {
  const a = hist.value.cpu
  if (!a || a.length < 2) return 0
  return a[a.length - 1].t - a[0].t
}

function histX(t) {
  const a = hist.value.cpu
  if (!a || !a.length) return 0
  const t0 = a[0].t, span = Math.max(histSpan(), 60)
  return ((t - t0) / span) * 800
}

function histY(v) {
  return 180 - (Math.min(v, 100) / 100) * 170
}

function histPath(arr) {
  if (!arr || !arr.length) return ''
  return arr.map(p => `${histX(p.t).toFixed(1)},${histY(p.v).toFixed(1)}`).join(' ')
}

const histTicks = computed(() => {
  const a = hist.value.cpu
  if (!a || a.length < 2 || histSpan() <= 86400) return []
  const span = histSpan()
  const step = span > 86400 * 15 ? 86400 * 5 : span > 86400 * 7 ? 86400 : 86400 / 2
  const first = Math.ceil(a[0].t / step) * step
  const ticks = []
  for (let t = first; t <= a[a.length - 1].t; t += step) ticks.push(t)
  return ticks.slice(0, 12)
})

function histTickLabel(t) {
  const d = new Date(t * 1000)
  const day = `${d.getMonth() + 1}/${d.getDate()}`
  const hh = String(d.getHours()).padStart(2, '0')
  if (histSpan() > 86400 * 7) return day
  return `${day} ${hh}时`
}

function histTimeLabel(t) {
  const d = new Date(t * 1000)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  if (histSpan() > 86400) return `${d.getMonth() + 1}/${d.getDate()} ${hh}:${mm}`
  return `${hh}:${mm}`
}

function histHover(e) {
  const a = hist.value.cpu
  if (!a || a.length < 2) { histHoverIdx.value = null; return }
  const rect = e.currentTarget.getBoundingClientRect()
  const ratio = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width))
  const t = a[0].t + ratio * Math.max(histSpan(), 60)
  let lo = 0, hi = a.length - 1
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (a[mid].t < t) lo = mid + 1
    else hi = mid
  }
  histHoverIdx.value = lo
}

const activeNets = computed(() =>
  (nets.value || []).filter(n => n.bytesRecv > 0 && !/^(lo|veth|br-|docker)/.test(n.name))
)

onMounted(() => {
  load()
  loadHistory()
  timer = setInterval(() => { if (!document.hidden) load() }, 5000)
  offVisible = onVisible(load)
})
onUnmounted(() => {
  clearInterval(timer)
  offVisible()
})
</script>
