<template>
  <div>
    <h2 class="page-title">告警</h2>
    <div class="grid grid-2">
      <div class="card">
        <div class="stat-label">活跃告警（{{ active.length }}）</div>
        <div v-if="!active.length" class="muted mt">✅ 当前无活跃告警</div>
        <table v-else class="mt">
          <thead><tr><th>时间</th><th>级别</th><th>指标</th><th>当前值</th></tr></thead>
          <tbody>
            <tr v-for="(a, i) in active" :key="i">
              <td class="muted">{{ fmtTime(a.time) }}</td>
              <td><span class="badge" :class="a.level === 'critical' ? 'red' : 'yellow'">{{ a.level }}</span></td>
              <td>{{ a.detail }}</td>
              <td>{{ a.value.toFixed(1) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="card">
        <div class="stat-label">最近事件（触发/恢复）</div>
        <div v-if="!recent.length" class="muted mt">暂无历史事件</div>
        <table v-else class="mt">
          <thead><tr><th>时间</th><th>状态</th><th>级别</th><th>指标</th><th>值</th></tr></thead>
          <tbody>
            <tr v-for="(a, i) in recent.slice().reverse().slice(0, 30)" :key="i">
              <td class="muted">{{ fmtTime(a.time) }}</td>
              <td><span class="badge" :class="a.active ? 'red' : 'green'">{{ a.active ? '触发' : '恢复' }}</span></td>
              <td>{{ a.level }}</td>
              <td>{{ a.detail }}</td>
              <td>{{ a.value.toFixed(1) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="card mt">
      <div class="stat-label">告警规则</div>
      <div class="muted mt" style="font-size:13px;line-height:1.8">
        磁盘 / 或 /workspace 使用率 &gt; 80% 警告、&gt; 90% 严重；<br>
        内存使用率 &gt; 80% 警告、&gt; 90% 严重；<br>
        load1 &gt; 16（20核机器约80%负载）警告；<br>
        异常容器（unhealthy/dead）&gt; 0 严重；<br>
        failed 状态 systemd 服务 &gt; 0 严重。<br>
        配置环境变量 <code>OPSWEB_WEBHOOK</code>（企业微信/钉钉机器人地址）可启用 webhook 通知。
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'

const active = ref([])
const recent = ref([])
let timer

function fmtTime(t) {
  if (!t) return '-'
  const d = new Date(t)
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
}

async function load() {
  try {
    const a = await api('/alerts')
    active.value = a.active
    recent.value = a.recent
  } catch { /* ignore */ }
}

onMounted(() => { load(); timer = setInterval(() => { if (!document.hidden) load() }, 30000) })
onUnmounted(() => clearInterval(timer))
</script>
