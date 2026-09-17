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
        <div v-if="pipelines.length">
          <table class="mt">
            <thead><tr><th>Job</th><th>项目</th><th>结果</th><th>耗时</th><th>完成时间</th></tr></thead>
            <tbody>
              <tr v-for="j in pipelines" :key="j.jobId">
                <td>#{{ j.jobId }}</td>
                <td>{{ j.project }}</td>
                <td><span class="badge" :class="j.status==='success' ? 'green' : 'red'">{{ j.status === 'success' ? '✧ 成功' : '× 失败' }}</span></td>
                <td>{{ j.duration || '-' }}</td>
                <td class="muted">{{ j.finishedAt || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="muted mt loading-tip">(っ˘ω˘ς) 暂无构建记录</div>
      </div>

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
import { ref, onMounted } from 'vue'
import { api } from '../api.js'

const s = ref(null)
const err = ref('')
const runnerLogs = ref('')
const tailN = ref(100)
const pipelines = ref([])

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
</script>
