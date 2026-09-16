const TOKEN_KEY = 'opsweb_token'

export function getToken() {
  return sessionStorage.getItem(TOKEN_KEY)
}

export function setToken(t) {
  sessionStorage.setItem(TOKEN_KEY, t)
}

export function clearToken() {
  sessionStorage.removeItem(TOKEN_KEY)
}

let redirecting = false

export async function api(path, opts = {}) {
  const headers = { 'Content-Type': 'application/json' }
  if (!opts.noAuth) {
    headers.Authorization = 'Bearer ' + getToken()
  }
  const res = await fetch('/api' + path, { ...opts, headers })
  const body = await res.json().catch(() => ({}))
  if (res.status === 401) {
    clearToken()
    if (!opts.noAuth && location.pathname !== '/login' && !redirecting) {
      redirecting = true
      location.replace('/login')
    }
    throw new Error(opts.noAuth ? (body.error || '登录失败') : '未登录或会话已过期')
  }
  if (!res.ok || body.code !== 0) {
    throw new Error(body.error || `HTTP ${res.status}`)
  }
  return body.data
}

export function fmtBytes(n) {
  if (n == null) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = Number(n)
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return v.toFixed(i === 0 ? 0 : 1) + ' ' + units[i]
}

export function fmtUptime(sec) {
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}天${h}小时`
  if (h > 0) return `${h}小时${m}分`
  return `${m}分钟`
}

export function pctColor(p) {
  if (p >= 90) return 'var(--red)'
  if (p >= 75) return 'var(--yellow)'
  return 'var(--green)'
}
