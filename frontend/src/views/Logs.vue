<template>
  <div>
    <h2 class="page-title">日志查看 ♪</h2>
    <div class="card">
      <div class="tabs">
        <button :class="['tab', mode === 'file' ? 'active' : '']" @click="mode = 'file'">📄 文件日志</button>
        <button :class="['tab', mode === 'journal' ? 'active' : '']" @click="switchJournal">🖥️ 系统日志 (journalctl)</button>
      </div>

      <!-- 文件日志: 目录浏览 + tail -->
      <template v-if="mode === 'file'">
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <span class="crumb muted">允许根目录:</span>
          <button v-for="r in roots" :key="r" class="btn crumb-btn" @click="browse(r)">{{ r }}</button>
        </div>
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap" class="mt">
          <input v-model="path" placeholder="目录或文件路径" style="flex:1;min-width:220px" @keyup.enter="pathIsFile ? loadFile() : browse(path)" />
          <input v-model.number="tail" style="width:70px" title="行数" />
          <button class="btn primary" @click="pathIsFile ? loadFile() : browse(path)">{{ pathIsFile ? '查看文件' : '浏览目录' }}</button>
        </div>

        <!-- 目录列表 -->
        <div v-if="listing" class="mt file-list">
          <div class="muted" style="margin-bottom:6px">{{ listing.path }}</div>
          <div v-for="d in listing.dirs" :key="d.name" class="file-row" @click="browse(listing.path === '/' ? '' : listing.path + '/' + d.name)">
            <span>📁 {{ d.name }}</span>
          </div>
          <div v-for="f in listing.files" :key="f.name" class="file-row" :class="{ selected: listing.path + '/' + f.name === path }" @click="pickFile(listing.path + '/' + f.name)">
            <span>{{ f.name.endsWith('.gz') ? '🗜️' : '📃' }} {{ f.name }}</span>
            <span class="muted" style="font-size:12px">{{ fmtSize(f.size) }}</span>
          </div>
          <div v-if="!listing.dirs.length && !listing.files.length" class="muted">(๑´ㅂ`๑) 这个目录空空的</div>
        </div>

        <div v-if="err" class="error-msg mt">{{ err }}</div>
        <div v-else-if="result">
          <div class="muted mt">{{ result.path }}（显示 {{ result.lines.length }} 行，文件大小 {{ fmtSize(result.size) }}，更新于 {{ result.mtime }}）</div>
          <pre class="log mt">{{ result.lines.join('\n') }}</pre>
        </div>
      </template>

      <!-- 系统日志: journalctl -->
      <template v-else>
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <select v-model="journalUnit" style="min-width:180px">
            <option value="">全部系统日志</option>
            <option v-for="u in units" :key="u" :value="u">{{ u }}</option>
            <option value="__custom">指定 unit…</option>
          </select>
          <input v-if="journalUnit === '__custom'" v-model="customUnit" placeholder="如 nginx.service" style="width:180px" />
          <select v-model="journalSince">
            <option value="10min ago">最近 10 分钟</option>
            <option value="1h ago">最近 1 小时</option>
            <option value="6h ago">最近 6 小时</option>
            <option value="today">今天</option>
            <option value="24h ago">最近 24 小时</option>
          </select>
          <input v-model="journalGrep" placeholder="关键字过滤（可选）" style="flex:1;min-width:140px" @keyup.enter="loadJournal" />
          <input v-model.number="tail" style="width:70px" title="行数" />
          <button class="btn primary" @click="loadJournal">查询</button>
        </div>
        <div v-if="err" class="error-msg mt">{{ err }}</div>
        <div v-else-if="journalLines !== null">
          <div class="muted mt">共 {{ journalLines.length }} 行（时间正序）</div>
          <pre class="log mt">{{ journalLines.join('\n') }}</pre>
        </div>
        <div v-else class="muted mt loading-tip">(๑>ᴗ<๑) 选择范围后点「查询」～</div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, fmtBytes } from '../api.js'

const roots = ['/var/log', '/workspace/baq-test/logs', '/workspace/szx-test/logs', '/workspace/baq-test/.deploy']

const mode = ref('file')
const path = ref('/var/log')
const tail = ref(200)
const listing = ref(null)
const result = ref(null)
const err = ref('')

const journalUnit = ref('')
const customUnit = ref('')
const journalSince = ref('1h ago')
const journalGrep = ref('')
const journalLines = ref(null)
const units = ref([])

const pathIsFile = computed(() => /\.[a-zA-Z0-9]+$/.test(path.value) && !path.value.endsWith('/'))

function fmtSize(n) {
  return fmtBytes(n)
}

async function browse(p) {
  err.value = ''
  result.value = null
  try {
    listing.value = await api(`/logs/list?path=${encodeURIComponent(p || '/')}`)
    path.value = listing.value.path
  } catch (e) { err.value = e.message; listing.value = null }
}

function pickFile(p) {
  path.value = p
  loadFile()
}

async function loadFile() {
  err.value = ''
  listing.value = null
  try {
    result.value = await api(`/logs/file?path=${encodeURIComponent(path.value)}&tail=${tail.value}`)
  } catch (e) { err.value = e.message; result.value = null }
}

function switchJournal() {
  mode.value = 'journal'
  if (!units.value.length) loadUnits()
}

async function loadUnits() {
  try {
    const d = await api('/services')
    units.value = (d.services || []).map(s => s.unit).filter(Boolean).slice(0, 50)
  } catch { /* ignore */ }
}

async function loadJournal() {
  err.value = ''
  result.value = null
  try {
    const unit = journalUnit.value === '__custom' ? customUnit.value : journalUnit.value
    const params = new URLSearchParams({ since: journalSince.value, tail: tail.value })
    if (unit) params.set('unit', unit)
    if (journalGrep.value) params.set('grep', journalGrep.value)
    const d = await api(`/logs/journal?${params}`)
    journalLines.value = d.lines || []
  } catch (e) { err.value = e.message; journalLines.value = null }
}

onMounted(() => { browse('/var/log') })
</script>

<style scoped>
.tabs { display: flex; gap: 8px; margin-bottom: 12px; }
.tab { border: none; background: transparent; padding: 8px 16px; border-radius: 999px; cursor: pointer; font-size: 14px; color: var(--muted); }
.tab.active { background: linear-gradient(135deg, var(--accent), var(--accent2)); color: #fff; }
.file-list { border: 1.5px solid var(--border); border-radius: 12px; max-height: 260px; overflow-y: auto; padding: 6px; }
.file-row { display: flex; justify-content: space-between; align-items: center; padding: 6px 10px; border-radius: 8px; cursor: pointer; font-size: 14px; }
.file-row:hover { background: rgba(255, 126, 182, 0.12); }
.file-row.selected { background: rgba(183, 148, 246, 0.2); }
.crumb-btn { font-size: 12px; padding: 4px 10px; }
</style>
