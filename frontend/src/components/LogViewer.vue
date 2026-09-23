<template>
  <div class="lv-wrap">
    <div class="lv-toolbar">
      <div class="lv-filter">
        <span class="lv-filter-icon">🔍</span>
        <input v-model="kw" :placeholder="placeholder" spellcheck="false" />
        <button v-if="kw" class="lv-clear" title="清空过滤" @click="kw = ''">✕</button>
      </div>
      <span class="lv-stat">
        <template v-if="kw">高亮 <b>{{ hitCount }}</b> 行 / 共 {{ lines.length }} 行</template>
        <template v-else>共 <b>{{ lines.length }}</b> 行</template>
      </span>
      <span v-if="paused" class="lv-paused">⏸ 已暂停自动滚动</span>
      <button class="btn" style="padding:4px 10px;font-size:12px" @click="toggleWrap">{{ wrapText ? '单行横滚' : '自动换行' }}</button>
      <button class="btn" style="padding:4px 12px;font-size:12px" @click="copyAll">{{ copied ? '✓ 已复制' : '复制全部' }}</button>
    </div>
    <div ref="box" class="lv-box" :class="{ 'lv-nowrap': !wrapText }" @scroll="onScroll">
      <div v-if="!lines.length" class="lv-empty">(๑´ㅂ`๑) 暂时没有日志内容</div>
      <div
        v-for="(l, i) in lines"
        :key="i"
        class="lv-line"
        :class="levelClass(l)"
        v-html="renderLine(l)"
      ></div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick } from 'vue'

const props = defineProps({
  lines: { type: Array, default: () => [] },
  placeholder: { type: String, default: '过滤关键字，命中的行会高亮' },
})

const kw = ref('')
const copied = ref(false)
const box = ref(null)
const paused = ref(false)
const wrapText = ref(true)

function toggleWrap() {
  wrapText.value = !wrapText.value
}

const hitCount = computed(() => {
  if (!kw.value) return 0
  const k = kw.value.toLowerCase()
  return props.lines.filter(l => l.toLowerCase().includes(k)).length
})

function levelClass(l) {
  if (/\b(error|fatal|panic|emerg)\b|失败|错误/i.test(l)) return 'lv-err'
  if (/\bwarn(ing)?\b|重试|retry/i.test(l)) return 'lv-warn'
  if (/\bdebug\b/i.test(l)) return 'lv-dbg'
  return ''
}

const escMap = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }
function esc(s) {
  return String(s || '').replace(/[&<>"]/g, c => escMap[c])
}

const tsRegex = /^(\[?\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?\]?|\[?[A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?\]?)\s+/

const tokenPattern = /(Caused by:[^\n\r]*|\bat\s+[a-zA-Z0-9_$.]+\([^\)]*\)|\b(?:error|failure_reason|err|msg)=(?:&quot;.*?&quot;|"[^"]*"|\S+)|\b(?:duration|duration_s|elapsed|cost)=[0-9.]+(?:ms|s|m)?|\b(?:status|code)=(?:&quot;.*?&quot;|"[^"]*"|\S+)|\b(?:ERROR|FATAL|PANIC|EMERG)\b:?|\b(?:WARN|WARNING)\b:?|\b(?:INFO)\b:?|\b(?:DEBUG|TRACE)\b:?|\b(?:SUCCESS|SUCCESSFUL|SUCCEEDED)\b|\b[a-zA-Z0-9_.]*(?:Exception|Error|Failure):?[^\s]*|\b[a-zA-Z_][a-zA-Z0-9_-]*=(?:&quot;.*?&quot;|"[^"]*"|\S+))/gi

function formatTokens(text) {
  return text.replace(tokenPattern, (match) => {
    const m = match.toLowerCase()
    if (m.startsWith('caused by:')) return `<span class="hl-cause">${match}</span>`
    if (m.startsWith('at ') && m.includes('(')) return `<span class="hl-stack">${match}</span>`
    if (m.startsWith('error=') || m.startsWith('failure_reason=') || m.startsWith('err=')) return `<span class="hl-err-kv">${match}</span>`
    if (m.startsWith('duration=') || m.startsWith('duration_s=') || m.startsWith('elapsed=') || m.startsWith('cost=')) return `<span class="hl-dur-kv">${match}</span>`
    if (m.startsWith('status=') || m.startsWith('code=')) {
      if (m.includes('200') || m.includes('success') || m.includes('ok')) return `<span class="hl-succ-kv">${match}</span>`
      if (m.includes('fail') || m.includes('err') || m.includes('500') || m.includes('404')) return `<span class="hl-fail-kv">${match}</span>`
      return `<span class="hl-status-kv">${match}</span>`
    }
    if (m.startsWith('error') || m.startsWith('fatal') || m.startsWith('panic') || m.startsWith('emerg')) return `<span class="hl-err-lbl">${match}</span>`
    if (m.startsWith('warn')) return `<span class="hl-warn-lbl">${match}</span>`
    if (m.startsWith('info')) return `<span class="hl-info-lbl">${match}</span>`
    if (m.startsWith('debug') || m.startsWith('trace')) return `<span class="hl-dbg-lbl">${match}</span>`
    if (m.includes('success') || m.includes('succeeded')) return `<span class="hl-succ">${match}</span>`
    if (m.endsWith('exception') || m.endsWith('error') || m.endsWith('failure') || m.includes('exception:') || m.includes('error:')) return `<span class="hl-exc">${match}</span>`
    if (match.includes('=')) {
      const eq = match.indexOf('=')
      return `<span class="hl-k">${match.slice(0, eq)}</span>=<span class="hl-v">${match.slice(eq + 1)}</span>`
    }
    return match
  })
}

function applyKeyword(html, kw) {
  if (!kw || !kw.trim()) return html
  const escKw = esc(kw.trim()).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const re = new RegExp(`(${escKw})`, 'gi')
  const parts = html.split(/(<[^>]+>)/g)
  return parts.map(p => {
    if (p.startsWith('<') && p.endsWith('>')) return p
    return p.replace(re, '<mark class="lv-hit">$1</mark>')
  }).join('')
}

function renderLine(l) {
  if (!l) return ''
  const raw = esc(l)
  let timeHtml = ''
  let body = raw
  const tsM = raw.match(tsRegex)
  if (tsM) {
    timeHtml = `<span class="hl-time">${tsM[1]}</span> `
    body = raw.slice(tsM[0].length)
  }
  const tokenized = timeHtml + formatTokens(body)
  return applyKeyword(tokenized, kw.value)
}

function copyAll() {
  const text = props.lines.join('\n')
  const done = () => { copied.value = true; setTimeout(() => { copied.value = false }, 1500) }
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(done).catch(() => fallbackCopy(text, done))
  } else {
    fallbackCopy(text, done)
  }
}

function fallbackCopy(text, done) {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  try { document.execCommand('copy'); done() } catch { /* ignore */ }
  ta.remove()
}

function onScroll() {
  const el = box.value
  if (!el) return
  paused.value = el.scrollTop + el.clientHeight < el.scrollHeight - 40
}

function scrollToBottom() {
  if (paused.value) return
  nextTick(() => { if (box.value) box.value.scrollTop = box.value.scrollHeight })
}

watch(() => props.lines.length, scrollToBottom)

defineExpose({ scrollToBottom })
</script>

<style scoped>
.lv-wrap { display: flex; flex-direction: column; border-radius: 12px; overflow: hidden; }
.lv-toolbar {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 8px 12px;
  background: rgba(255, 126, 182, 0.08);
  border: 1.5px solid var(--border);
  border-bottom: none;
  border-radius: 12px 12px 0 0;
  font-size: 12px;
}
.lv-filter {
  display: flex; align-items: center; gap: 6px;
  background: #fff;
  border: 1.5px solid var(--border);
  border-radius: 999px;
  padding: 4px 10px;
  min-width: 220px; flex: 1; max-width: 380px;
}
.lv-filter input { border: none; outline: none; background: transparent; font-size: 13px; width: 100%; color: var(--text); }
.lv-clear { border: none; background: transparent; cursor: pointer; color: var(--muted); font-size: 12px; padding: 0 2px; }
.lv-clear:hover { color: var(--accent-deep); }
.lv-stat { color: var(--muted); white-space: nowrap; }
.lv-stat b { color: var(--accent-deep); }
.lv-paused { color: #d99a2b; font-weight: 600; white-space: nowrap; }
.lv-box {
  background: linear-gradient(160deg, #2b233a, #1d1828);
  border: 1px solid rgba(201, 168, 245, 0.35);
  border-radius: 0 0 12px 12px;
  padding: 10px 0;
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 12px;
  line-height: 1.6;
  overflow: auto;
  max-height: 540px;
  color: #e2e8f0;
  box-shadow: inset 0 2px 10px rgba(0, 0, 0, 0.25);
}
.lv-box.lv-nowrap .lv-line {
  white-space: pre;
  word-break: normal;
}
.lv-line {
  padding: 0 14px;
  white-space: pre-wrap;
  word-break: break-word;
}
.lv-line:hover {
  background: rgba(255, 255, 255, 0.06);
}
.lv-err {
  border-left: 3px solid #f43f5e;
  background: rgba(244, 63, 94, 0.06);
}
.lv-warn {
  border-left: 3px solid #fbbf24;
  background: rgba(251, 191, 36, 0.05);
}
.lv-dbg {
  opacity: 0.85;
}

:deep(.hl-time) {
  color: #38bdf8;
  font-weight: 600;
}
:deep(.hl-cause) {
  color: #ff3366 !important;
  background: rgba(255, 51, 102, 0.18);
  border: 1px solid rgba(255, 51, 102, 0.35);
  border-radius: 4px;
  padding: 1px 6px;
  font-weight: 700;
  display: inline-block;
}
:deep(.hl-stack) {
  color: #94a3b8;
  font-style: italic;
  opacity: 0.88;
}
:deep(.hl-exc) {
  color: #f43f5e;
  font-weight: 700;
}
:deep(.hl-succ) {
  color: #4ade80;
  font-weight: 700;
  background: rgba(74, 222, 128, 0.15);
  padding: 0 4px;
  border-radius: 3px;
}
:deep(.hl-fail) {
  color: #f43f5e;
  font-weight: 700;
  background: rgba(244, 63, 94, 0.18);
  padding: 0 4px;
  border-radius: 3px;
}
:deep(.hl-err-lbl) {
  color: #f87171;
  font-weight: 700;
  background: rgba(248, 113, 113, 0.15);
  padding: 0 4px;
  border-radius: 3px;
}
:deep(.hl-warn-lbl) {
  color: #fbbf24;
  font-weight: 700;
  background: rgba(251, 191, 36, 0.15);
  padding: 0 4px;
  border-radius: 3px;
}
:deep(.hl-info-lbl) {
  color: #60a5fa;
  font-weight: 600;
}
:deep(.hl-dbg-lbl) {
  color: #a78bfa;
}
:deep(.hl-err-kv) {
  color: #fda4af;
  font-weight: 600;
}
:deep(.hl-dur-kv) {
  color: #fde047;
  font-weight: 600;
}
:deep(.hl-succ-kv) {
  color: #86efac;
  font-weight: 600;
}
:deep(.hl-fail-kv) {
  color: #fb7185;
  font-weight: 600;
}
:deep(.hl-status-kv) {
  color: #93c5fd;
}
:deep(.hl-k) {
  color: #94a3b8;
}
:deep(.hl-v) {
  color: #f8fafc;
}

:deep(.lv-hit) {
  background: rgba(255, 214, 102, 0.95);
  color: #3f2a00;
  border-radius: 2px;
  padding: 0 2px;
}
.lv-empty { padding: 18px; text-align: center; color: #bfa8d9; }
</style>
