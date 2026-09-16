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
