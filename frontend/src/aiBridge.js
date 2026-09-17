// 跨页面与 AI 助手交接的轻量桥：预填消息、携带上下文前缀
const PREFILL_KEY = 'ai-prefill'
const CTX_KEY = 'ai-ctx-inject'

// 其他页面调用：把要分析的内容交给 AI 页，下次激活时消费
export function requestAiPrefill(message, ctx = '') {
  try {
    if (ctx) localStorage.setItem(CTX_KEY, ctx)
    localStorage.setItem(PREFILL_KEY, JSON.stringify({ msg: message, at: Date.now() }))
  } catch { /* ignore */ }
}

// AI 页激活时调用：取出预填内容（一次性，取走即删）
export function takeAiPrefill() {
  try {
    const raw = localStorage.getItem(PREFILL_KEY)
    if (!raw) return null
    localStorage.removeItem(PREFILL_KEY)
    const { msg, at } = JSON.parse(raw)
    if (!msg || Date.now() - at > 60_000) return null
    const ctx = localStorage.getItem(CTX_KEY) || ''
    localStorage.removeItem(CTX_KEY)
    return { msg, ctx }
  } catch { return null }
}
