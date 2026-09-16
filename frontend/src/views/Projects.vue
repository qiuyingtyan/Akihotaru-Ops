<template>
  <div>
    <h2 class="page-title">项目状态 ♪</h2>
    <div v-if="err" class="error-msg">(｡•́︿•̀｡) {{ err }}</div>
    <div v-else class="grid grid-2">
      <div class="card" v-for="p in list" :key="p.name">
        <div style="display:flex;justify-content:space-between;align-items:center">
          <strong style="font-size:16px">{{ p.name }}</strong>
          <span class="badge blue">{{ p.kind }}</span>
        </div>
        <div class="muted mt" style="font-size:13px">路径: {{ p.path }}</div>
        <div class="muted" style="font-size:13px">磁盘占用: {{ p.diskUsageMB }} MB</div>
        <div v-if="p.lastDeploy" class="muted" style="font-size:13px">最近备份/发布: {{ p.lastDeploy }}</div>
        <div class="mt">
          <div v-for="c in p.containers" :key="c" style="padding:3px 0;font-size:13px">
            {{ c }}
          </div>
        </div>
        <div class="mt" style="display:flex;gap:8px">
          <button class="btn primary" @click="deploy(p)" :disabled="deploying===p.name">
            {{ deploying===p.name ? '部署中...' : '✨ 重新部署' }}
          </button>
          <button class="btn" @click="showDeployStatus(p)">部署输出</button>
        </div>
      </div>
    </div>

    <div v-if="deployLog.project" class="card mt">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <strong>部署输出: {{ deployLog.project }}</strong>
        <span>
          <button class="btn" @click="showDeployStatus(deployLog.p)">刷新</button>
          <button class="btn" @click="deployLog.project=''">关闭</button>
        </span>
      </div>
      <pre class="log mt">{{ deployLog.text }}</pre>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'

const list = ref([])
const err = ref('')
const deploying = ref('')
const deployLog = ref({ project: '', text: '', p: null })
let timer

async function load() {
  try { list.value = await api('/projects') }
  catch (e) { err.value = '加载失败: ' + e.message }
}

async function deploy(p) {
  if (!confirm(`确定要重新部署「${p.name}」吗？部署过程可能需要几分钟。`)) return
  deploying.value = p.name
  deployLog.value = { project: p.name, text: '部署已发起，等待输出...', p }
  try {
    const out = await api(`/projects/${encodeURIComponent(p.name)}/deploy`, { method: 'POST' })
    deployLog.value.text = out || '（无输出）部署完成'
    setTimeout(load, 1500)
  } catch (e) {
    deployLog.value.text = '部署失败: ' + e.message
  }
  deploying.value = ''
}

async function showDeployStatus(p) {
  try {
    deployLog.value = { project: p.name, text: await api(`/projects/${encodeURIComponent(p.name)}/deploy-status`), p }
  } catch (e) { alert('获取失败: ' + e.message) }
}

onMounted(() => { load(); timer = setInterval(() => { if (!document.hidden) load() }, 15000) })
onUnmounted(() => clearInterval(timer))
</script>
