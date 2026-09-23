<template>
  <div>
    <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:12px">
      <h2 class="page-title" style="margin-bottom:0">🛡️ 存储防爆守卫 ♪</h2>
      <div style="display:flex;gap:8px">
        <button class="btn" @click="loadOverview">🔄 刷新数据</button>
        <button class="btn primary" @click="deployLogrotate" :disabled="deployingLr">
          {{ deployingLr ? '配置中...' : '📝 一键部署 Logrotate 轮转' }}
        </button>
        <button class="btn danger" @click="cleanArchives" :disabled="cleaning">
          {{ cleaning ? '清理中...' : '🧹 一键清理过期归档日志' }}
        </button>
      </div>
    </div>

    <div v-if="err" class="error-msg mt">(｡•́︿•̀｡) {{ err }}</div>

    <div class="grid grid-2 mt">
      <div class="card" v-for="d in overview.disks" :key="d.mountpoint">
        <div style="display:flex;justify-content:space-between;align-items:center">
          <strong>挂载点: {{ d.mountpoint }}</strong>
          <span class="badge" :class="(d.usedPct || 0) >= overview.criticalThreshold ? 'red' : (d.usedPct || 0) >= overview.warningThreshold ? 'yellow' : 'green'">
            {{ (d.usedPct || 0).toFixed(1) }}%
          </span>
        </div>
        <div class="muted mt" style="font-size:13px">设备: {{ d.device }} ({{ d.fsType }})</div>
        <div class="storage-bar mt">
          <div class="storage-bar-fill" :style="{ width: Math.min(d.usedPct, 100) + '%', background: getBarColor(d.usedPct) }"></div>
        </div>
        <div style="display:flex;justify-content:space-between;font-size:12px;margin-top:6px" class="muted">
          <span>已用: {{ fmtBytes(d.used) }}</span>
          <span>可用: {{ fmtBytes(d.free) }}</span>
          <span>总量: {{ fmtBytes(d.total) }}</span>
        </div>
        <div v-if="d.usedPct >= overview.criticalThreshold" class="danger-box mt">
          🚨 警告：磁盘使用率已超过临界水位 {{ overview.criticalThreshold }}%！未扩容前请尽快截断超大日志或清理垃圾！
        </div>
      </div>
    </div>

    <div class="card mt">
      <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px">
        <div>
          <strong style="font-size:16px">🎯 超大文件雷达 (TOP 20)</strong>
          <div class="muted" style="font-size:12px;margin-top:2px">自动检测超过 10MB 的超大文件与日志文件，支持无感知截断清空（不破坏正在运行的进程句柄）</div>
        </div>
        <div style="display:flex;gap:8px;align-items:center">
          <input v-model="scanPath" placeholder="自定义扫描路径（默认 /workspace, /var/log）" style="width:240px;padding:5px 8px;font-size:12px" />
          <button class="btn" @click="scanFiles" :disabled="scanning">{{ scanning ? '扫描中...' : '🔎 重新扫描' }}</button>
        </div>
      </div>

      <table class="mt" style="width:100%">
        <thead>
          <tr>
            <th style="width:50px">#</th>
            <th>文件路径</th>
            <th style="width:110px">大小</th>
            <th style="width:170px">修改时间</th>
            <th style="width:120px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(f, idx) in files" :key="f.path">
            <td>{{ idx + 1 }}</td>
            <td style="word-break:break-all">
              <strong>{{ f.name }}</strong>
              <div class="muted" style="font-size:11px">{{ f.path }}</div>
            </td>
            <td>
              <span class="badge" :class="f.size > 500*1024*1024 ? 'red' : f.size > 100*1024*1024 ? 'yellow' : 'blue'">
                {{ f.sizeFormatted }}
              </span>
            </td>
            <td class="muted" style="font-size:12px">{{ f.modTime }}</td>
            <td>
              <button class="btn danger" style="padding:3px 8px;font-size:12px" @click="truncateFile(f)" :disabled="truncating===f.path">
                {{ truncating===f.path ? '处理中...' : '⚡ 安全清空' }}
              </button>
            </td>
          </tr>
          <tr v-if="files.length === 0">
            <td colspan="5" class="muted" style="text-align:center;padding:18px">
              (๑>ᴗ<๑) 干净整洁！未发现大于 10MB 的超大日志或数据文件
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card mt">
      <strong style="font-size:15px">⚙️ 存储自保与防爆策略设置</strong>
      <div style="display:flex;gap:16px;align-items:center;flex-wrap:wrap;margin-top:12px">
        <label style="display:flex;align-items:center;gap:6px;font-size:13px">
          <span>预警水位:</span>
          <input type="number" v-model.number="overview.warningThreshold" style="width:70px;padding:4px 6px" min="50" max="95" /> %
        </label>
        <label style="display:flex;align-items:center;gap:6px;font-size:13px">
          <span>严重临界水位:</span>
          <input type="number" v-model.number="overview.criticalThreshold" style="width:70px;padding:4px 6px" min="60" max="99" /> %
        </label>
        <label style="display:flex;align-items:center;gap:6px;font-size:13px">
          <input type="checkbox" v-model="overview.autoCleanEnabled" />
          <span>临界水位时自动触发历史归档清理</span>
        </label>
        <button class="btn primary" @click="saveSettings">保存策略</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api, onVisible } from '../api.js'
import { confirmDialog, toast } from '../ui.js'

const overview = ref({
  disks: [],
  warningThreshold: 85,
  criticalThreshold: 90,
  autoCleanEnabled: true
})
const files = ref([])
const err = ref('')
const scanning = ref(false)
const scanPath = ref('')
const truncating = ref('')
const cleaning = ref(false)
const deployingLr = ref(false)

let timer
let offVisible

function fmtBytes(bytes) {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1)
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

function getBarColor(pct) {
  if (pct >= overview.value.criticalThreshold) return 'linear-gradient(90deg, #fb7a9e, #f0569e)'
  if (pct >= overview.value.warningThreshold) return 'linear-gradient(90deg, #f6c05c, #fb7a9e)'
  return 'linear-gradient(90deg, #4fd1a5, #7cc7f7)'
}

async function loadOverview() {
  try {
    const res = await api('/storage/overview')
    overview.value = res
    if (res.topFiles) {
      files.value = res.topFiles
    }
    err.value = ''
  } catch (e) {
    err.value = '加载存储数据失败: ' + e.message
  }
}

async function scanFiles() {
  scanning.value = true
  try {
    const p = scanPath.value ? `?path=${encodeURIComponent(scanPath.value)}` : ''
    files.value = await api(`/storage/large-files${p}`)
    toast('扫描完成 ♡', 'success')
  } catch (e) {
    toast('扫描失败: ' + e.message, 'error')
  } finally {
    scanning.value = false
  }
}

async function truncateFile(f) {
  const ok = await confirmDialog({
    title: '⚡ 安全截断日志文件',
    message: `确定要将文件「${f.name}」(${f.sizeFormatted}) 内容截断清空为 0 字节吗？此操作会立即释放磁盘空间，且不会影响正在写入的进程！`,
    danger: true,
    okText: '确认清空'
  })
  if (!ok) return

  truncating.value = f.path
  try {
    await api('/storage/truncate-file', {
      method: 'POST',
      body: JSON.stringify({ path: f.path })
    })
    toast('文件已成功清空，磁盘空间已释放 ♡', 'success')
    await scanFiles()
    await loadOverview()
  } catch (e) {
    toast('清空失败: ' + e.message, 'error')
  } finally {
    truncating.value = ''
  }
}

async function cleanArchives() {
  const ok = await confirmDialog({
    title: '清理历史归档日志',
    message: '确定要一键扫描并清理 7 天前已压缩的 *.gz / *.zip / *.bak 历史归档日志及 /tmp 缓存文件吗？',
    okText: '开始清理'
  })
  if (!ok) return

  cleaning.value = true
  try {
    const res = await api('/storage/clean-archives', {
      method: 'POST',
      body: JSON.stringify({ days: 7 })
    })
    toast(`清理完成！共删除 ${res.cleanedCount} 个历史文件，释放 ${res.freedText} 空间 ♡`, 'success')
    await scanFiles()
    await loadOverview()
  } catch (e) {
    toast('清理失败: ' + e.message, 'error')
  } finally {
    cleaning.value = false
  }
}

async function deployLogrotate() {
  const ok = await confirmDialog({
    title: '部署 Logrotate 定额轮转',
    message: '系统将根据纳管的应用服务日志路径，自动在 /etc/logrotate.d/opsweb-services 部署每日 50MB 自动轮转规则（保留 7 份并使用 copytruncate 防止磁盘写爆）。是否立即生效？',
    okText: '立即部署'
  })
  if (!ok) return

  deployingLr.value = true
  try {
    const res = await api('/storage/deploy-logrotate', { method: 'POST' })
    toast(res || 'Logrotate 规则已成功生效 ♡', 'success')
  } catch (e) {
    toast('部署失败: ' + e.message, 'error')
  } finally {
    deployingLr.value = false
  }
}

async function saveSettings() {
  try {
    await api('/storage/settings', {
      method: 'POST',
      body: JSON.stringify({
        warningThreshold: Number(overview.value.warningThreshold) || 85,
        criticalThreshold: Number(overview.value.criticalThreshold) || 90,
        autoCleanEnabled: Boolean(overview.value.autoCleanEnabled)
      })
    })
    toast('防爆策略已更新 ♡', 'success')
  } catch (e) {
    toast('保存失败: ' + e.message, 'error')
  }
}

onMounted(() => {
  loadOverview()
  timer = setInterval(() => {
    if (!document.hidden) loadOverview()
  }, 30000)
  offVisible = onVisible(loadOverview)
})

onUnmounted(() => {
  clearInterval(timer)
  offVisible()
})
</script>

<style scoped>
.storage-bar {
  height: 12px;
  background: rgba(220, 205, 230, 0.4);
  border-radius: 6px;
  overflow: hidden;
}
.storage-bar-fill {
  height: 100%;
  border-radius: 6px;
  transition: width 0.4s ease;
}
.danger-box {
  background: rgba(251, 122, 158, 0.12);
  border: 1px solid var(--red);
  color: var(--accent-deep);
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 500;
}
</style>
