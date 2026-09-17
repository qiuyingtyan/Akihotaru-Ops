<template>
  <div class="ai-page">
    <div class="ai-head">
      <h2 class="page-title">AI 运维助手 🤖</h2>
      <div class="ai-head-actions">
        <span v-if="!enabled" class="badge yellow">未配置 API Key</span>
        <span v-else class="badge green">{{ modelName || '已连接' }}</span>
        <button class="btn" @click="clearChat">🗑️ 新对话</button>
      </div>
    </div>

    <div ref="chatBox" class="ai-chat card">
      <div v-if="!messages.length" class="ai-welcome">
        <div class="ai-welcome-icon">🌸</div>
        <p>你好，我是 AI 运维助手～</p>
        <p class="muted">可以问我服务器状态、帮你看日志、重启容器或部署项目。危险操作会先经过风险分析并需要你确认哦。</p>
        <div class="ai-suggestions">
          <button v-for="q in suggestions" :key="q" class="btn" @click="send(q)">{{ q }}</button>
        </div>
      </div>

      <div v-for="(m, i) in messages" :key="i" :class="['ai-msg', m.role]">
        <div class="ai-avatar">{{ m.role === 'user' ? '👤' : '🌸' }}</div>
        <div class="ai-bubble">
          <div class="ai-text" v-html="renderText(m.content)"></div>
          <div v-if="m.pendingCards?.length" class="ai-cards">
            <div v-for="p in m.pendingCards" :key="p.id" :class="['ai-card', p.level]">
              <div class="ai-card-title">
                <span v-if="p.level === 'shell'" class="badge red">Shell 命令</span>
                <span v-else class="badge yellow">操作确认</span>
                <span v-if="p.status" class="badge" :class="p.status === '已执行' ? 'green' : 'gray'">{{ p.status }}</span>
              </div>
              <pre v-if="p.command" class="ai-card-cmd">{{ p.command }}</pre>
              <div v-if="p.riskHints?.length" class="ai-card-risk">
                ⚠️ 风险点：{{ p.riskHints.join('、') }}
              </div>
              <div v-if="!p.status" class="ai-card-btns">
                <button class="btn danger" :disabled="busy" @click="reject(p)">拒绝</button>
                <button class="btn primary" :disabled="busy" @click="approve(p)">✓ 批准执行</button>
              </div>
              <pre v-if="p.output" class="ai-card-out">{{ p.output }}</pre>
            </div>
          </div>
        </div>
      </div>

      <div v-if="thinking" class="ai-msg assistant">
        <div class="ai-avatar">🌸</div>
        <div class="ai-bubble"><div class="ai-typing"><span></span><span></span><span></span></div></div>
      </div>
    </div>

    <div class="ai-input-bar">
      <input
        v-model="draft"
        class="ai-input"
        placeholder="描述你的问题或想执行的操作…"
        :disabled="thinking"
        @keyup.enter="send()"
      />
      <button class="btn primary ai-send" :disabled="thinking || !draft.trim()" @click="send()">发送 ➤</button>
    </div>
    <p class="ai-foot muted">AI 输出仅供参考，写操作和 shell 命令需人工批准，全部记录审计日志</p>
  </div>
</template>

<script setup>
import { ref, nextTick, onMounted } from 'vue'
import { api } from '../api.js'
import { toast } from '../ui.js'

const messages = ref([])
const draft = ref('')
const thinking = ref(false)
const enabled = ref(false)
const modelName = ref('')
const chatBox = ref(null)

const suggestions = [
  '服务器现在状态怎么样？',
  '有没有异常的容器或服务？',
  '看看最近的构建任务结果',
  '磁盘使用率多少？',
]

onMounted(async () => {
  try {
    const s = await api('/ai/status')
    enabled.value = s.enabled
    modelName.value = s.model
  } catch { /* ignore */ }
})

function esc(s) {
  return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function renderText(text) {
  let h = esc(text)
  h = h.replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
  h = h.replace(/`([^`]+)`/g, '<code>$1</code>')
  h = h.replace(/\n/g, '<br/>')
  return h
}

function scrollBottom() {
  nextTick(() => {
    if (chatBox.value) chatBox.value.scrollTop = chatBox.value.scrollHeight
  })
}

async function send(preset) {
  const text = (preset ?? draft.value).trim()
  if (!text || thinking.value) return
  if (!enabled.value) { toast('AI 功能未配置（缺少 OPSWEB_AI_KEY）', 'error'); return }
  draft.value = ''
  messages.value.push({ role: 'user', content: text })
  thinking.value = true
  scrollBottom()
  try {
    const r = await api('/ai/chat', { method: 'POST', body: JSON.stringify({ message: text }) })
    messages.value.push({
      role: 'assistant',
      content: r.reply,
      pendingCards: (r.pending || []).map(p => ({ ...p, status: '' })),
    })
  } catch (e) {
    messages.value.push({ role: 'assistant', content: ' :( ' + e.message })
  }
  thinking.value = false
  scrollBottom()
}

async function approve(p) {
  p.busy = true
  try {
    const r = await api('/ai/approve', { method: 'POST', body: JSON.stringify({ id: p.id }) })
    p.status = '已执行'
    p.output = (r.ok ? '' : '[执行出错]\n') + r.output
    toast(r.ok ? '执行成功 ♡' : '执行出错，请查看输出', r.ok ? 'success' : 'error')
  } catch (e) {
    p.status = '执行失败'
    p.output = e.message
    toast(e.message, 'error')
  }
  p.busy = false
  scrollBottom()
}

async function reject(p) {
  try {
    await api('/ai/reject', { method: 'POST', body: JSON.stringify({ id: p.id }) })
    p.status = '已拒绝'
    toast('已拒绝该操作', 'success')
  } catch (e) {
    toast(e.message, 'error')
  }
}

function clearChat() {
  api('/ai/reset', { method: 'POST' }).catch(() => {})
  messages.value = []
}
</script>

<style scoped>
.ai-page { display: flex; flex-direction: column; height: calc(100vh - 130px); }
.ai-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; }
.ai-head-actions { display: flex; align-items: center; gap: 8px; }
.ai-chat {
  flex: 1;
  overflow-y: auto;
  margin-top: 12px;
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.ai-welcome { text-align: center; margin: auto; max-width: 420px; }
.ai-welcome-icon { font-size: 42px; margin-bottom: 8px; }
.ai-suggestions { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; margin-top: 14px; }
.ai-msg { display: flex; gap: 10px; align-items: flex-start; }
.ai-msg.user { flex-direction: row-reverse; }
.ai-avatar {
  width: 34px; height: 34px; border-radius: 50%;
  background: var(--panel2); border: 1px solid var(--border);
  display: flex; align-items: center; justify-content: center;
  font-size: 17px; flex-shrink: 0;
}
.ai-bubble {
  max-width: 78%;
  background: var(--panel-solid);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 10px 14px;
  font-size: 14px;
  line-height: 1.65;
  box-shadow: var(--shadow-sm);
  overflow-wrap: break-word;
}
.ai-msg.user .ai-bubble {
  background: linear-gradient(135deg, #ffe3f0, #f0e4ff);
  border-color: rgba(255, 160, 205, 0.5);
}
.ai-text :deep(code) {
  background: var(--panel2); border-radius: 6px; padding: 1px 6px;
  font-size: 13px; color: var(--accent-deep);
}
.ai-cards { margin-top: 10px; display: flex; flex-direction: column; gap: 8px; }
.ai-card {
  border: 1.5px solid var(--border);
  border-radius: 12px;
  padding: 10px 12px;
  background: var(--panel2);
}
.ai-card.shell { border-color: rgba(251, 122, 158, 0.55); background: #fff5f8; }
.ai-card-title { display: flex; gap: 8px; align-items: center; margin-bottom: 6px; }
.ai-card-cmd {
  background: linear-gradient(160deg, #43395c, #352d49);
  color: #ffe9f4; border-radius: 8px; padding: 8px 12px;
  font-size: 13px; overflow-x: auto; white-space: pre-wrap; word-break: break-all;
}
.ai-card-risk { color: #d99a2b; font-size: 13px; margin-top: 6px; }
.ai-card-btns { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; }
.ai-card-out {
  margin-top: 8px; background: #fff; border: 1px solid var(--border);
  border-radius: 8px; padding: 8px 12px; font-size: 12.5px;
  max-height: 260px; overflow: auto; white-space: pre-wrap; word-break: break-all;
}
.ai-typing { display: flex; gap: 5px; padding: 4px 0; }
.ai-typing span {
  width: 8px; height: 8px; border-radius: 50%;
  background: var(--accent); opacity: 0.4;
  animation: ai-blink 1.2s infinite;
}
.ai-typing span:nth-child(2) { animation-delay: 0.2s; }
.ai-typing span:nth-child(3) { animation-delay: 0.4s; }
@keyframes ai-blink { 0%, 80%, 100% { opacity: 0.3; } 40% { opacity: 1; } }
.ai-input-bar { display: flex; gap: 10px; margin-top: 12px; }
.ai-input {
  flex: 1;
  background: var(--panel-solid);
  border: 1.5px solid var(--border);
  border-radius: 999px;
  padding: 11px 20px;
  font-size: 14px;
  color: var(--text);
  outline: none;
  transition: all 0.2s;
}
.ai-input:focus { border-color: var(--accent); box-shadow: 0 0 0 4px rgba(255, 126, 182, 0.14); }
.ai-send { padding: 11px 22px; }
.ai-foot { text-align: center; font-size: 12px; margin-top: 8px; }
</style>
