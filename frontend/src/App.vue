<template>
  <div class="app-root">
    <template v-if="loggedOut === false">
    <aside class="sidebar">
      <div class="logo">(๑>ᴗ<๑) pf3090</div>
      <router-link v-for="m in menus" :key="m.to" :to="m.to">
        {{ $route.path === m.to ? m.on : m.icon }} {{ m.label }}
      </router-link>
      <div class="sidebar-footer">
        <span class="muted" style="font-size:12px">{{ version }}</span>
        <button class="btn" style="margin-top:8px;width:100%" @click="logout">再见啦 ♡</button>
      </div>
    </aside>
    <main class="main">
      <div v-if="alertsCnt > 0" class="alert-banner" @click="$router.push('/alerts')">
        (｡•́︿•̀｡) 有 {{ alertsCnt }} 条告警在闹脾气，点我去看看～
      </div>
      <router-view />
    </main>
    </template>
    <router-view v-else />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api, clearToken, getToken } from './api.js'

const router = useRouter()
const route = useRoute()
const version = ref('')
const alertsCnt = ref(0)
const loggedOut = ref(!getToken())

watch(() => route.path, (p) => {
  loggedOut.value = !getToken() || p === '/login'
})

const menus = [
  { to: '/', label: '总览', icon: '🌸', on: '🌷' },
  { to: '/containers', label: '容器', icon: '📦', on: '🎁' },
  { to: '/projects', label: '项目', icon: '🚀', on: '✨' },
  { to: '/cicd', label: 'CI/CD', icon: '⚙️', on: '🔧' },
  { to: '/services', label: '系统服务', icon: '🖥️', on: '💫' },
  { to: '/logs', label: '日志', icon: '📄', on: '📝' },
  { to: '/alerts', label: '告警', icon: '🔔', on: '🚨' },
]

function logout() {
  clearToken()
  api('/logout', { method: 'POST' }).catch(() => {})
  router.push('/login')
  loggedOut.value = true
}

onMounted(async () => {
  if (!getToken()) return
  try {
    const v = await api('/version')
    version.value = 'v' + v.version
    const a = await api('/alerts')
    alertsCnt.value = a.active.length
    setInterval(async () => {
      if (!document.hidden && getToken()) {
        try { alertsCnt.value = (await api('/alerts')).active.length } catch { /* ignore */ }
      }
    }, 30000)
  } catch { /* not logged in */ }
})
</script>
