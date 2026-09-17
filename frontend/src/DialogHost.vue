<template>
  <div v-if="dialog.open" class="dlg-mask" @click.self="dialogCancel">
    <div class="dlg">
      <div class="dlg-title">{{ dialog.title }}</div>
      <div class="dlg-msg">{{ dialog.message }}</div>
      <input
        v-if="dialog.input"
        ref="inp"
        v-model="dialog.inputValue"
        :placeholder="dialog.inputPlaceholder"
        class="dlg-input"
        @keyup.enter="dialogOk"
      />
      <div class="dlg-btns">
        <button class="btn" @click="dialogCancel">取消</button>
        <button :class="['btn', dialog.danger ? 'danger' : 'primary']" @click="dialogOk">{{ dialog.okText }}</button>
      </div>
    </div>
  </div>
  <div class="toast-box">
    <transition-group name="toast">
      <div v-for="t in toasts" :key="t.id" :class="['toast', t.type]">{{ t.msg }}</div>
    </transition-group>
  </div>
</template>

<script setup>
import { ref, watch, nextTick } from 'vue'
import { dialog, toasts, dialogOk, dialogCancel } from './ui.js'

const inp = ref(null)

watch(() => dialog.open, (v) => {
  if (v && dialog.input) nextTick(() => inp.value && inp.value.focus())
})
</script>

<style>
.dlg-mask {
  position: fixed;
  inset: 0;
  background: rgba(93, 74, 102, 0.35);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.dlg {
  background: var(--panel-solid);
  border: 1px solid rgba(255, 200, 232, 0.8);
  border-radius: 18px;
  padding: 22px;
  width: 340px;
  max-width: calc(100vw - 40px);
  box-shadow: var(--shadow);
  animation: dlg-in 0.18s ease;
}
@keyframes dlg-in {
  from { transform: scale(0.94); opacity: 0; }
  to { transform: scale(1); opacity: 1; }
}
.dlg-title { font-weight: 700; color: var(--accent-deep); margin-bottom: 8px; }
.dlg-msg { color: var(--text); font-size: 14px; line-height: 1.6; word-break: break-all; }
.dlg-input {
  width: 100%;
  margin-top: 12px;
  background: var(--panel2);
  border: 1.5px solid var(--border);
  border-radius: 10px;
  padding: 9px 12px;
  color: var(--text);
  font-size: 14px;
  outline: none;
  transition: all 0.2s;
}
.dlg-input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 4px rgba(255, 126, 182, 0.14);
}
.dlg-btns { display: flex; justify-content: flex-end; gap: 8px; margin-top: 18px; }
.toast-box {
  position: fixed;
  top: 18px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 1100;
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: center;
  pointer-events: none;
}
.toast {
  background: var(--panel-solid);
  border: 1px solid var(--border);
  color: var(--text);
  border-radius: 999px;
  padding: 9px 20px;
  font-size: 13px;
  box-shadow: var(--shadow-sm);
  max-width: 70vw;
  word-break: break-all;
}
.toast.error { border-color: rgba(251, 122, 158, 0.5); color: var(--red); background: #fff5f8; }
.toast.success { border-color: rgba(79, 209, 165, 0.5); color: #2fae88; background: #f2fbf8; }
.toast-enter-active, .toast-leave-active { transition: all 0.25s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
