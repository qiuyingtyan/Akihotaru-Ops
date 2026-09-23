<script setup>
import { ref, onMounted, computed } from 'vue'
import {
  GetDefaultServerUrl,
  OpenExternalBrowser,
  WindowMinimise,
  WindowToggleMaximise,
  WindowClose,
  IsWindowMaximised,
} from '../wailsjs/go/main/App'

const serverList = ref([
  { name: '本地开发节点', url: 'http://127.0.0.1:9800' },
])

const serverUrl = ref('http://127.0.0.1:9800')
const currentUrl = ref('http://127.0.0.1:9800')
const iframeRef = ref(null)
const isLoading = ref(true)
const isMaximised = ref(false)
const showDropdown = ref(false)
const isEditing = ref(false)

const currentNodeName = computed(() => {
  const cleanCurrent = currentUrl.value.replace(/\/$/, '')
  const hit = serverList.value.find(s => s.url.replace(/\/$/, '') === cleanCurrent)
  return hit ? hit.name.split(' ')[0] : '自定义节点'
})

function loadServerList() {
  try {
    const raw = localStorage.getItem('akihotaru_servers_v1')
    if (raw) {
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed) && parsed.length > 0) {
        serverList.value = parsed.map(s => ({ name: s.name, url: s.url }))
      }
    }
  } catch (e) {
    console.error(e)
  }
}

onMounted(async () => {
  loadServerList()
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

  setInterval(async () => {
    try {
      isMaximised.value = await IsWindowMaximised()
    } catch { /* ignore */ }
  }, 500)
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
  currentUrl.value = u
  isEditing.value = false
  showDropdown.value = false
}

function selectServer(s) {
  serverUrl.value = s.url
  navigate()
}

function gotoServersManager() {
  showDropdown.value = false
  const base = currentUrl.value.replace(/\/+$/, '')
  currentUrl.value = `${base}/servers`
  if (iframeRef.value) {
    isLoading.value = true
    iframeRef.value.src = currentUrl.value
  }
}

function reload() {
  if (iframeRef.value) {
    isLoading.value = true
    iframeRef.value.src = currentUrl.value
  }
}

function handleIframeLoad() {
  isLoading.value = false
}

function openInBrowser() {
  OpenExternalBrowser(currentUrl.value)
}

function handleMinimise() {
  WindowMinimise()
}

function handleToggleMaximise() {
  WindowToggleMaximise()
  setTimeout(async () => {
    try { isMaximised.value = await IsWindowMaximised() } catch {}
  }, 100)
}

function handleClose() {
  WindowClose()
}
</script>

<template>
  <div class="window-container" :class="{ 'is-max': isMaximised }">
    <!-- 沉浸式一体化暗夜霓虹标题栏 -->
    <header class="titlebar" @dblclick="handleToggleMaximise">
      <!-- 左侧品牌与状态区 -->
      <div class="titlebar-left">
        <div class="brand-badge" title="秋萤云台 · AkiHotaru - 秋夜流萤，微光守候，静默自愈">
          <span class="brand-flower">✨</span>
          <div class="brand-titles">
            <span class="brand-cn">秋萤云台</span>
            <span class="brand-en">AKIHOTARU</span>
          </div>
        </div>
        <div class="status-pill" title="秋萤守护常驻中，通信链路正常">
          <span class="pulse-dot"></span>
          <span class="status-txt">守护中</span>
        </div>
      </div>

      <!-- 中间极客卡片式服务器切换栏 -->
      <div class="titlebar-center">
        <div class="server-capsule">
          <span class="server-badge">⚡ {{ currentNodeName }}</span>

          <div v-if="!isEditing" class="url-display" @click="isEditing = true">
            <span class="url-text">{{ currentUrl }}</span>
            <span class="edit-hint" title="点击编辑地址">✎</span>
          </div>

          <input
            v-else
            v-model="serverUrl"
            class="url-input"
            autofocus
            @blur="isEditing = false"
            @keyup.enter="navigate"
          />

          <!-- 下拉选择按钮 -->
          <button
            class="capsule-btn dropdown-toggle"
            :class="{ active: showDropdown }"
            title="选择预设服务器"
            @click.stop="showDropdown = !showDropdown"
          >
            ▾
          </button>

          <!-- 刷新按钮 -->
          <button class="capsule-btn" title="刷新页面" @click.stop="reload">
            <span class="icon-spin">🔄</span>
          </button>

          <!-- 系统浏览器打开 -->
          <button class="capsule-btn" title="在默认浏览器中打开此页面" @click.stop="openInBrowser">
            🌐
          </button>

          <!-- 预设服务器下拉菜单 -->
          <div v-if="showDropdown" class="preset-dropdown">
            <div class="dropdown-hd">已纳管主机节点</div>
            <div
              v-for="s in serverList"
              :key="s.url"
              class="dropdown-item"
              :class="{ selected: s.url === currentUrl }"
              @click.stop="selectServer(s)"
            >
              <div class="item-name">{{ s.name }}</div>
              <div class="item-url">{{ s.url }}</div>
            </div>
            <div class="dropdown-action" @click.stop="gotoServersManager">
              <span>⚙️ 管理全部服务器资产...</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧原生级无缝三键 -->
      <div class="titlebar-right">
        <button class="sys-btn btn-min" title="最小化" @click="handleMinimise">
          <svg width="10" height="1" viewBox="0 0 10 1">
            <rect width="10" height="1" fill="currentColor" />
          </svg>
        </button>
        <button class="sys-btn btn-max" :title="isMaximised ? '还原' : '最大化'" @click="handleToggleMaximise">
          <svg v-if="!isMaximised" width="10" height="10" viewBox="0 0 10 10">
            <rect width="9" height="9" x="0.5" y="0.5" fill="none" stroke="currentColor" stroke-width="1" />
          </svg>
          <svg v-else width="10" height="10" viewBox="0 0 10 10">
            <path d="M2.5,0.5 H9.5 V7.5" fill="none" stroke="currentColor" stroke-width="1" />
            <rect width="7" height="7" x="0.5" y="2.5" fill="none" stroke="currentColor" stroke-width="1" />
          </svg>
        </button>
        <button class="sys-btn btn-close" title="关闭" @click="handleClose">
          <svg width="10" height="10" viewBox="0 0 10 10">
            <line x1="1" y1="1" x2="9" y2="9" stroke="currentColor" stroke-width="1.2" />
            <line x1="9" y1="1" x2="1" y2="9" stroke="currentColor" stroke-width="1.2" />
          </svg>
        </button>
      </div>
    </header>

    <!-- 顶栏下方的主体展示区 -->
    <main class="window-body">
      <!-- 极细霓虹渐变加载进度条 -->
      <div v-if="isLoading" class="neon-progress">
        <div class="neon-glow-bar"></div>
      </div>

      <!-- 嵌入的核心运维 Web 控制台 -->
      <iframe
        ref="iframeRef"
        :src="currentUrl"
        class="embedded-frame"
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
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
  background: transparent;
}

/* 整个客户端边框与暗夜霓虹外轮廓 */
.window-container {
  display: flex;
  flex-direction: column;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  background: #171322;
  border: 1px solid rgba(251, 122, 158, 0.32);
  border-radius: 10px;
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.65), inset 0 1px 0 rgba(255, 255, 255, 0.12);
  transition: border-radius 0.2s ease;
}
.window-container.is-max {
  border: none;
  border-radius: 0;
  box-shadow: none;
}

/* 沉浸式一体化暗夜霓虹标题栏 */
.titlebar {
  height: 42px;
  background: radial-gradient(circle at 12% 0%, rgba(251, 122, 158, 0.2) 0%, transparent 55%),
              radial-gradient(circle at 88% 0%, rgba(167, 139, 250, 0.15) 0%, transparent 50%),
              linear-gradient(180deg, #281f3a 0%, #1c162a 100%);
  border-bottom: 1px solid rgba(251, 122, 158, 0.22);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 0 0 14px;
  z-index: 1000;
  flex-shrink: 0;
  --wails-draggable: drag;
}

/* 左侧品牌与呼吸灯 */
.titlebar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  --wails-draggable: drag;
}
.brand-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: default;
}
.brand-flower {
  font-size: 16px;
  filter: drop-shadow(0 0 8px #4ade80);
  animation: glowFirefly 2.5s infinite alternate ease-in-out;
}
@keyframes glowFirefly {
  0% { transform: scale(0.95); filter: drop-shadow(0 0 4px #4ade80); }
  100% { transform: scale(1.1); filter: drop-shadow(0 0 10px #86efac); }
}
.brand-titles {
  display: flex;
  flex-direction: column;
  line-height: 1.1;
}
.brand-cn {
  font-size: 13px;
  font-weight: 800;
  letter-spacing: 0.8px;
  background: linear-gradient(135deg, #a7f3d0, #6ee7b7 40%, #ff9ec6 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}
.brand-en {
  font-size: 8px;
  font-weight: 700;
  letter-spacing: 1.4px;
  color: #c4b5fd;
  opacity: 0.85;
}
.status-pill {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 2px 8px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.25);
  border-radius: 999px;
}
.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #34d399;
  box-shadow: 0 0 8px #34d399;
  animation: pulse 1.8s infinite ease-in-out;
}
@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.45; transform: scale(0.85); }
}
.status-txt {
  font-size: 10px;
  font-weight: 700;
  color: #6ee7b7;
  letter-spacing: 0.5px;
}

/* 中间极客胶囊控制台 */
.titlebar-center {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
  max-width: 640px;
  padding: 0 10px;
  --wails-draggable: drag;
}
.server-capsule {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(14, 10, 22, 0.72);
  border: 1px solid rgba(251, 122, 158, 0.28);
  border-radius: 999px;
  padding: 3px 8px;
  box-shadow: inset 0 1px 3px rgba(0, 0, 0, 0.4), 0 2px 8px rgba(0, 0, 0, 0.25);
  position: relative;
  --wails-draggable: no-drag;
}
.server-badge {
  font-size: 11px;
  font-weight: 700;
  color: #ff9ec6;
  background: rgba(251, 122, 158, 0.18);
  padding: 2px 8px;
  border-radius: 999px;
  white-space: nowrap;
}
.url-display {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
  transition: background 0.15s ease;
}
.url-display:hover {
  background: rgba(255, 255, 255, 0.08);
}
.url-text {
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 12px;
  color: #e2e8f0;
  white-space: nowrap;
}
.edit-hint {
  font-size: 11px;
  color: #94a3b8;
  opacity: 0.7;
}
.url-input {
  font-family: Consolas, "JetBrains Mono", monospace;
  font-size: 12px;
  color: #fff;
  background: transparent;
  border: none;
  border-bottom: 1px solid #fb7a9e;
  outline: none;
  padding: 1px 4px;
  min-width: 220px;
}
.capsule-btn {
  background: transparent;
  border: none;
  color: #cbd5e1;
  font-size: 12px;
  padding: 3px 6px;
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s ease;
}
.capsule-btn:hover {
  background: rgba(255, 255, 255, 0.14);
  color: #fff;
}
.dropdown-toggle.active {
  color: #fb7a9e;
  transform: rotate(180deg);
}
.icon-spin {
  display: inline-block;
  transition: transform 0.3s ease;
}
.capsule-btn:hover .icon-spin {
  transform: rotate(90deg);
}

/* 预设服务器下拉浮层 */
.preset-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
  width: 280px;
  background: #231b34;
  border: 1px solid rgba(251, 122, 158, 0.35);
  border-radius: 10px;
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.6);
  padding: 6px;
  z-index: 2000;
}
.dropdown-hd {
  font-size: 10px;
  font-weight: 700;
  color: #a78bfa;
  padding: 4px 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.dropdown-item {
  padding: 7px 10px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.dropdown-item:hover {
  background: rgba(251, 122, 158, 0.15);
}
.dropdown-item.selected {
  background: rgba(251, 122, 158, 0.22);
  border-left: 2px solid #fb7a9e;
}
.dropdown-action {
  margin-top: 6px;
  padding: 6px 10px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  font-size: 11px;
  color: #a78bfa;
  cursor: pointer;
  border-radius: 4px;
  text-align: center;
  transition: all 0.15s ease;
}
.dropdown-action:hover {
  background: rgba(167, 139, 250, 0.18);
  color: #fff;
}
.item-name {
  font-size: 12px;
  font-weight: 600;
  color: #f1e9f8;
}
.item-url {
  font-size: 11px;
  font-family: Consolas, monospace;
  color: #94a3b8;
  margin-top: 1px;
}

/* 右侧无缝窗口控制三键 */
.titlebar-right {
  display: flex;
  align-items: center;
  height: 100%;
  --wails-draggable: no-drag;
}
.sys-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 42px;
  background: transparent;
  border: none;
  color: #cbd5e1;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}
.sys-btn:hover {
  background: rgba(255, 255, 255, 0.09);
  color: #ffffff;
}
.btn-close:hover {
  background: #f43f5e !important;
  color: #ffffff !important;
}

/* 主体容器与霓虹微光加载条 */
.window-body {
  flex: 1;
  position: relative;
  background: #171322;
  overflow: hidden;
}
.neon-progress {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  background: rgba(0, 0, 0, 0.25);
  z-index: 100;
  overflow: hidden;
}
.neon-glow-bar {
  width: 35%;
  height: 100%;
  background: linear-gradient(90deg, #fb7a9e, #c084fc, #38bdf8);
  box-shadow: 0 0 8px rgba(251, 122, 158, 0.8);
  animation: neonRun 1.2s infinite ease-in-out;
}
@keyframes neonRun {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(350%); }
}
.embedded-frame {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
  background: #ffffff;
}
</style>
