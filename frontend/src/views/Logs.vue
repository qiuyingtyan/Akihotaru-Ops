<template>
  <div>
    <h2 class="page-title">日志查看 ♪</h2>
    <div class="card">
      <div class="tabs">
        <button :class="['tab', mode === 'file' ? 'active' : '']" @click="mode = 'file'">📄 文件日志</button>
        <button :class="['tab', mode === 'journal' ? 'active' : '']" @click="switchJournal">🖥️ 系统日志 (journalctl)</button>
      </div>

      <!-- 文件日志: 目录浏览 + tail + 实时跟随 -->
      <template v-if="mode === 'file'">
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <span class="crumb muted">允许根目录:</span>
          <button v-for="r in roots" :key="r" class="btn crumb-btn" @click="browse(r)">{{ r }}</button>
        </div>
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap" class="mt">
          <input v-model="path" placeholder="目录或文件路径" style="flex:1;min-width:220px" @keyup.enter="pathIsFile ? loadFile() : browse(path)" />
          <input v-model.number="tail" style="width:70px" title="行数" />
          <button class="btn primary" @click="pathIsFile ? loadFile() : browse(path)">{{ pathIsFile ? '查看文件' : '浏览目录' }}</button>
          <button v-if="pathIsFile && !following" class="btn live" @click="startFollow">🔴 实时跟随</button>
          <button v-if="following" class="btn danger" @click="stopFollow">⏹ 停止跟随</button>
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
        <div v-else-if="followLines !== null">
          <div class="muted mt">
            <span class="live-dot"></span> 实时跟随中: {{ path }}（{{ followLines.length }} 行，新日志自动滚动）
          </div>
          <pre class="log mt" ref="followBox">{{ followLines.join('\n') }}</pre>
        </div>
        <div v-else-if="result">
          <div class="muted mt" style="display:flex;justify-content:space-between;align-items:center">
            <span>{{ result.path }}（显示 {{ result.lines.length }} 行，文件大小 {{ fmtSize(result.size) }}，更新于 {{ result.mtime }}）</span>
            <button class="btn" @click="analyzeLog(result.lines)">🤖 AI 分析报错</button>
          </div>
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
          <button v-if="!jFollowing" class="btn live" @click="startJournalFollow">🔴 实时跟随</button>
          <button v-else class="btn danger" @click="stopJournalFollow">⏹ 停止跟随</button>
        </div>
        <div v-if="err" class="error-msg mt">{{ err }}</div>
        <div v-else-if="jFollowLines !== null">
          <div class="muted mt"><span class="live-dot"></span> 实时跟随中{{ jUnitLabel() }}（新日志自动滚动）</div>
          <pre class="log mt" ref="jFollowBox">{{ jFollowLines.join('\n') }}</pre>
        </div>
        <div v-else-if="journalLines !== null">
          <div class="muted mt" style="display:flex;justify-content:space-between;align-items:center">
            <span>共 {{ journalLines.length }} 行（时间正序）</span>
            <button class="btn" @click="analyzeLog(journalLines)">🤖 AI 分析报错</button>
          </div>
          <pre class="log mt">{{ journalLines.join('\n') }}</pre>
        </div>
        <div v-else class="muted mt loading-tip">(๑>ᴗ<๑) 选择范围后点「查询」～</div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { api, fmtBytes } from '../api.js'
import { openSSE } from '../sse.js'
import { requestAiPrefill } from '../aiBridge.js'
import { useRouter } from 'vue-router'

const router = useRouter()

function analyzeLog(lines) {
  const text = (lines || []).slice(-200).join('\n')
  if (!text.trim()) return
  requestAiPrefill(
    `帮我分析以下日志内容，找出报错和异常，解释可能的原因并给出处理建议：`,
    text.slice(0, 8000),
  )
  router.push('/ai')
}

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

const followLines = ref(null)
const jFollowLines = ref(null)
const following = ref(false)
const jFollowing = ref(false)
const followBox = ref(null)
const jFollowBox = ref(null)
let es = null
let jEs = null

const FOLLOW_CAP = 5000

const pathIsFile = computed(() => /\.[a-zA-Z0-9]+$/.test(path.value) && !path.value.endsWith('/'))

function tailN() {
  const v = Math.floor(Number(tail.value))
  if (!Number.isFinite(v) || v < 1) return 1
  return Math.min(v, 5000)
}

function fmtSize(n) {
  return fmtBytes(n)
}

function pushLines(arr, chunk) {
  for (const l of chunk.split('\n')) arr.push(l)
  if (arr.length > FOLLOW_CAP) arr.splice(0, arr.length - FOLLOW_CAP)
}

function autoScroll(box) {
  nextTick(() => { if (box.value) box.value.scrollTop = box.value.scrollHeight })
}

function startFollow() {
  if (!pathIsFile.value) return
  stopFollow()
  err.value = ''
  result.value = null
  listing.value = null
  followLines.value = []
  following.value = true
  es = openSSE('/logs/follow', { path: path.value, tail: String(Math.min(tailN(), 500)) }, {
    ready: () => {},
    history: (data) => { if (data) pushLines(followLines.value, data); autoScroll(followBox) },
    lines: (data) => { pushLines(followLines.value, data); autoScroll(followBox) },
    note: (data) => { followLines.value.push(data); autoScroll(followBox) },
    error: (msg) => { err.value = msg; stopFollow() },
  })
  es.onerror = () => { if (following.value) { err.value = '连接断开'; stopFollow() } }
}

function stopFollow() {
  if (es) { es.close(); es = null }
  following.value = false
  followLines.value = null
}

function jUnitLabel() {
  const u = journalUnit.value === '__custom' ? customUnit.value : journalUnit.value
  return u ? `: ${u}` : ''
}

function startJournalFollow() {
  stopJournalFollow()
  err.value = ''
  journalLines.value = null
  jFollowLines.value = []
  jFollowing.value = true
  const unit = journalUnit.value === '__custom' ? customUnit.value : journalUnit.value
  const params = {}
  if (unit) params.unit = unit
  if (journalGrep.value) params.grep = journalGrep.value
  jEs = openSSE('/logs/journal/follow', params, {
    ready: () => {},
    lines: (data) => { pushLines(jFollowLines.value, data); autoScroll(jFollowBox) },
    error: (msg) => { err.value = msg; stopJournalFollow() },
  })
  jEs.onerror = () => { if (jFollowing.value) { err.value = '连接断开'; stopJournalFollow() } }
}

function stopJournalFollow() {
  if (jEs) { jEs.close(); jEs = null }
  jFollowing.value = false
  jFollowLines.value = null
}

async function browse(p) {
  err.value = ''
  result.value = null
  stopFollow()
  try {
    listing.value = await api(`/logs/list?path=${encodeURIComponent(p || '/')}`)
    path.value = listing.value.path
  } catch (e) { err.value = e.message; listing.value = null }
}

function pickFile(p) {
  stopFollow()
  path.value = p
  loadFile()
}

async function loadFile() {
  stopFollow()
  err.value = ''
  listing.value = null
  try {
    result.value = await api(`/logs/file?path=${encodeURIComponent(path.value)}&tail=${tailN()}`)
  } catch (e) { err.value = e.message; result.value = null }
}

function switchJournal() {
  stopJournalFollow()
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
  stopJournalFollow()
  err.value = ''
  result.value = null
  try {
    const unit = journalUnit.value === '__custom' ? customUnit.value : journalUnit.value
    const params = new URLSearchParams({ since: journalSince.value, tail: tailN() })
    if (unit) params.set('unit', unit)
    if (journalGrep.value) params.set('grep', journalGrep.value)
    const d = await api(`/logs/journal?${params}`)
    journalLines.value = d.lines || []
  } catch (e) { err.value = e.message; journalLines.value = null }
}

onMounted(() => { browse('/var/log') })
onUnmounted(() => { stopFollow(); stopJournalFollow() })
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
.btn.live { background: #fff0f5; border: 1.5px solid var(--accent); color: var(--accent); }
.live-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: #ff4d6d; margin-right: 4px; animation: blink 1.2s infinite; }
@keyframes blink { 0%,100% { opacity: 1; } 50% { opacity: 0.25; } }
</style>
