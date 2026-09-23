<template>
  <div class="servers-view">
    <!-- 顶部状态栏与工具栏 -->
    <header class="view-header">
      <div class="title-group">
        <h2>🖥️ 主机资产与运维工作台</h2>
        <span class="sub-title">像 FinalShell 一样自由纳管多台服务器，集中监控与一键自愈</span>
      </div>

      <div class="header-actions">
        <button class="btn primary" @click="openAddModal">
          <span>＋ 新建服务器</span>
        </button>
        <button class="btn" :disabled="pingingAll" @click="pingAll">
          <span :class="{ 'icon-spin': pingingAll }">⚡</span>
          {{ pingingAll ? '探活中...' : '全网测速' }}
        </button>
        <button class="btn" title="导出资产配置 JSON" @click="handleExport">
          <span>📤 导出</span>
        </button>
        <label class="btn file-btn" title="导入资产配置 JSON">
          <span>📥 导入</span>
          <input type="file" accept=".json" style="display:none" @change="handleImportFile" />
        </label>
      </div>
    </header>

    <!-- 统计指标条与过滤栏 -->
    <div class="metrics-filter-bar panel">
      <div class="stat-badges">
        <div class="stat-pill">
          <span class="stat-label">总纳管主机</span>
          <span class="stat-val">{{ servers.length }}</span>
        </div>
        <div class="stat-pill online">
          <span class="stat-label">在线就绪</span>
          <span class="stat-val">{{ onlineCount }}</span>
        </div>
        <div class="stat-pill offline">
          <span class="stat-label">离线/超时</span>
          <span class="stat-val">{{ offlineCount }}</span>
        </div>
        <div class="stat-pill active">
          <span class="stat-label">当前运维连接</span>
          <span class="stat-val highlight">{{ activeServer ? activeServer.name : '未选择' }}</span>
        </div>
      </div>

      <div class="search-group">
        <div class="group-tabs">
          <button
            v-for="g in groupList"
            :key="g"
            class="tab-btn"
            :class="{ active: currentGroup === g }"
            @click="currentGroup = g"
          >
            {{ g }}
          </button>
        </div>
        <input
          v-model="searchQuery"
          type="text"
          class="search-input"
          placeholder="搜索服务器名称、IP、分组或标签..."
        />
      </div>
    </div>

    <!-- 服务器卡片矩阵 -->
    <div class="server-grid">
      <div
        v-for="s in filteredServers"
        :key="s.id"
        class="server-card panel"
        :class="{
          'is-active': activeServerId === s.id,
          'is-online': s.status === 'online',
          'is-offline': s.status === 'offline',
        }"
      >
        <div class="card-head">
          <div class="head-left">
            <span class="status-indicator" :class="s.status" :title="statusTitle(s)"></span>
            <div class="server-name" :title="s.name">{{ s.name }}</div>
          </div>
          <span class="group-tag">{{ s.group || '未分组' }}</span>
        </div>

        <div class="card-url" :title="s.url">
          <span class="url-label">节点地址:</span>
          <code>{{ s.url }}</code>
        </div>

        <div class="card-desc">
          {{ s.description || '暂无详细描述信息' }}
        </div>

        <div class="card-tags">
          <span v-for="t in s.tags" :key="t" class="tag-pill">{{ t }}</span>
          <span v-if="s.latency != null" class="latency-pill" :class="latencyLevel(s.latency)">
            ⚡ {{ s.latency }}ms
          </span>
        </div>

        <div class="card-footer">
          <div class="action-left">
            <button
              v-if="activeServerId === s.id"
              class="btn active-indicator"
              disabled
            >
              🟢 当前连接中
            </button>
            <button
              v-else
              class="btn primary connect-btn"
              @click="connectServer(s)"
            >
              ⚡ 进入运维
            </button>
          </div>

          <div class="action-right">
            <button class="btn icon-btn" title="测速探活" @click="pingSingle(s)">
              🔄
            </button>
            <button class="btn icon-btn" title="编辑服务器" @click="openEditModal(s)">
              ✎
            </button>
            <button class="btn icon-btn danger" title="删除服务器" @click="confirmDelete(s)">
              🗑
            </button>
          </div>
        </div>
      </div>

      <!-- 快捷添加空卡片 -->
      <div class="server-card add-card" @click="openAddModal">
        <div class="add-icon">＋</div>
        <div class="add-txt">添加新的服务器节点</div>
        <div class="add-sub">支持添加生产、测试或云主机</div>
      </div>
    </div>

    <!-- 新建/编辑服务器模态窗 -->
    <div v-if="showModal" class="modal-backdrop" @click.self="showModal = false">
      <div class="modal-card panel">
        <div class="modal-header">
          <h3>{{ editingId ? '编辑服务器配置' : '新建纳管服务器' }}</h3>
          <button class="close-btn" @click="showModal = false">×</button>
        </div>

        <div class="modal-body">
          <div class="form-item">
            <label>服务器名称 <span class="req">*</span></label>
            <input
              v-model="form.name"
              type="text"
              placeholder="例如：杭州核心集群 01 节点"
            />
          </div>

          <div class="form-item">
            <label>控制台 / API 地址 <span class="req">*</span></label>
            <div class="url-input-wrap">
              <input
                v-model="form.url"
                type="text"
                placeholder="例如：http://192.168.1.100:9800"
              />
              <button class="btn test-btn" :disabled="testing" @click="testCurrentForm">
                {{ testing ? '测试中...' : '测试连通' }}
              </button>
            </div>
            <div v-if="testResult" class="test-feedback" :class="testResult.success ? 'ok' : 'fail'">
              {{ testResult.msg }}
            </div>
          </div>

          <div class="form-row">
            <div class="form-item">
              <label>环境分组</label>
              <input
                v-model="form.group"
                type="text"
                placeholder="例如：生产环境 / 测试开发"
              />
            </div>
            <div class="form-item">
              <label>标签 (逗号分隔)</label>
              <input
                v-model="form.tagsInput"
                type="text"
                placeholder="例如：核心, Docker, PG"
              />
            </div>
          </div>

          <div class="form-item">
            <label>访问令牌 / Token (可选)</label>
            <input
              v-model="form.token"
              type="password"
              placeholder="如该服务器开启了专属访问令牌，在此填写"
            />
          </div>

          <div class="form-item">
            <label>备注说明</label>
            <textarea
              v-model="form.description"
              rows="2"
              placeholder="例如：承载核心自愈与业务容器"
            ></textarea>
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn" @click="showModal = false">取消</button>
          <button class="btn primary" @click="saveServerForm">保存并应用</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  getServers,
  saveServers,
  getActiveServer,
  getActiveServerId,
  setActiveServerId,
  pingServer,
  exportServersJson,
  importServersJson,
} from '../utils/servers.js'

const router = useRouter()
const servers = ref([])
const activeServerId = ref(getActiveServerId())
const searchQuery = ref('')
const currentGroup = ref('全部')
const pingingAll = ref(false)

const showModal = ref(false)
const editingId = ref(null)
const testing = ref(false)
const testResult = ref(null)

const form = ref({
  name: '',
  url: '',
  group: '生产环境',
  tagsInput: '',
  token: '',
  description: '',
})

const activeServer = computed(() => {
  return servers.value.find(s => s.id === activeServerId.value)
})

const onlineCount = computed(() => servers.value.filter(s => s.status === 'online').length)
const offlineCount = computed(() => servers.value.filter(s => s.status === 'offline').length)

const groupList = computed(() => {
  const set = new Set(['全部'])
  for (const s of servers.value) {
    if (s.group) set.add(s.group)
  }
  return Array.from(set)
})

const filteredServers = computed(() => {
  return servers.value.filter(s => {
    if (currentGroup.value !== '全部' && s.group !== currentGroup.value) return false
    if (!searchQuery.value.trim()) return true
    const q = searchQuery.value.toLowerCase()
    return (
      (s.name && s.name.toLowerCase().includes(q)) ||
      (s.url && s.url.toLowerCase().includes(q)) ||
      (s.group && s.group.toLowerCase().includes(q)) ||
      (s.description && s.description.toLowerCase().includes(q)) ||
      (s.tags && s.tags.some(t => t.toLowerCase().includes(q)))
    )
  })
})

onMounted(() => {
  loadList()
  // 首次打开自动探活
  pingAll()
})

function loadList() {
  servers.value = getServers()
  activeServerId.value = getActiveServerId()
}

function statusTitle(s) {
  if (s.status === 'online') return `在线就绪 (响应延迟 ${s.latency}ms)`
  if (s.status === 'offline') return '离线或无法连通'
  if (s.status === 'error') return '接口异常或服务未就绪'
  return '未检测'
}

function latencyLevel(ms) {
  if (ms < 50) return 'fast'
  if (ms < 150) return 'medium'
  return 'slow'
}

async function pingSingle(s) {
  s.status = 'checking'
  await pingServer(s)
  saveServers(servers.value)
}

async function pingAll() {
  if (pingingAll.value) return
  pingingAll.value = true
  const tasks = servers.value.map(s => {
    s.status = 'checking'
    return pingServer(s)
  })
  await Promise.allSettled(tasks)
  saveServers(servers.value)
  pingingAll.value = false
}

function connectServer(s) {
  setActiveServerId(s.id)
  activeServerId.value = s.id

  // 如果是在独立桌面端环境，向外通知切换 iframe
  const curOrigin = window.location.origin.replace(/\/$/, '')
  const targetOrigin = s.url.replace(/\/$/, '')

  if (curOrigin !== targetOrigin) {
    // 跨主机切换：如果在不同源，保存目标地址并重定向或打开
    localStorage.setItem('opsweb_desktop_server', targetOrigin)
    // 跳转到目标主机的控制台
    window.location.href = `${targetOrigin}/`
  } else {
    // 同一主机直接回仪表盘
    router.push('/')
  }
}

function openAddModal() {
  editingId.value = null
  testResult.value = null
  form.value = {
    name: '',
    url: 'http://',
    group: '生产环境',
    tagsInput: 'Docker, 核心',
    token: '',
    description: '',
  }
  showModal.value = true
}

function openEditModal(s) {
  editingId.value = s.id
  testResult.value = null
  form.value = {
    name: s.name,
    url: s.url,
    group: s.group || '未分组',
    tagsInput: Array.isArray(s.tags) ? s.tags.join(', ') : '',
    token: s.token || '',
    description: s.description || '',
  }
  showModal.value = true
}

async function testCurrentForm() {
  if (!form.value.url) return
  testing.value = true
  testResult.value = null
  const temp = { url: form.value.url }
  await pingServer(temp)
  testing.value = false
  if (temp.status === 'online') {
    testResult.value = { success: true, msg: `连接正常！响应延迟 ${temp.latency}ms` }
  } else {
    testResult.value = { success: false, msg: `连接失败，无法连通目标服务或探活接口未开放` }
  }
}

function saveServerForm() {
  if (!form.value.name.trim() || !form.value.url.trim()) {
    alert('请填写服务器名称和连接地址')
    return
  }

  const tags = form.value.tagsInput
    ? form.value.tagsInput.split(/[,，]/).map(t => t.trim()).filter(Boolean)
    : []

  if (editingId.value) {
    const idx = servers.value.findIndex(s => s.id === editingId.value)
    if (idx !== -1) {
      servers.value[idx] = {
        ...servers.value[idx],
        name: form.value.name.trim(),
        url: form.value.url.trim().replace(/\/+$/, ''),
        group: form.value.group.trim(),
        tags,
        token: form.value.token.trim(),
        description: form.value.description.trim(),
      }
    }
  } else {
    const newId = 'srv_' + Date.now()
    servers.value.push({
      id: newId,
      name: form.value.name.trim(),
      url: form.value.url.trim().replace(/\/+$/, ''),
      group: form.value.group.trim() || '通用节点',
      tags,
      token: form.value.token.trim(),
      description: form.value.description.trim(),
      status: 'unknown',
      latency: null,
      lastChecked: null,
    })
  }

  saveServers(servers.value)
  showModal.value = false
}

function confirmDelete(s) {
  if (confirm(`确定要移除服务器「${s.name}」吗？`)) {
    servers.value = servers.value.filter(item => item.id !== s.id)
    saveServers(servers.value)
    if (activeServerId.value === s.id && servers.value.length > 0) {
      setActiveServerId(servers.value[0].id)
      activeServerId.value = servers.value[0].id
    }
  }
}

function handleExport() {
  exportServersJson()
}

function handleImportFile(e) {
  const file = e.target.files[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = (evt) => {
    try {
      importServersJson(evt.target.result)
      loadList()
      pingAll()
      alert('服务器资产配置导入成功！')
    } catch (err) {
      alert('导入失败: ' + err.message)
    }
  }
  reader.readAsText(file)
}
</script>

<style scoped>
.servers-view {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 1400px;
  margin: 0 auto;
}

.view-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}
.title-group h2 {
  font-size: 20px;
  font-weight: 800;
  margin: 0 0 4px 0;
  color: var(--text);
}
.sub-title {
  font-size: 13px;
  color: var(--text-muted);
}
.header-actions {
  display: flex;
  gap: 8px;
}
.file-btn {
  cursor: pointer;
}

/* 统计与搜索条 */
.metrics-filter-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  padding: 12px 16px;
  gap: 12px;
  border-radius: 12px;
}
.stat-badges {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.stat-pill {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: rgba(0, 0, 0, 0.04);
  border-radius: 8px;
  font-size: 12px;
}
.stat-label {
  color: var(--text-muted);
}
.stat-val {
  font-weight: 700;
  color: var(--text);
}
.stat-pill.online .stat-val { color: #10b981; }
.stat-pill.offline .stat-val { color: #f43f5e; }
.stat-pill.active .highlight { color: #fb7a9e; font-weight: 800; }

.search-group {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.group-tabs {
  display: flex;
  gap: 4px;
}
.tab-btn {
  background: transparent;
  border: 1px solid transparent;
  padding: 4px 10px;
  font-size: 12px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-muted);
  transition: all 0.15s ease;
}
.tab-btn:hover {
  color: var(--text);
  background: rgba(0, 0, 0, 0.04);
}
.tab-btn.active {
  background: var(--pink-soft, rgba(251, 122, 158, 0.15));
  color: #fb7a9e;
  font-weight: 700;
  border-color: rgba(251, 122, 158, 0.3);
}
.search-input {
  padding: 5px 10px;
  font-size: 12px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--panel);
  color: var(--text);
  min-width: 220px;
}

/* 服务器卡片网格 */
.server-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}
.server-card {
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  border: 1px solid var(--border);
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
  position: relative;
}
.server-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.08);
}
.server-card.is-active {
  border-color: #fb7a9e;
  box-shadow: 0 0 0 1px #fb7a9e, 0 8px 24px rgba(251, 122, 158, 0.2);
}

.card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.head-left {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow: hidden;
}
.status-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  background: #94a3b8;
}
.status-indicator.online {
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
}
.status-indicator.offline {
  background: #f43f5e;
  box-shadow: 0 0 8px #f43f5e;
}
.status-indicator.checking {
  background: #38bdf8;
  animation: pulse 1s infinite alternate;
}
@keyframes pulse { 0% { opacity: 0.3; } 100% { opacity: 1; } }

.server-name {
  font-weight: 700;
  font-size: 15px;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.group-tag {
  font-size: 11px;
  padding: 2px 7px;
  border-radius: 4px;
  background: rgba(167, 139, 250, 0.15);
  color: #8b5cf6;
  font-weight: 600;
  white-space: nowrap;
}

.card-url {
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-muted);
}
.card-url code {
  font-family: Consolas, monospace;
  color: var(--text);
  background: rgba(0, 0, 0, 0.04);
  padding: 2px 6px;
  border-radius: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-desc {
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.4;
  height: 34px;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.card-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: center;
  min-height: 22px;
}
.tag-pill {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.05);
  color: var(--text-muted);
}
.latency-pill {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 700;
}
.latency-pill.fast { background: rgba(16, 185, 129, 0.15); color: #10b981; }
.latency-pill.medium { background: rgba(245, 158, 11, 0.15); color: #f59e0b; }
.latency-pill.slow { background: rgba(244, 63, 94, 0.15); color: #f43f5e; }

.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid var(--border);
}
.connect-btn {
  font-size: 12px;
  padding: 4px 12px;
}
.active-indicator {
  font-size: 11px;
  padding: 4px 10px;
  color: #10b981;
  background: rgba(16, 185, 129, 0.12);
  border: 1px solid rgba(16, 185, 129, 0.3);
  font-weight: 700;
}
.action-right {
  display: flex;
  gap: 4px;
}
.icon-btn {
  padding: 4px 8px;
  font-size: 12px;
}
.icon-btn.danger:hover {
  color: #f43f5e;
}

/* 快捷添加卡片 */
.add-card {
  border: 2px dashed var(--border);
  background: transparent;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  min-height: 180px;
  transition: all 0.2s ease;
}
.add-card:hover {
  border-color: #fb7a9e;
  background: rgba(251, 122, 158, 0.05);
}
.add-icon {
  font-size: 28px;
  color: #fb7a9e;
  margin-bottom: 4px;
}
.add-txt {
  font-size: 14px;
  font-weight: 700;
  color: var(--text);
}
.add-sub {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 2px;
}

/* 模态弹窗 */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
}
.modal-card {
  width: 90%;
  max-width: 520px;
  border-radius: 14px;
  padding: 20px;
  border: 1px solid var(--border);
  box-shadow: 0 16px 36px rgba(0, 0, 0, 0.3);
  background: var(--panel-solid);
}
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.modal-header h3 {
  margin: 0;
  font-size: 17px;
  font-weight: 800;
}
.close-btn {
  background: transparent;
  border: none;
  font-size: 20px;
  cursor: pointer;
  color: var(--text-muted);
}
.modal-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.form-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.form-item label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text);
}
.req { color: #f43f5e; }
.form-item input, .form-item textarea {
  padding: 7px 10px;
  font-size: 13px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--bg);
  color: var(--text);
}
.url-input-wrap {
  display: flex;
  gap: 8px;
}
.url-input-wrap input {
  flex: 1;
}
.test-btn {
  font-size: 12px;
  white-space: nowrap;
}
.test-feedback {
  font-size: 11px;
  margin-top: 4px;
  padding: 3px 8px;
  border-radius: 4px;
}
.test-feedback.ok { background: rgba(16, 185, 129, 0.15); color: #10b981; }
.test-feedback.fail { background: rgba(244, 63, 94, 0.15); color: #f43f5e; }

.form-row {
  display: flex;
  gap: 12px;
}
.form-row .form-item {
  flex: 1;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}
.icon-spin {
  display: inline-block;
  animation: spin 1s infinite linear;
}
@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}
</style>
