<template>
  <div>
    <h2 class="page-title">日志查看</h2>
    <div class="card">
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
        <input v-model="path" placeholder="/workspace/baq-test/logs/..." style="flex:1;background:var(--panel2);border:1px solid var(--border);color:var(--text);border-radius:6px;padding:6px 10px;font-family:monospace" @keyup.enter="loadFile" />
        <input v-model.number="tail" style="width:70px;background:var(--panel2);border:1px solid var(--border);color:var(--text);border-radius:6px;padding:6px 10px" title="行数" />
        <button class="btn" @click="loadFile">查看</button>
      </div>
      <div class="muted mt" style="font-size:12px">
        允许目录: /workspace/baq-test/logs, /workspace/szx-test/logs, /workspace/baq-test/.deploy, /var/log
      </div>
      <div v-if="err" class="error-msg mt">{{ err }}</div>
      <div v-else-if="result">
        <div class="muted mt">{{ result.path }}（{{ result.lines.length }} 行）</div>
        <pre class="log mt">{{ result.lines.join('\n') }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '../api.js'

const path = ref('/workspace/baq-test/logs/main/')
const tail = ref(200)
const result = ref(null)
const err = ref('')

async function loadFile() {
  err.value = ''
  try {
    result.value = await api(`/logs/file?path=${encodeURIComponent(path.value)}&tail=${tail.value}`)
  } catch (e) { err.value = e.message; result.value = null }
}
</script>
