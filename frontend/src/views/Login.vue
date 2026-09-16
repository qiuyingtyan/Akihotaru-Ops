<template>
  <div class="login-wrap">
    <div class="login-card">
      <h2>🛡 pf3090 运维面板</h2>
      <div v-if="err" class="login-err">{{ err }}</div>
      <input v-model="token" type="password" placeholder="请输入访问 Token" @keyup.enter="login" />
      <button class="btn" @click="login" :disabled="loading">{{ loading ? '验证中...' : '登 录' }}</button>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, setToken, getToken } from '../api.js'

const router = useRouter()
const token = ref(getToken() || '')
const err = ref('')
const loading = ref(false)

async function login() {
  if (!token.value) return
  loading.value = true
  err.value = ''
  const saved = sessionStorage.getItem('opsweb_token')
  setToken(token.value)
  try {
    await api('/ping')
    router.go(0)
  } catch (e) {
    sessionStorage.setItem('opsweb_token', saved || '')
    err.value = 'Token 无效：' + e.message
    loading.value = false
  }
}
</script>
