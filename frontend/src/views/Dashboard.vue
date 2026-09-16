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
          <svg viewBox="0 0 800 180" style="width:100%;height:180px">
            <polyline :points="histPath(hist.cpu)" fill="none" stroke="#ff7eb6" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
            <polyline :points="histPath(hist.mem)" fill="none" stroke="#b794f6" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <div class="muted">🌸 CPU%　💜 内存%</div>
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
import { api, fmtBytes, fmtUptime, pctColor } from '../api.js'

const o = ref(null)
const disks = ref([])
const nets = ref([])
const hist = ref({ cpu: [], mem: [] })
const err = ref('')
const days = ref(1)
let timer

async function load() {
  try {
    const [ov, dk, nt, hs] = await Promise.all([
      api('/overview'), api('/disk'), api('/net'), api(`/load/history?days=${days.value}`)
    ])
    o.value = ov; disks.value = dk || []; nets.value = nt || []; hist.value = hs
    err.value = ''
  } catch (e) { err.value = '加载失败: ' + e.message }
}

function changeDays() { load() }

const activeNets = computed(() =>
  (nets.value || []).filter(n => n.bytesRecv > 0 && !/^(lo|veth|br-|docker)/.test(n.name))
)

function histPath(arr) {
  if (!arr.length) return ''
  const t0 = arr[0].t, t1 = arr[arr.length-1].t
  const span = Math.max(t1 - t0, 60)
  return arr.map(p => {
    const x = ((p.t - t0) / span) * 800
    const y = 180 - (Math.min(p.v, 100) / 100) * 170
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
}

onMounted(() => { load(); timer = setInterval(() => { if (!document.hidden) load() }, 5000) })
onUnmounted(() => clearInterval(timer))
</script>
