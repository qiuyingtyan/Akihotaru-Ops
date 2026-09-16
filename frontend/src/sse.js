import { getToken } from './api.js'

export function sseURL(path, params) {
  const q = new URLSearchParams(params)
  q.set('token', getToken() || '')
  return '/api' + path + '?' + q.toString()
}

export function openSSE(path, params, handlers) {
  const es = new EventSource(sseURL(path, params))
  if (handlers.ready) es.addEventListener('ready', e => handlers.ready(e.data))
  if (handlers.history) es.addEventListener('history', e => handlers.history(e.data))
  if (handlers.lines) es.addEventListener('lines', e => handlers.lines(e.data))
  if (handlers.note) es.addEventListener('note', e => handlers.note && handlers.note(e.data))
  if (handlers.error) es.addEventListener('error', e => {
    if (e.data) handlers.error(e.data)
    else handlers.error('连接已断开')
  })
  return es
}
