<template>
  <div class="app-root">
    <DialogHost />
    <template v-if="loggedOut === false">
    <aside class="sidebar">
      <div class="logo">✨ 秋萤云台</div>

      <!-- 类似 FinalShell 的当前主机快速切换胶囊 -->
      <div class="current-host-card" title="点击切换运维主机 (像 FinalShell 一样自由切换)" @click="$router.push('/servers')">
        <div class="host-status-dot"></div>
        <div class="host-detail">
          <div class="host-title">{{ currentHostName }}</div>
          <div class="host-sub">切换管理主机 ▾</div>
        </div>
      </div>

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
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api, clearToken, getToken } from './api.js'
import { getActiveServer } from './utils/servers.js'
import DialogHost from './DialogHost.vue'

const router = useRouter()
const route = useRoute()
const version = ref('')
const alertsCnt = ref(0)
const loggedOut = ref(!getToken())
const currentHost = ref(getActiveServer())

const currentHostName = computed(() => {
  return currentHost.value ? currentHost.value.name : '未选择主机'
})

watch(() => route.path, (p) => {
  loggedOut.value = !getToken() || p === '/login'
  currentHost.value = getActiveServer()
})

const menus = [
  { to: '/servers', label: '主机工作台', icon: '🖥️', on: '⚡' },
  { to: '/', label: '节点总览', icon: '🌸', on: '🌷' },
  { to: '/projects', label: '业务自愈', icon: '🚀', on: '✨' },
  { to: '/storage', label: '存储守卫', icon: '🛡️', on: '✨' },
  { to: '/containers', label: '容器管理', icon: '📦', on: '🎁' },
  { to: '/cicd', label: 'CI/CD', icon: '⚙️', on: '🔧' },
  { to: '/services', label: '系统服务', icon: '💻', on: '💫' },
  { to: '/logs', label: '日志平台', icon: '📄', on: '📝' },
  { to: '/alerts', label: '告警中心', icon: '🔔', on: '🚨' },
  { to: '/ai', label: 'AI 助手', icon: '🤖', on: '💫' },
  { to: '/users', label: '账号权限', icon: '👤', on: '👑' },
]

function logout() {
  clearToken()
  alertsCnt.value = 0
  api('/logout', { method: 'POST' }).catch(() => {})
  router.push('/login')
  loggedOut.value = true
}

let alertTimer
onMounted(async () => {
  if (!getToken()) return
  try {
    const v = await api('/version')
    version.value = 'v' + v.version
    const a = await api('/alerts')
    alertsCnt.value = a.active.length
    alertTimer = setInterval(async () => {
      if (!document.hidden && getToken()) {
        try { alertsCnt.value = (await api('/alerts')).active.length } catch { /* ignore */ }
      }
    }, 30000)
  } catch { /* not logged in */ }
})
onUnmounted(() => clearInterval(alertTimer))
</script>
