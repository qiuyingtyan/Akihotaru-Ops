<template>
  <div class="login-wrap">
    <div class="login-mascot">✨(๑˃ᴗ˂)ﻭ</div>
    <div class="login-card">
      <h2>秋萤云台 · AkiHotaru</h2>
      <div class="login-sub">秋夜流萤，微光守候，静默自愈 ♡</div>
      <div v-if="err" class="login-err">(｡•́︿•̀｡) {{ err }}</div>
      <input v-model="username" type="text" placeholder="用户名 ♪" autocomplete="username" @keyup.enter="login" />
      <input v-model="password" type="password" placeholder="密码 ♪" autocomplete="current-password" @keyup.enter="login" />
      <button class="btn primary" @click="login" :disabled="loading">{{ loading ? '验证中...' : '进入小屋 ♡' }}</button>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api, setToken } from '../api.js'

const username = ref('')
const password = ref('')
const err = ref('')
const loading = ref(false)

async function login() {
  if (!username.value || !password.value) return
  loading.value = true
  err.value = ''
  try {
    const r = await api('/login', {
      method: 'POST',
      body: JSON.stringify({ username: username.value, password: password.value }),
      noAuth: true
    })
    setToken(r.token)
    location.replace('/')
  } catch (e) {
    err.value = e.message
    loading.value = false
  }
}
</script>
