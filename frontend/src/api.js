const TOKEN_KEY = 'opsweb_token'

export function getToken() {
  const q = new URLSearchParams(location.search).get('token')
  if (q) {
    sessionStorage.setItem(TOKEN_KEY, q)
    history.replaceState(null, '', location.pathname)
    return q
  }
  return sessionStorage.getItem(TOKEN_KEY)
}

export function setToken(t) {
  sessionStorage.setItem(TOKEN_KEY, t)
}

export function clearToken() {
  sessionStorage.removeItem(TOKEN_KEY)
}

export async function api(path, opts = {}) {
  const res = await fetch('/api' + path, {
    ...opts,
    headers: {
      'Content-Type': 'application/json',
      Authorization: 'Bearer ' + getToken()
    }
  })
  const body = await res.json().catch(() => ({}))
  if (res.status === 401) {
    clearToken()
    if (location.pathname !== '/login' && !redirecting) {
      redirecting = true
      location.replace('/login')
    }
    throw new Error('未登录或 token 已失效')
  }
  if (!res.ok || body.code !== 0) {
    throw new Error(body.error || `HTTP ${res.status}`)
  }
  return body.data
}

let redirecting = false

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
