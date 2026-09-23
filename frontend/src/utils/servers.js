const STORAGE_KEY = 'akihotaru_servers_v1'
const ACTIVE_KEY = 'akihotaru_active_server_id'

const DEFAULT_SERVERS = [
  {
    id: 'local_node',
    name: '本地运维节点',
    url: window.location.origin.includes('http') ? window.location.origin : 'http://127.0.0.1:9800',
    group: '本地环境',
    description: '当前本机宿主实例',
    tags: ['默认', '单机'],
    status: 'unknown',
    latency: null,
    lastChecked: null,
  }
]

export function getServers() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      saveServers(DEFAULT_SERVERS)
      return [...DEFAULT_SERVERS]
    }
    const list = JSON.parse(raw)
    return Array.isArray(list) ? list : [...DEFAULT_SERVERS]
  } catch (e) {
    console.error('Failed to parse server list:', e)
    return [...DEFAULT_SERVERS]
  }
}

export function saveServers(list) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(list))
    window.dispatchEvent(new CustomEvent('akihotaru-servers-changed', { detail: list }))
  } catch (e) {
    console.error('Failed to save server list:', e)
  }
}

export function getActiveServerId() {
  return localStorage.getItem(ACTIVE_KEY) || 'local_node'
}

export function getActiveServer() {
  const list = getServers()
  const activeId = getActiveServerId()
  const hit = list.find(s => s.id === activeId)
  if (hit) return hit
  if (list.length > 0) {
    setActiveServerId(list[0].id)
    return list[0]
  }
  return null
}

export function setActiveServerId(id) {
  localStorage.setItem(ACTIVE_KEY, id)
  window.dispatchEvent(new CustomEvent('akihotaru-active-server-changed', { detail: id }))
}

export async function pingServer(server) {
  const startTime = Date.now()
  const cleanUrl = server.url.replace(/\/+$/, '')
  const probeUrl = `${cleanUrl}/api/health?_t=${startTime}`

  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), 4000)

  try {
    const res = await fetch(probeUrl, {
      method: 'GET',
      mode: 'cors',
      signal: controller.signal,
    })
    clearTimeout(timer)
    const elapsed = Date.now() - startTime
    if (res.ok) {
      server.status = 'online'
      server.latency = elapsed
    } else {
      server.status = 'error'
      server.latency = elapsed
    }
  } catch (err) {
    clearTimeout(timer)
    server.status = 'offline'
    server.latency = null
  }
  server.lastChecked = Date.now()
  return server
}

export function exportServersJson() {
  const list = getServers()
  const dataStr = 'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(list, null, 2))
  const dlAnchor = document.createElement('a')
  dlAnchor.setAttribute('href', dataStr)
  dlAnchor.setAttribute('download', `akihotaru-servers-${new Date().toISOString().slice(0, 10)}.json`)
  document.body.appendChild(dlAnchor)
  dlAnchor.click()
  dlAnchor.remove()
}

export function importServersJson(jsonStr) {
  const parsed = JSON.parse(jsonStr)
  if (!Array.isArray(parsed)) throw new Error('无效的服务器资产配置格式 (需为数组)')
  for (const s of parsed) {
    if (!s.id || !s.name || !s.url) {
      throw new Error('服务器资产缺少必要字段 (id, name, url)')
    }
  }
  saveServers(parsed)
  if (parsed.length > 0) {
    setActiveServerId(parsed[0].id)
  }
  return parsed
}
