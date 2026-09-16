<template>
  <div>
    <h2 class="page-title">账号管理 ♪</h2>

    <div class="card">
      <h3>修改我的密码</h3>
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap" class="mt">
        <input v-model="oldPass" type="password" placeholder="原密码" style="width:160px" />
        <input v-model="newPass" type="password" placeholder="新密码（至少6位）" style="width:180px" />
        <button class="btn primary" @click="changePass">确认修改</button>
        <span v-if="passMsg" :class="passOk ? 'muted' : 'error-msg'" style="margin-left:8px">{{ passMsg }}</span>
      </div>
    </div>

    <div class="card mt">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <h3>用户列表</h3>
        <button class="btn" @click="loadUsers">刷新</button>
      </div>
      <table class="mt">
        <thead><tr><th>用户名</th><th>角色</th><th>创建时间</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="u in users" :key="u.username">
            <td>{{ u.username }}</td>
            <td><span class="badge blue">{{ u.role }}</span></td>
            <td class="muted">{{ u.createdAt }}</td>
            <td>
              <button v-if="u.username !== 'admin'" class="btn" style="font-size:12px;padding:4px 10px" @click="resetPass(u.username)">重置密码</button>
              <button v-if="u.username !== 'admin'" class="btn danger" style="font-size:12px;padding:4px 10px" @click="delUser(u.username)">删除</button>
              <span v-else class="muted" style="font-size:12px">内置管理员</span>
            </td>
          </tr>
        </tbody>
      </table>
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap" class="mt">
        <input v-model="newUser" placeholder="新用户名" style="width:160px" />
        <input v-model="newUserPass" placeholder="初始密码（至少6位）" style="width:180px" />
        <button class="btn primary" @click="addUser">✚ 新增用户</button>
        <span v-if="userMsg" :class="userOk ? 'muted' : 'error-msg'">{{ userMsg }}</span>
      </div>
    </div>

    <div class="card mt">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <h3>操作审计（最近 200 条）</h3>
        <button class="btn" @click="loadAudit">刷新</button>
      </div>
      <table class="mt">
        <thead><tr><th>时间</th><th>IP</th><th>对象</th><th>操作</th><th>结果</th></tr></thead>
        <tbody>
          <tr v-for="(a, i) in audit" :key="i">
            <td class="muted" style="white-space:nowrap">{{ a.time }}</td>
            <td class="muted">{{ a.ip }}</td>
            <td>{{ a.target }}</td>
            <td>{{ a.action }}</td>
            <td><span :class="a.result.startsWith('OK') ? 'badge green' : 'badge red'">{{ a.result.slice(0, 40) }}</span></td>
          </tr>
        </tbody>
      </table>
      <div v-if="!audit.length" class="muted mt">(๑´ㅂ`๑) 还没有操作记录</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'

const users = ref([])
const audit = ref([])
const oldPass = ref('')
const newPass = ref('')
const passMsg = ref('')
const passOk = ref(false)
const newUser = ref('')
const newUserPass = ref('')
const userMsg = ref('')
const userOk = ref(false)

async function loadUsers() {
  try { users.value = (await api('/users')) || [] } catch (e) { userMsg.value = e.message }
}
async function loadAudit() {
  try { audit.value = (await api('/audit')) || [] } catch { /* ignore */ }
}

async function changePass() {
  passMsg.value = ''
  try {
    await api('/account/password', { method: 'POST', body: JSON.stringify({ oldPassword: oldPass.value, newPassword: newPass.value }) })
    passOk.value = true
    passMsg.value = '(๑>ᴗ<๑) 修改成功，下次登录用新密码哦'
    oldPass.value = newPass.value = ''
  } catch (e) {
    passOk.value = false
    passMsg.value = e.message
  }
}

async function addUser() {
  userMsg.value = ''
  try {
    await api('/users', { method: 'POST', body: JSON.stringify({ username: newUser.value, password: newUserPass.value }) })
    userOk.value = true
    userMsg.value = '已添加 ✓'
    newUser.value = newUserPass.value = ''
    loadUsers()
  } catch (e) { userOk.value = false; userMsg.value = e.message }
}

async function delUser(name) {
  if (!confirm(`确定删除用户「${name}」吗？`)) return
  try { await api(`/users/${name}`, { method: 'DELETE' }); loadUsers() } catch (e) { userMsg.value = e.message }
}

async function resetPass(name) {
  const p = prompt(`为「${name}」设置新密码（至少6位）：`)
  if (!p) return
  try { await api(`/users/${name}/password`, { method: 'POST', body: JSON.stringify({ password: p }) }); userOk.value = true; userMsg.value = `已重置 ${name} 的密码 ✓` } catch (e) { userOk.value = false; userMsg.value = e.message }
}

onMounted(() => { loadUsers(); loadAudit() })
</script>
