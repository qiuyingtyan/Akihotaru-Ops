<template>
  <div class="ai-page">
    <div class="ai-head">
      <h2 class="page-title">AI 运维助手 🤖</h2>
      <div class="ai-head-actions">
        <span v-if="!enabled" class="badge yellow">未配置 API Key</span>
        <span v-else class="badge green">{{ modelName || '已连接' }}</span>
        <button class="btn" @click="toggleHistory">📚 历史（{{ convs.length }}/20）</button>
        <button v-if="isAdmin" class="btn" @click="openSettings">⚙️ 设置</button>
        <button class="btn" :disabled="thinking" @click="newChat">✨ 新对话</button>
      </div>
    </div>

    <div v-if="showHistory" class="card ai-history">
      <div class="ai-history-head">
        <span class="ai-history-title-head">历史对话 <span class="muted">（最多保留 20 次，点击可继续对话）</span></span>
        <button class="btn" @click="toggleHistory">收起</button>
      </div>
      <div v-if="!convs.length" class="ai-history-empty muted">还没有历史对话</div>
      <div
        v-for="cv in convs"
        :key="cv.convId"
        :class="['ai-history-item', { active: cv.convId === convId }]"
      >
        <div class="ai-history-main" @click="openConv(cv)">
          <div class="ai-history-title">{{ cv.title || '（无标题）' }}</div>
          <div class="ai-history-meta">{{ cv.msgs }} 条消息 · 最后 {{ cv.lastTime }}</div>
        </div>
        <button class="btn danger ai-history-del" @click="deleteOne(cv)">删除</button>
      </div>
    </div>

    <div v-if="showSettings" class="card ai-settings">
      <div class="ai-settings-title">AI 接口设置 <span class="muted">（仅管理员）</span></div>
      <div class="ai-settings-grid">
        <label>API Key</label>
        <div class="ai-key-row">
          <input
            v-model="form.apiKey"
            class="ai-input"
            :type="showKey ? 'text' : 'password'"
            :placeholder="hasKey ? `已保存 ${keyMasked}，留空则不修改，输入 - 清除` : 'sk-...'"
          />
          <button class="btn" @click="showKey = !showKey">{{ showKey ? '隐藏' : '显示' }}</button>
        </div>
        <label>接口地址</label>
        <input v-model="form.baseUrl" class="ai-input" placeholder="https://api.deepseek.com（留空用默认）" />
        <label>模型</label>
        <input v-model="form.model" class="ai-input" placeholder="deepseek-chat（留空用默认）" />
      </div>
      <div class="ai-settings-btns">
        <button class="btn" :disabled="testing" @click="testSettings">{{ testing ? '测试中…' : '🔌 测试连通' }}</button>
        <button class="btn primary" :disabled="saving" @click="saveSettings">{{ saving ? '保存中…' : '保存' }}</button>
        <button class="btn" @click="showSettings = false">收起</button>
      </div>
      <p class="ai-foot">Key 保存在服务器数据库中（页面仅显示打码形式）；兼容 OpenAI 接口的服务均可（DeepSeek / Qwen / GLM / Kimi 等）</p>
    </div>

    <div ref="chatBox" class="ai-chat card">
      <div v-if="!messages.length && !thinking" class="ai-welcome">
        <div class="ai-welcome-icon">🌸</div>
        <p>你好，我是 AI 运维助手～</p>
        <p class="muted">可以问我服务器状态、帮你看日志、重启容器或部署项目。危险操作会先经过风险分析并需要你确认哦。</p>
        <div class="ai-suggestions">
          <button v-for="q in suggestions" :key="q" class="btn" @click="send(q)">{{ q }}</button>
        </div>
      </div>

      <div v-for="(m, i) in messages" :key="m.id || 'live-' + i" :class="['ai-msg', m.role]">
        <div class="ai-avatar">{{ m.role === 'user' ? '👤' : '🌸' }}</div>
        <div class="ai-bubble">
          <div v-if="m.time" class="ai-time">{{ m.time }}</div>
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
                <button class="btn danger" :disabled="p.busy" @click="reject(p)">{{ p.busy ? '处理中…' : '拒绝' }}</button>
                <button class="btn primary" :disabled="p.busy" @click="approve(p)">{{ p.busy ? '执行中…' : '✓ 批准执行' }}</button>
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

      <div v-if="followUpLoading" class="ai-msg assistant">
        <div class="ai-avatar">🌸</div>
        <div class="ai-bubble"><div class="ai-typing"><span></span><span></span><span></span></div></div>
      </div>

      <div v-if="followUpText" class="ai-msg assistant">
        <div class="ai-avatar">🌸</div>
        <div class="ai-bubble">
          <div class="ai-text" v-html="renderText(followUpText)"></div>
        </div>
      </div>
    </div>

    <div class="ai-input-bar">
      <div class="ai-input-wrap">
        <input
          v-model="draft"
          class="ai-input"
          placeholder="描述你的问题或想执行的操作…"
          :disabled="thinking"
          @compositionend="imeComposing = false"
          @compositionstart="imeComposing = true"
          @keyup.enter="onEnterKey"
        />
        <div class="ai-ctx" :class="{ warn: ctxPct <= 20 }">
          <div class="ai-ctx-bar"><div class="ai-ctx-fill" :style="{ width: ctxPct + '%' }"></div></div>
          <span class="ai-ctx-label">剩余 {{ ctxPct }}% · {{ ctxUsedK }}k/{{ ctxWindowK }}k</span>
        </div>
      </div>
      <button class="btn primary ai-send" :disabled="thinking || !draft.trim()" @click="send()">发送 ➤</button>
    </div>
    <p class="ai-foot muted">AI 输出仅供参考，写操作和 shell 命令需人工批准，全部记录审计日志</p>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, onMounted } from 'vue'
import { api } from '../api.js'
import { toast, confirmDialog } from '../ui.js'

const messages = ref([])
const draft = ref('')
const thinking = ref(false)
const enabled = ref(false)
const modelName = ref('')
const chatBox = ref(null)

const convId = ref(0)
const convs = ref([])
const showHistory = ref(false)
const ctxUsed = ref(0)
const followUpText = ref('')
const followUpLoading = ref(false)
const imeComposing = ref(false)

function onEnterKey() {
  if (imeComposing.value) return
  send()
}

const isAdmin = ref(false)
const showSettings = ref(false)
const showKey = ref(false)
const testing = ref(false)
const saving = ref(false)
const hasKey = ref(false)
const keyMasked = ref('')
const form = ref({ apiKey: '', baseUrl: '', model: '' })

const suggestions = [
  '服务器现在状态怎么样？',
  '有没有异常的容器或服务？',
  '看看最近的构建任务结果',
  '磁盘使用率多少？',
]

const CTX_WINDOW = 131072
const ctxPct = computed(() => Math.max(0, Math.round(100 - (ctxUsed.value / CTX_WINDOW) * 100)))
const ctxUsedK = computed(() => Math.round(ctxUsed.value / 1000))
const ctxWindowK = computed(() => Math.round(CTX_WINDOW / 1000))

onMounted(async () => {
  try {
    const s = await api('/ai/status')
    enabled.value = s.enabled
    modelName.value = s.model
  } catch { /* ignore */ }
  try {
    await api('/users')
    isAdmin.value = true
  } catch { /* non-admin */ }
  try {
    const st = await api('/ai/settings')
    hasKey.value = st.hasKey
    keyMasked.value = st.keyMasked
    form.value.baseUrl = st.baseUrl || ''
    form.value.model = st.model || ''
    if (st.enabled) enabled.value = true
  } catch { /* non-admin */ }
  try {
    convs.value = (await api('/ai/history')) || []
  } catch { /* no history */ }
})

async function toggleHistory() {
  showHistory.value = !showHistory.value
  if (showHistory.value) await refreshConvs()
}

async function refreshConvs() {
  try { convs.value = (await api('/ai/history')) || [] } catch { /* ignore */ }
}

async function loadConvMessages(id) {
  const h = await api('/ai/history?conv=' + id)
  messages.value = (h || []).map(m => ({
    role: m.role === 'user' ? 'user' : 'assistant',
    content: m.text || '',
    time: m.time,
    pendingCards: (m.cards || []).map(cd => ({
      id: cd.id,
      level: cd.level === 'shell' ? 'shell' : 'write',
      command: cd.command,
      riskHints: cd.riskHints,
      status: cd.status,
      output: cd.output,
    })),
  }))
}

async function openConv(cv) {
  if (thinking.value) { toast('请等当前回复完成', 'error'); return }
  try {
    await loadConvMessages(cv.convId)
    convId.value = cv.convId
    showHistory.value = false
    scrollBottom()
  } catch (e) {
    toast(e.message, 'error')
  }
}

async function deleteOne(cv) {
  const ok = await confirmDialog({ title: '删除历史对话', message: `删除这条历史对话（${cv.msgs} 条消息）？不可恢复。`, danger: true })
  if (!ok) return
  try {
    await api('/ai/history/clear', { method: 'POST', body: JSON.stringify({ conv: cv.convId }) })
    if (cv.convId === convId.value) await newChat(false)
    await refreshConvs()
    toast('已删除该历史对话', 'success')
  } catch (e) {
    toast(e.message, 'error')
  }
}

async function newChat(refreshList = true) {
  if (thinking.value) { toast('请等当前回复完成', 'error'); return }
  try { await api('/ai/reset', { method: 'POST', body: JSON.stringify({ conv: convId.value }) }) } catch { /* ignore */ }
  convId.value = 0
  messages.value = []
  ctxUsed.value = 0
  followUpText.value = ''
  followUpLoading.value = false
  if (refreshList) showHistory.value = false
}

async function clearChat() {
  const hadConv = convId.value > 0
  await newChat()
  if (hadConv) toast('已开始新对话，原对话仍可在历史中查看', 'success')
}

async function openSettings() { showSettings.value = !showSettings.value }

async function testSettings() {
  testing.value = true
  try {
    await api('/ai/settings/test', { method: 'POST', body: JSON.stringify(form.value) })
    toast('连接成功，AI 接口可用 ♡', 'success')
  } catch (e) {
    toast('连接失败: ' + e.message, 'error')
  }
  testing.value = false
}

async function saveSettings() {
  saving.value = true
  try {
    const r = await api('/ai/settings', { method: 'POST', body: JSON.stringify(form.value) })
    enabled.value = r.enabled
    modelName.value = r.model || form.value.model
    toast(r.enabled ? '已保存，AI 助手已启用 ♡' : '已清除配置', 'success')
    form.value.apiKey = ''
    const st = await api('/ai/settings')
    hasKey.value = st.hasKey
    keyMasked.value = st.keyMasked
  } catch (e) {
    toast(e.message, 'error')
  }
  saving.value = false
}

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
  if (!enabled.value) { toast('AI 功能未配置，请联系管理员在设置中填写 API Key', 'error'); return }
  draft.value = ''
  messages.value.push({ role: 'user', content: text })
  thinking.value = true
  scrollBottom()
  try {
    const r = await api('/ai/chat', {
      method: 'POST',
      body: JSON.stringify({ message: text, conv: convId.value }),
    })
    if (!convId.value) {
      convId.value = r.conv
      await refreshConvs()
    }
    messages.value.push({
      role: 'assistant',
      content: r.reply,
      pendingCards: (r.pending || []).map(p => ({ ...p, status: '' })),
    })
    if (r.usage?.prompt_tokens) ctxUsed.value = r.usage.prompt_tokens
  } catch (e) {
    messages.value.push({ role: 'assistant', content: ' :( ' + e.message })
  }
  thinking.value = false
  scrollBottom()
}

async function approve(p) {
  p.busy = true
  followUpText.value = ''
  followUpLoading.value = true
  try {
    const r = await api('/ai/approve', {
      method: 'POST',
      body: JSON.stringify({ id: p.id, conv: convId.value }),
    })
    p.status = '已执行'
    p.output = (r.ok ? '' : '[执行出错]\n') + r.output
    toast(r.ok ? '执行成功 ♡' : '执行出错，请查看输出', r.ok ? 'success' : 'error')
    scrollBottom()
    if (r.follow_up) {
      followUpText.value = r.follow_up
      followUpLoading.value = false
      scrollBottom()
    }
  } catch (e) {
    p.status = '执行失败'
    p.output = e.message
    toast(e.message, 'error')
  }
  if (followUpLoading.value) followUpLoading.value = false
  p.busy = false
  scrollBottom()
  persistFollowUp()
}

async function persistFollowUp() {
  if (!followUpText.value) return
  const text = followUpText.value
  followUpText.value = ''
  messages.value.push({ role: 'assistant', content: text })
}

async function reject(p) {
  p.busy = true
  try {
    await api('/ai/reject', { method: 'POST', body: JSON.stringify({ id: p.id, conv: convId.value }) })
    p.status = '已拒绝'
    toast('已拒绝该操作', 'success')
  } catch (e) {
    toast(e.message, 'error')
  }
  p.busy = false
}
</script>

<style scoped>
.ai-page { display: flex; flex-direction: column; height: calc(100vh - 130px); }
.ai-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; }
.ai-head-actions { display: flex; align-items: center; gap: 8px; }
.ai-history { margin-top: 12px; padding: 12px 16px; }
.ai-history-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.ai-history-title-head { font-weight: 700; color: var(--accent-deep); font-size: 14px; }
.ai-history-empty { padding: 6px 2px; font-size: 13px; }
.ai-history-item {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 10px; border-radius: 10px;
  border: 1px solid transparent;
  transition: all 0.15s;
}
.ai-history-item:hover { background: var(--panel2); }
.ai-history-item.active { border-color: var(--accent); background: rgba(255, 126, 182, 0.08); }
.ai-history-main { flex: 1; cursor: pointer; min-width: 0; }
.ai-history-title {
  font-size: 14px; font-weight: 600; color: var(--text);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.ai-history-meta { font-size: 12px; color: var(--muted); margin-top: 2px; }
.ai-history-del { flex-shrink: 0; padding: 5px 12px; font-size: 12px; }
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
.ai-time { font-size: 11px; color: var(--muted); margin-bottom: 3px; }
.ai-msg.user .ai-time { text-align: right; }
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
.ai-input-bar { display: flex; gap: 10px; margin-top: 12px; align-items: flex-end; }
.ai-input-wrap { flex: 1; display: flex; flex-direction: column; gap: 5px; }
.ai-input {
  width: 100%;
  background: var(--panel-solid);
  border: 1.5px solid var(--border);
  border-radius: 999px;
  padding: 11px 20px;
  font-size: 14px;
  color: var(--text);
  outline: none;
  transition: all 0.2s;
  box-sizing: border-box;
}
.ai-input:focus { border-color: var(--accent); box-shadow: 0 0 0 4px rgba(255, 126, 182, 0.14); }
.ai-ctx { display: flex; align-items: center; gap: 8px; padding: 0 6px; }
.ai-ctx-bar {
  flex: 0 0 120px; height: 6px; border-radius: 999px;
  background: var(--panel2); border: 1px solid var(--border);
  overflow: hidden;
}
.ai-ctx-fill { height: 100%; background: linear-gradient(90deg, var(--accent), #b78cf7); transition: width 0.4s; }
.ai-ctx-label { font-size: 11.5px; color: var(--muted); white-space: nowrap; }
.ai-ctx.warn .ai-ctx-fill { background: linear-gradient(90deg, #f5a623, #f7681c); }
.ai-ctx.warn .ai-ctx-label { color: #d9730f; font-weight: 700; }
.ai-send { padding: 11px 22px; }
.ai-foot { text-align: center; font-size: 12px; margin-top: 8px; }
.ai-settings { margin-top: 12px; padding: 16px 18px; }
.ai-settings-title { font-weight: 700; margin-bottom: 12px; color: var(--accent-deep); }
.ai-settings-grid {
  display: grid;
  grid-template-columns: 90px 1fr;
  gap: 10px 12px;
  align-items: center;
}
.ai-settings-grid label { font-size: 13px; color: var(--muted); font-weight: 600; text-align: right; }
.ai-settings-grid .ai-input { padding: 9px 14px; }
.ai-key-row { display: flex; gap: 8px; }
.ai-key-row .ai-input { flex: 1; }
.ai-settings-btns { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
.ai-settings .ai-foot { text-align: left; margin-top: 10px; }
</style>
