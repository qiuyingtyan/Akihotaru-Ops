

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { api, fmtBytes, getToken } from '../api.js'
import { openSSE } from '../sse.js'
import { requestAiPrefill } from '../aiBridge.js'
import { useRouter } from 'vue-router'
import LogViewer from '../components/LogViewer.vue'

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
const loadingFile = ref(false)
const jLoading = ref(false)
const nameFilter = ref('')

const journalUnit = ref('')
const customUnit = ref('')
const journalSince = ref('1h ago')
const journalPriority = ref('')
const journalGrep = ref('')
const journalLines = ref(null)
const units = ref([])
const unitOpen = ref(false)
const unitKw = ref('')
const unitSearch = ref(null)

const followLines = ref(null)
const jFollowLines = ref(null)
const following = ref(false)
const jFollowing = ref(false)
const followViewer = ref(null)
const jFollowViewer = ref(null)
let es = null
let jEs = null

const FOLLOW_CAP = 5000

const pathIsFile = computed(() => /\.[a-zA-Z0-9]+$/.test(path.value) && !path.value.endsWith('/'))

const crumbs = computed(() => {
  if (!listing.value) return []
  const parts = listing.value.path.split('/').filter(Boolean)
  return parts.map((name, i) => ({ name, path: '/' + parts.slice(0, i + 1).join('/') }))
})

const filteredDirs = computed(() => filterName(listing.value?.dirs))
const filteredFiles = computed(() => filterName(listing.value?.files))

function filterName(arr) {
  const kw = nameFilter.value.trim().toLowerCase()
  if (!kw) return arr || []
  return (arr || []).filter(x => x.name.toLowerCase().includes(kw))
}

const filteredUnits = computed(() => {
  const kw = unitKw.value.trim().toLowerCase()
  if (!kw) return units.value
  return units.value.filter(u => u.toLowerCase().includes(kw))
})

const unitLabel = computed(() => {
  if (journalUnit.value === '__custom') return customUnit.value || '指定 unit…'
  return journalUnit.value || '🌐 全部系统日志'
})

function toggleUnitPanel() {
  unitOpen.value = !unitOpen.value
  if (unitOpen.value) nextTick(() => { if (unitSearch.value) unitSearch.value.focus() })
}

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
    history: (data) => { if (data) pushLines(followLines.value, data); followViewer.value?.scrollToBottom() },
    lines: (data) => { pushLines(followLines.value, data); followViewer.value?.scrollToBottom() },
    note: (data) => { followLines.value.push(data); followViewer.value?.scrollToBottom() },
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
    lines: (data) => { pushLines(jFollowLines.value, data); jFollowViewer.value?.scrollToBottom() },
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
  nameFilter.value = ''
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
  loadingFile.value = true
  try {
    result.value = await api(`/logs/file?path=${encodeURIComponent(path.value)}&tail=${tailN()}`)
  } catch (e) { err.value = e.message; result.value = null } finally { loadingFile.value = false }
}

function downloadFile() {
  if (!pathIsFile.value) return
  window.open(`/api/logs/download?path=${encodeURIComponent(path.value)}&token=${encodeURIComponent(getToken())}`, '_blank')
}

function pickUnit(u) {
  journalUnit.value = u
  unitOpen.value = false
  unitKw.value = ''
}

function switchJournal() {
  stopJournalFollow()
  mode.value = 'journal'
  if (!units.value.length) loadUnits()
}

async function loadUnits() {
  try {
    const d = await api('/logs/journal/units')
    units.value = d.units || []
  } catch {
    try {
      const s = await api('/services')
      units.value = (s.services || []).map(x => x.unit).filter(Boolean).slice(0, 50)
    } catch { /* ignore */ }
  }
}

async function loadJournal() {
  stopJournalFollow()
  err.value = ''
  result.value = null
  jLoading.value = true
  try {
    const unit = journalUnit.value === '__custom' ? customUnit.value : journalUnit.value
    const params = new URLSearchParams({ since: journalSince.value, tail: String(tailN()) })
    if (unit) params.set('unit', unit)
    if (journalGrep.value) params.set('grep', journalGrep.value)
    if (journalPriority.value) params.set('priority', journalPriority.value)
    const d = await api(`/logs/journal?${params}`)
    journalLines.value = d.lines || []
  } catch (e) { err.value = e.message; journalLines.value = null } finally { jLoading.value = false }
}

function onDocClick(e) {
  if (unitOpen.value && !e.target.closest('.unit-select')) unitOpen.value = false
}

onMounted(() => {
  browse('/var/log')
  document.addEventListener('click', onDocClick)
})
onUnmounted(() => {
  stopFollow()
  stopJournalFollow()
  document.removeEventListener('click', onDocClick)
})
</script>

<style scoped>
.tabs { display: flex; gap: 8px; margin-bottom: 12px; }
.tab { border: none; background: transparent; padding: 8px 16px; border-radius: 999px; cursor: pointer; font-size: 14px; color: var(--muted); }
.tab.active { background: linear-gradient(135deg, var(--accent), var(--accent2)); color: #fff; }
.file-list { border: 1.5px solid var(--border); border-radius: 12px; max-height: 300px; overflow-y: auto; padding: 6px; }
.file-row { display: flex; justify-content: space-between; align-items: center; padding: 6px 10px; border-radius: 8px; cursor: pointer; font-size: 14px; }
.file-row:hover { background: rgba(255, 126, 182, 0.12); }
.file-row.selected { background: rgba(183, 148, 246, 0.2); }
.crumb-btn { font-size: 12px; padding: 4px 10px; }
.btn.live { background: #fff0f5; border: 1.5px solid var(--accent); color: var(--accent); }
.live-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: #ff4d6d; margin-right: 4px; animation: blink 1.2s infinite; }
@keyframes blink { 0%,100% { opacity: 1; } 50% { opacity: 0.25; } }

.crumbs { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
.crumb-sep { color: var(--muted); font-size: 12px; }
.crumb-btn.current { background: rgba(255, 126, 182, 0.15); color: var(--accent-deep); font-weight: 600; }

.unit-select { position: relative; }
.unit-toggle { display: flex; align-items: center; gap: 8px; min-width: 180px; }
.unit-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; flex: 1; }
.unit-panel {
  position: absolute; top: calc(100% + 6px); left: 0; z-index: 30;
  width: 320px; max-width: 85vw;
  background: var(--panel-solid);
  border: 1.5px solid var(--border);
  border-radius: 12px;
  box-shadow: var(--shadow);
  overflow: hidden;
}
.unit-panel input {
  width: 100%; border: none; outline: none; padding: 9px 12px;
  border-bottom: 1.5px solid var(--border); font-size: 13px; color: var(--text);
}
.unit-list { max-height: 260px; overflow-y: auto; }
</style>
