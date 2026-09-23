<script setup>
import { ref, onMounted } from 'vue'
import { GetDefaultServerUrl, OpenExternalBrowser, ToggleFullscreen } from '../wailsjs/go/main/App'

const serverUrl = ref('http://192.168.1.19:9800')
const currentUrl = ref('http://192.168.1.19:9800')
const iframeRef = ref(null)
const isLoading = ref(true)
const loadError = ref(false)

onMounted(async () => {
  const saved = localStorage.getItem('opsweb_desktop_server')
  if (saved) {
    serverUrl.value = saved
    currentUrl.value = saved
  } else {
    try {
      const def = await GetDefaultServerUrl()
      if (def) {
        serverUrl.value = def
        currentUrl.value = def
      }
    } catch (e) {
      console.error(e)
    }
  }
})

function navigate() {
  let u = serverUrl.value.trim()
  if (!u) return
  if (!/^https?:\/\//i.test(u)) {
    u = 'http://' + u
    serverUrl.value = u
  }
  localStorage.setItem('opsweb_desktop_server', u)
  isLoading.value = true
  loadError.value = false
  currentUrl.value = u
}

function reload() {
  if (iframeRef.value) {
    isLoading.value = true
    loadError.value = false
    iframeRef.value.src = currentUrl.value
  }
}

function handleIframeLoad() {
  isLoading.value = false
}

function openInBrowser() {
  OpenExternalBrowser(currentUrl.value)
}

function toggleMax() {
  ToggleFullscreen()
}
</script>

<template>
  <div class="client-layout">
    <header class="client-header">
      <div class="brand">
        <span class="logo">🌸</span>
        <span class="title">pf3090 运维桌面端</span>
      </div>
      <div class="nav-bar">
        <span class="srv-label">服务器:</span>
        <input
          v-model="serverUrl"
          class="srv-input"
          placeholder="例如 http://192.168.1.19:9800"
          @keyup.enter="navigate"
        />
        <button class="nav-btn primary" title="连接该服务器" @click="navigate">连接</button>
        <button class="nav-btn" title="刷新页面" @click="reload">🔄</button>
      </div>
      <div class="actions">
        <button class="action-btn" title="在系统浏览器中打开" @click="openInBrowser">🌐 浏览器打开</button>
        <button class="action-btn" title="最大化/还原" @click="toggleMax">⛶</button>
      </div>
    </header>

    <main class="client-body">
      <div v-if="isLoading" class="loading-bar">
        <div class="loading-indicator"></div>
      </div>
      <iframe
        ref="iframeRef"
        :src="currentUrl"
        class="content-frame"
        @load="handleIframeLoad"
      ></iframe>
    </main>
  </div>
</template>

<style>
* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
  user-select: none;
}
html, body, #app {
  width: 100%;
  height: 100%;
  overflow: hidden;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  background: #181422;
  color: #f1e9f8;
}
.client-layout {
  display: flex;
  flex-direction: column;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
}
.client-header {
  height: 44px;
  background: linear-gradient(90deg, #271f3a, #1f182f);
  border-bottom: 1px solid rgba(201, 168, 245, 0.25);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  gap: 12px;
  z-index: 100;
  flex-shrink: 0;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
  font-size: 13px;
  color: #ff9ec6;
  white-space: nowrap;
}
.brand .logo {
  font-size: 16px;
}
.nav-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  max-width: 600px;
}
.srv-label {
  font-size: 11px;
  color: #bfa8d9;
  white-space: nowrap;
}
.srv-input {
  flex: 1;
  background: rgba(0, 0, 0, 0.35);
  border: 1px solid rgba(201, 168, 245, 0.3);
  border-radius: 6px;
  padding: 3px 8px;
  color: #fff;
  font-size: 12px;
  outline: none;
  font-family: Consolas, monospace;
}
.srv-input:focus {
  border-color: #fb7a9e;
  box-shadow: 0 0 6px rgba(251, 122, 158, 0.4);
}
.nav-btn {
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(201, 168, 245, 0.2);
  color: #f1e9f8;
  border-radius: 6px;
  padding: 3px 10px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.nav-btn:hover {
  background: rgba(255, 255, 255, 0.18);
}
.nav-btn.primary {
  background: linear-gradient(135deg, #fb7a9e, #e8537f);
  border: none;
  font-weight: 600;
  color: #fff;
}
.nav-btn.primary:hover {
  filter: brightness(1.1);
}
.actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
.action-btn {
  background: transparent;
  border: 1px solid rgba(201, 168, 245, 0.25);
  color: #d8c7ed;
  border-radius: 6px;
  padding: 3px 8px;
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.action-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
}
.client-body {
  flex: 1;
  position: relative;
  background: #181422;
  overflow: hidden;
}
.loading-bar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: rgba(0, 0, 0, 0.2);
  z-index: 10;
  overflow: hidden;
}
.loading-indicator {
  width: 40%;
  height: 100%;
  background: linear-gradient(90deg, #fb7a9e, #a78bfa);
  animation: loadingAnim 1.2s infinite ease-in-out;
}
@keyframes loadingAnim {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(300%); }
}
.content-frame {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
  background: #ffffff;
}
</style>
