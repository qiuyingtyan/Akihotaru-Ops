<template>
  <div>
    <h2 class="page-title">系统服务 ♪</h2>
    <div v-if="err" class="error-msg">(｡•́︿•̀｡) {{ err }}</div>
    <div v-else class="card">
      <table>
        <thead><tr><th>服务</th><th>描述</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="sv in list" :key="sv.name">
            <td>{{ sv.name }}</td>
            <td class="muted">{{ sv.desc }}</td>
            <td>
              <span class="badge" :class="sv.active==='active' ? 'green' : sv.active==='failed' ? 'red' : 'gray'">{{ sv.active }}</span>
            </td>
            <td>
              <button class="btn" @click="act(sv,'restart')" :disabled="busy===sv.name">重启</button>
              <button class="btn danger" @click="act(sv,'stop')" v-if="sv.active==='active'" :disabled="busy===sv.name">停止</button>
              <button class="btn" @click="act(sv,'start')" v-else :disabled="busy===sv.name">启动</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card mt">
      <div class="stat-label">资源占用 TOP（按 CPU）</div>
      <table class="mt">
        <thead><tr><th>PID</th><th>进程</th><th>CPU%</th><th>内存%</th><th>RSS</th></tr></thead>
        <tbody>
          <tr v-for="p in procs" :key="p.pid">
            <td>{{ p.pid }}</td>
            <td>{{ p.name }}</td>
            <td>{{ p.cpuPercent.toFixed(1) }}</td>
            <td>{{ p.memPercent.toFixed(1) }}</td>
            <td class="muted">{{ p.rssMB }} MB</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api, onVisible } from '../api.js'
import { confirmDialog, toast } from '../ui.js'

const list = ref([])
const procs = ref([])
const err = ref('')
const busy = ref('')
let timer
let offVisible

async function load() {
  try {
    procs.value = await api('/processes')
  } catch (e) { /* ignore */ }
}

async function loadServices() {
  try { list.value = await api('/services'); err.value = '' }
  catch (e) { err.value = '加载失败: ' + e.message }
}

async function act(sv, action) {
  if (action === 'stop') {
    const ok = await confirmDialog({
      title: '停止系统服务',
      message: `确定要停止系统服务 ${sv.name} 吗？这可能导致相关业务不可用！`,
      danger: true,
      okText: '停止'
    })
    if (!ok) return
  }
  if (action === 'restart') {
    const ok = await confirmDialog({
      title: '重启系统服务',
      message: `确定要重启系统服务 ${sv.name} 吗？`,
      okText: '重启'
    })
    if (!ok) return
  }
  busy.value = sv.name
  try {
    await api(`/services/${sv.name}/${action}`, { method: 'POST' })
    toast('操作成功 ♡', 'success')
    setTimeout(loadServices, 1000)
  } catch (e) { toast('操作失败: ' + e.message, 'error') }
  busy.value = ''
}

onMounted(() => {
  loadServices()
  load()
  timer = setInterval(() => { if (!document.hidden) load() }, 5000)
  offVisible = onVisible(load)
})
onUnmounted(() => { clearInterval(timer); offVisible() })
</script>
