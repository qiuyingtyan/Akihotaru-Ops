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
      <button class="btn" style="padding:4px 12px;font-size:12px" @click="copyAll">{{ copied ? '✓ 已复制' : '复制全部' }}</button>
    </div>
    <div ref="box" class="lv-box" @scroll="onScroll">
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
  return s.replace(/[&<>"]/g, c => escMap[c])
}

function renderLine(l) {
  let html = esc(l)
  if (kw.value) {
    const k = esc(kw.value).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    try {
      html = html.replace(new RegExp(k, 'gi'), m => `<mark class="lv-hit">${m}</mark>`)
    } catch { /* invalid regex-like input already escaped */ }
  }
  return html
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
  background: linear-gradient(160deg, #43395c, #352d49);
  border: 1px solid rgba(201, 168, 245, 0.35);
  border-radius: 0 0 12px 12px;
  padding: 10px 0;
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 12px;
  line-height: 1.6;
  overflow: auto;
  max-height: 520px;
  color: #f3d9ff;
  box-shadow: inset 0 2px 10px rgba(0, 0, 0, 0.25);
}
.lv-line { padding: 0 14px; white-space: pre-wrap; word-break: break-all; }
.lv-line:hover { background: rgba(255, 255, 255, 0.06); }
.lv-err { color: #ff9db1; }
.lv-warn { color: #ffd479; }
.lv-dbg { color: #b9a8e3; opacity: 0.85; }
.lv-hit { background: rgba(255, 214, 102, 0.85); color: #4a3a10; border-radius: 3px; padding: 0 1px; }
.lv-empty { padding: 18px; text-align: center; color: #bfa8d9; }
</style>
