<template>
  <div>
    <h2 class="page-title">Docker 容器</h2>
    <div v-if="err" class="error-msg">{{ err }}</div>
    <div v-else class="card">
      <table>
        <thead>
          <tr><th>名称</th><th>镜像</th><th>状态</th><th>端口</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="c in list" :key="c.name">
            <td>{{ c.name }}</td>
            <td class="muted">{{ c.image }}</td>
            <td><span class="badge" :class="stateClass(c.state)">{{ c.state }}</span> <span class="muted">{{ c.status }}</span></td>
            <td class="muted">{{ (c.ports||[]).join(', ') || '-' }}</td>
            <td>
              <button class="btn" @click="act(c, 'restart')" :disabled="busy===c.name">重启</button>
              <button class="btn" @click="act(c, 'stop')" v-if="c.state==='running'" :disabled="busy===c.name">停止</button>
              <button class="btn" @click="act(c, 'start')" v-else :disabled="busy===c.name">启动</button>
              <button class="btn" @click="showLogs(c)">日志</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="logBox" class="card mt">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <strong>日志: {{ logBox.name }}</strong>
        <span>
          <button class="btn" @click="showLogs(logBox.c)">刷新</button>
          <button class="btn" @click="logBox=null">关闭</button>
        </span>
      </div>
      <pre class="log mt">{{ logBox.text || '（无日志）' }}</pre>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'

const list = ref([])
const err = ref('')
const busy = ref('')
const logBox = ref(null)
let timer

async function load() {
  try { list.value = await api('/docker/containers'); err.value = '' }
  catch (e) { err.value = '加载失败: ' + e.message }
}

async function act(c, action) {
  if (action === 'stop' || action === 'remove') {
    if (!confirm(`确定要${action === 'stop' ? '停止' : '删除'}容器 ${c.name} 吗？`)) return
  }
  busy.value = c.name
  try {
    await api(`/docker/containers/${c.name}/${action}`, { method: 'POST' })
    setTimeout(load, 800)
  } catch (e) { alert('操作失败: ' + e.message) }
  busy.value = ''
}

async function showLogs(c) {
  try {
    const text = await api(`/docker/container/${c.name}/logs?tail=300`)
    logBox.value = { name: c.name, text, c }
  } catch (e) { alert('获取日志失败: ' + e.message) }
}

function stateClass(s) {
  if (s === 'running') return 'green'
  if (s === 'exited') return 'gray'
  if (s === 'restarting') return 'yellow'
  return 'red'
}

onMounted(() => { load(); timer = setInterval(() => { if (!document.hidden) load() }, 8000) })
onUnmounted(() => clearInterval(timer))
</script>
