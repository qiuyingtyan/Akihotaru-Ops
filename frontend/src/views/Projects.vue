<template>
  <div>
    <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:12px">
      <h2 class="page-title" style="margin-bottom:0">🚀 业务服务与灾备治理 ♪</h2>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button class="btn" @click="runSelfCheck">🩺 开机/断电自检</button>
        <button class="btn primary" @click="batchAction('restart')" :disabled="batchLoading">
          {{ batchLoading ? '执行中...' : '🔄 一键依序重启全量服务' }}
        </button>
        <button class="btn" @click="batchAction('start')" :disabled="batchLoading">
          {{ batchLoading ? '启动中...' : '⚡ 一键依序拉起' }}
        </button>
        <button class="btn" @click="syncSystemd">⚙️ 同步 Systemd 单元</button>
        <button class="btn" @click="exportBundle">📦 导出生产交付包</button>
        <button class="btn primary" @click="openModal()">➕ 纳管新服务</button>
      </div>
    </div>

    <div v-if="selfCheck" class="card mt" :style="{ borderLeft: selfCheck.allHealthy ? '5px solid var(--green)' : '5px solid var(--red)' }">
      <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px">
        <div>
          <span style="font-size:16px;font-weight:700">
            {{ selfCheck.allHealthy ? '✨ 系统健康与开机自愈正常' : '⚠️ 存在未就绪或异常微服务' }}
          </span>
          <span class="muted" style="margin-left:12px;font-size:13px">
            主机运行时间: {{ selfCheck.uptimeFormatted }} · 纳管总数: {{ selfCheck.totalCount }} · 健康: {{ selfCheck.healthyCount }} · 未就绪: {{ selfCheck.unhealthyCount }}
          </span>
        </div>
        <div v-if="!selfCheck.allHealthy">
          <button class="btn danger" style="padding:4px 10px;font-size:12px" @click="batchAction('start')">
            🚨 一键依序自愈拉起全部异常服务
          </button>
        </div>
      </div>
    </div>

    <div v-if="err" class="error-msg mt">(｡•́︿•̀｡) {{ err }}</div>

    <div class="tabs mt" style="margin-bottom:12px">
      <button v-for="g in groupList" :key="g" :class="['tab', curGroup === g ? 'active' : '']" @click="curGroup = g">
        {{ g === 'all' ? '🌸 全部服务' : g }} ({{ countByGroup(g) }})
      </button>
    </div>

    <div class="grid grid-2">
      <div class="card" v-for="s in filteredServices" :key="s.name">
        <div style="display:flex;justify-content:space-between;align-items:flex-start">
          <div>
            <strong style="font-size:16px">{{ s.displayName }}</strong>
            <span class="muted" style="font-size:12px;margin-left:8px">ops-{{ s.name }}.service</span>
          </div>
          <div style="display:flex;gap:6px;align-items:center">
            <span class="badge" :class="s.level === 1 ? 'red' : s.level === 2 ? 'yellow' : 'blue'">
              L{{ s.level }} {{ s.level === 1 ? '基础' : s.level === 2 ? '核心' : '业务' }}
            </span>
            <span class="badge" :class="s.status === 'active' ? 'green' : s.status === 'failed' ? 'red' : 'gray'">
              {{ s.status === 'active' ? '● 运行中' : s.status === 'failed' ? '✖ 异常' : '○ 已停止' }}
            </span>
          </div>
        </div>

        <div style="display:flex;gap:12px;flex-wrap:wrap;margin-top:10px;font-size:13px">
          <div>
            <span class="muted">端口:</span>
            <span class="badge" :class="s.portListening ? 'green' : s.port > 0 ? 'red' : 'gray'" style="margin-left:4px">
              {{ s.port > 0 ? `${s.port} (${s.portListening ? '通' : '未监听'})` : '无' }}
            </span>
          </div>
          <div>
            <span class="muted">PID:</span>
            <strong style="margin-left:4px">{{ s.pid > 0 ? s.pid : '-' }}</strong>
          </div>
          <div>
            <span class="muted">CPU:</span>
            <span style="margin-left:4px">{{ s.cpuPercent > 0 ? s.cpuPercent.toFixed(1) + '%' : '0%' }}</span>
          </div>
          <div>
            <span class="muted">内存:</span>
            <span style="margin-left:4px">{{ s.rssMB > 0 ? s.rssMB + ' MB' : '-' }}</span>
          </div>
          <div>
            <span class="muted">自启:</span>
            <span class="badge" :class="s.autoStart ? 'blue' : 'gray'" style="margin-left:4px">
              {{ s.autoStart ? '开机自启' : '手动' }}
            </span>
          </div>
        </div>

        <div class="muted mt" style="font-size:12px;word-break:break-all">
          <div>目录: {{ s.workDir || '-' }}</div>
          <div style="margin-top:2px">启动: {{ s.execStart }}</div>
          <div v-if="s.logPath" style="margin-top:2px">日志: {{ s.logPath }}</div>
          <div v-if="s.afterDeps" style="margin-top:2px">依赖: {{ s.afterDeps }}</div>
        </div>

        <div class="mt" style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px">
          <div style="display:flex;gap:6px">
            <button class="btn primary" @click="act(s, 'restart')" :disabled="busy===s.name">重启</button>
            <button class="btn danger" v-if="s.status==='active'" @click="act(s, 'stop')" :disabled="busy===s.name">停止</button>
            <button class="btn" v-else @click="act(s, 'start')" :disabled="busy===s.name">启动</button>
            <button class="btn" v-if="s.logPath" @click="viewLog(s.logPath)">📄 日志</button>
          </div>
          <div style="display:flex;gap:6px">
            <button class="btn" @click="openModal(s)">编辑</button>
            <button class="btn danger" @click="deleteService(s)">删除</button>
          </div>
        </div>
      </div>

      <div v-if="filteredServices.length === 0" class="card muted" style="grid-column:1/-1;text-align:center;padding:32px">
        (๑>ᴗ<๑) 当前分组暂无纳管的业务服务，点击右上角「➕ 纳管新服务」添加吧～
      </div>
    </div>

    <div class="card mt" style="margin-top:28px">
      <div style="display:flex;justify-content:space-between;align-items:center;cursor:pointer" @click="showLegacy = !showLegacy">
        <strong>📦 原 Docker Compose 项目向后兼容区（{{ legacyList.length }} 个）</strong>
        <span>{{ showLegacy ? '▲ 收起' : '▼ 展开' }}</span>
      </div>
      <div v-if="showLegacy" class="grid grid-2 mt">
        <div class="card" v-for="p in legacyList" :key="p.name" style="background:var(--panel2)">
          <div style="display:flex;justify-content:space-between;align-items:center">
            <strong>{{ p.name }}</strong>
            <span class="badge blue">{{ p.kind }}</span>
          </div>
          <div class="muted mt" style="font-size:12px">路径: {{ p.path }} · 磁盘: {{ p.diskUsageMB }} MB</div>
          <div class="mt" style="font-size:12px">
            <div v-for="c in p.containers" :key="c" style="padding:2px 0">{{ c }}</div>
          </div>
          <div class="mt">
            <button class="btn" @click="deployLegacy(p)" :disabled="deployingLegacy===p.name">
              {{ deployingLegacy===p.name ? '部署中...' : '重新部署' }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="modal.show" class="modal-mask">
      <div class="modal-card">
        <div style="display:flex;justify-content:space-between;align-items:center">
          <strong style="font-size:17px">{{ modal.isEdit ? '✏️ 编辑业务服务' : '➕ 纳管新业务服务' }}</strong>
          <button class="btn" @click="modal.show=false">✕</button>
        </div>

        <div class="mt form-grid">
          <div>
            <label class="form-label">服务标识名 (英文，用于生成 ops-{name}.service)*</label>
            <input v-model="modal.form.name" :disabled="modal.isEdit" placeholder="例如 api-gateway, auth-service" />
          </div>
          <div>
            <label class="form-label">显示名称*</label>
            <input v-model="modal.form.displayName" placeholder="例如 核心网关服务" />
          </div>
          <div>
            <label class="form-label">业务分组</label>
            <input v-model="modal.form.groupName" placeholder="例如 核心服务, 业务微服务, 通用组件" />
          </div>
          <div>
            <label class="form-label">启动顺序优先级</label>
            <select v-model.number="modal.form.level">
              <option :value="1">Level 1: 基础设施 (数据库/Nacos/Redis等，断电优先拉起)</option>
              <option :value="2">Level 2: 核心网关 / 基础组件</option>
              <option :value="3">Level 3: 业务微服务 / 上层应用</option>
            </select>
          </div>
          <div style="grid-column:1/-1">
            <label class="form-label">工作目录 (WorkingDirectory)*</label>
            <input v-model="modal.form.workDir" placeholder="例如 /workspace/app" />
          </div>
          <div style="grid-column:1/-1">
            <label class="form-label">启动命令或脚本路径 (ExecStart)*</label>
            <input v-model="modal.form.execStart" placeholder="例如 ./start.sh 或 /workspace/app/start.sh 或 java -jar app.jar" />
          </div>
          <div style="grid-column:1/-1">
            <label class="form-label">停止命令或脚本路径 (ExecStop，选填)</label>
            <input v-model="modal.form.execStop" placeholder="留空则由 Systemd 自动发送优雅终止信号" />
          </div>
          <div>
            <label class="form-label">健康检测端口 (Port)</label>
            <input type="number" v-model.number="modal.form.port" placeholder="例如 8080" />
          </div>
          <div>
            <label class="form-label">进程托管模式 (ServiceType)</label>
            <select v-model="modal.form.serviceType">
              <option value="">自动检测 (包含 .sh 则为 forking 脚本模式)</option>
              <option value="forking">forking (脚本带 nohup/后台 & 运行)</option>
              <option value="simple">simple (前台直接运行如 java/node)</option>
            </select>
          </div>
          <div style="grid-column:1/-1">
            <label class="form-label">启动依赖 (After/Wants 服务)</label>
            <input v-model="modal.form.afterDeps" placeholder="例如 postgresql.service nacos.service" />
          </div>
          <div style="grid-column:1/-1">
            <label class="form-label">关联日志路径 (用于面板直达查看与 Logrotate 轮转)</label>
            <input v-model="modal.form.logPath" placeholder="例如 /workspace/app/logs/app.log" />
          </div>
          <div style="display:flex;align-items:center;gap:16px;grid-column:1/-1;margin-top:6px">
            <label style="display:flex;align-items:center;gap:6px">
              <input type="checkbox" v-model="modal.form.autoStart" />
              <span>开机自动拉起 (WantedBy=multi-user.target)</span>
            </label>
            <label style="display:flex;align-items:center;gap:6px">
              <span>崩溃重启策略:</span>
              <select v-model="modal.form.restartPolicy" style="width:130px;padding:3px 6px">
                <option value="always">always (崩溃秒级拉起)</option>
                <option value="on-failure">on-failure (异常退出拉起)</option>
                <option value="no">no (不自动拉起)</option>
              </select>
            </label>
          </div>
        </div>

        <div class="mt" style="display:flex;justify-content:flex-end;gap:8px">
          <button class="btn" @click="modal.show=false">取消</button>
          <button class="btn primary" @click="saveModal" :disabled="saving">
            {{ saving ? '保存中...' : '保存并同步生成 Systemd 单元' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, onVisible, getToken } from '../api.js'
import { confirmDialog, toast } from '../ui.js'

const router = useRouter()
const services = ref([])
const legacyList = ref([])
const selfCheck = ref(null)
const err = ref('')
const busy = ref('')
const batchLoading = ref(false)
const showLegacy = ref(false)
const deployingLegacy = ref('')
const curGroup = ref('all')
const saving = ref(false)

const modal = ref({
  show: false,
  isEdit: false,
  editId: 0,
  form: {
    name: '',
    displayName: '',
    groupName: '核心服务',
    level: 3,
    workDir: '',
    execStart: '',
    execStop: '',
    afterDeps: '',
    port: 0,
    logPath: '',
    restartPolicy: 'always',
    serviceType: '',
    autoStart: true
  }
})

let timer
let offVisible

const groupList = computed(() => {
  const set = new Set(['all'])
  services.value.forEach(s => {
    if (s.groupName) set.add(s.groupName)
  })
  return Array.from(set)
})

const filteredServices = computed(() => {
  if (curGroup.value === 'all') return services.value
  return services.value.filter(s => s.groupName === curGroup.value)
})

function countByGroup(g) {
  if (g === 'all') return services.value.length
  return services.value.filter(s => s.groupName === g).length
}

async function loadData() {
  try {
    const [svcs, check, leg] = await Promise.all([
      api('/app-services'),
      api('/app-services/self-check'),
      api('/projects').catch(() => [])
    ])
    services.value = svcs
    selfCheck.value = check
    legacyList.value = leg
    err.value = ''
  } catch (e) {
    err.value = '加载服务列表失败: ' + e.message
  }
}

async function runSelfCheck() {
  try {
    selfCheck.value = await api('/app-services/self-check')
    toast('自检完成 ♡', 'success')
  } catch (e) {
    toast('自检失败: ' + e.message, 'error')
  }
}

async function act(s, action) {
  if (action === 'stop') {
    const ok = await confirmDialog({
      title: '停止服务',
      message: `确定要停止业务服务「${s.displayName}」吗？相关功能将暂时不可用！`,
      danger: true,
      okText: '停止'
    })
    if (!ok) return
  }

  busy.value = s.name
  try {
    await api(`/app-services/${encodeURIComponent(s.name)}/${action}`, { method: 'POST' })
    toast('操作成功 ♡', 'success')
    setTimeout(loadData, 1000)
  } catch (e) {
    toast('操作失败: ' + e.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function batchAction(action) {
  const title = action === 'start' ? '一键依序启动' : '一键依序重启'
  const ok = await confirmDialog({
    title,
    message: `确定要按照依赖优先级（Level 1 基础 -> Level 2 核心 -> Level 3 应用）依序${action==='start'?'启动':'重启'}所有纳管服务吗？`,
    okText: '立即执行'
  })
  if (!ok) return

  batchLoading.value = true
  try {
    await api(`/app-services-batch/${action}`, { method: 'POST' })
    toast('依序编排已完成 ♡', 'success')
    setTimeout(loadData, 1500)
  } catch (e) {
    toast('批量操作失败: ' + e.message, 'error')
  } finally {
    batchLoading.value = false
  }
}

async function syncSystemd() {
  const ok = await confirmDialog({
    title: '同步 Systemd 单元',
    message: '将为所有纳管服务重新生成 /etc/systemd/system/ops-*.service 并重新加载 systemd daemon，是否继续？',
    okText: '立即同步'
  })
  if (!ok) return

  try {
    const res = await api('/app-services-sync-systemd', { method: 'POST' })
    toast(`已成功同步 ${res.synced ? res.synced.length : 0} 个 Systemd 单元 ♡`, 'success')
    setTimeout(loadData, 1000)
  } catch (e) {
    toast('同步失败: ' + e.message, 'error')
  }
}

function exportBundle() {
  window.open('/api/app-services/export-bundle?token=' + encodeURIComponent(getToken() || ''), '_blank')
}

function viewLog(path) {
  router.push({ path: '/logs', query: { path } })
}

function openModal(s) {
  if (s) {
    modal.value = {
      show: true,
      isEdit: true,
      editId: s.id,
      form: {
        name: s.name || '',
        displayName: s.displayName || '',
        groupName: s.groupName || '核心服务',
        level: Number(s.level) || 3,
        workDir: s.workDir || '',
        execStart: s.execStart || '',
        execStop: s.execStop || '',
        afterDeps: s.afterDeps || '',
        port: Number(s.port) || 0,
        logPath: s.logPath || '',
        restartPolicy: s.restartPolicy || 'always',
        serviceType: s.serviceType || '',
        autoStart: s.autoStart !== false
      }
    }
  } else {
    modal.value = {
      show: true,
      isEdit: false,
      editId: 0,
      form: {
        name: '',
        displayName: '',
        groupName: curGroup.value !== 'all' ? curGroup.value : '核心服务',
        level: 3,
        workDir: '/workspace',
        execStart: '',
        execStop: '',
        afterDeps: 'network.target',
        port: 0,
        logPath: '',
        restartPolicy: 'always',
        serviceType: '',
        autoStart: true
      }
    }
  }
}

async function saveModal() {
  const f = modal.value.form
  if (!f.name || !f.displayName || !f.execStart) {
    toast('请完整填写标识名、显示名称和启动命令', 'error')
    return
  }

  const payload = {
    name: String(f.name).trim(),
    displayName: String(f.displayName).trim(),
    groupName: f.groupName ? String(f.groupName).trim() : '核心服务',
    level: Number(f.level) || 3,
    workDir: f.workDir ? String(f.workDir).trim() : '/workspace',
    execStart: String(f.execStart).trim(),
    execStop: f.execStop ? String(f.execStop).trim() : '',
    afterDeps: f.afterDeps ? String(f.afterDeps).trim() : '',
    port: Number(f.port) || 0,
    logPath: f.logPath ? String(f.logPath).trim() : '',
    restartPolicy: f.restartPolicy || 'always',
    serviceType: f.serviceType || '',
    autoStart: Boolean(f.autoStart)
  }

  saving.value = true
  try {
    if (modal.value.isEdit) {
      await api(`/app-services/${modal.value.editId}`, {
        method: 'PUT',
        body: JSON.stringify(payload)
      })
      toast('修改成功 ♡', 'success')
    } else {
      await api('/app-services', {
        method: 'POST',
        body: JSON.stringify(payload)
      })
      toast('新增服务并完成 Systemd 注册 ♡', 'success')
    }
    modal.value.show = false
    loadData()
  } catch (e) {
    toast('保存失败: ' + e.message, 'error')
  } finally {
    saving.value = false
  }
}

async function deleteService(s) {
  const ok = await confirmDialog({
    title: '删除纳管服务',
    message: `确定要删除「${s.displayName}」吗？系统将自动停止并注销 ops-${s.name}.service 单元！`,
    danger: true,
    okText: '删除'
  })
  if (!ok) return

  try {
    await api(`/app-services/${s.id}`, { method: 'DELETE' })
    toast('已删除服务 ♡', 'success')
    loadData()
  } catch (e) {
    toast('删除失败: ' + e.message, 'error')
  }
}

async function deployLegacy(p) {
  const ok = await confirmDialog({
    title: '重新部署',
    message: `确定要重新部署「${p.name}」吗？`,
    okText: '部署'
  })
  if (!ok) return
  deployingLegacy.value = p.name
  try {
    await api(`/projects/${encodeURIComponent(p.name)}/deploy`, { method: 'POST' })
    toast('部署完成 ♡', 'success')
    loadData()
  } catch (e) {
    toast('部署失败: ' + e.message, 'error')
  } finally {
    deployingLegacy.value = ''
  }
}

onMounted(() => {
  loadData()
  timer = setInterval(() => {
    if (!document.hidden) loadData()
  }, 10000)
  offVisible = onVisible(loadData)
})

onUnmounted(() => {
  clearInterval(timer)
  offVisible()
})
</script>

<style scoped>
.modal-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(45, 30, 55, 0.45);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 16px;
}
.modal-card {
  background: var(--panel-solid);
  width: 680px;
  max-width: 95vw;
  max-height: 90vh;
  overflow-y: auto;
  border-radius: var(--radius);
  padding: 24px;
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.2);
}
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.form-label {
  display: block;
  font-size: 12px;
  color: var(--muted);
  margin-bottom: 4px;
}
.form-grid input, .form-grid select {
  width: 100%;
  padding: 8px 10px;
  font-size: 13px;
}
</style>
