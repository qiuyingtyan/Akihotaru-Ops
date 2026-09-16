<template>
  <div>
    <aside class="sidebar">
      <div class="logo">🛡 pf3090 运维</div>
      <router-link to="/">📊 总览</router-link>
      <router-link to="/containers">📦 容器</router-link>
      <router-link to="/projects">🚀 项目</router-link>
      <router-link to="/cicd">⚙️ CI/CD</router-link>
      <router-link to="/services">🖥 系统服务</router-link>
      <router-link to="/logs">📄 日志</router-link>
      <router-link to="/alerts">🔔 告警</router-link>
      <div class="sidebar-footer">
        <span class="muted" style="font-size:12px">{{ version }}</span>
        <button class="btn" style="margin-top:8px;width:100%" @click="logout">退出</button>
      </div>
    </aside>
    <main class="main">
      <div v-if="alertsCnt > 0" class="alert-banner" @click="$router.push('/alerts')">
        ⚠️ 当前有 {{ alertsCnt }} 条活跃告警，点击查看
      </div>
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, clearToken, getToken } from './api.js'

const router = useRouter()
const version = ref('')
const alertsCnt = ref(0)

function logout() {
  clearToken()
  router.go(0)
}

onMounted(async () => {
  try {
    const v = await api('/version')
    version.value = 'v' + v.version
    const a = await api('/alerts')
    alertsCnt.value = a.active.length
    setInterval(async () => {
      if (!document.hidden) {
        try { alertsCnt.value = (await api('/alerts')).active.length } catch { /* ignore */ }
      }
    }, 30000)
  } catch { /* router guard handles */ }
})
</script>
