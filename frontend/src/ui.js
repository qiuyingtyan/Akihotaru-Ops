import { reactive } from 'vue'

export const dialog = reactive({
  open: false,
  title: '',
  message: '',
  input: null,
  inputValue: '',
  inputPlaceholder: '',
  okText: '确定',
  danger: false,
  _resolve: null
})

export const toasts = reactive([])

let toastSeq = 0

export function toast(msg, type = 'info') {
  const id = ++toastSeq
  toasts.push({ id, msg, type })
  setTimeout(() => {
    const i = toasts.findIndex(t => t.id === id)
    if (i >= 0) toasts.splice(i, 1)
  }, 3500)
}

export function confirmDialog(opts = {}) {
  return new Promise((resolve) => {
    Object.assign(dialog, {
      open: true,
      title: opts.title || '请确认',
      message: opts.message || '',
      input: null,
      inputValue: '',
      inputPlaceholder: '',
      okText: opts.okText || '确定',
      danger: !!opts.danger,
      _resolve: resolve
    })
  })
}

export function promptDialog(opts = {}) {
  return new Promise((resolve) => {
    Object.assign(dialog, {
      open: true,
      title: opts.title || '请输入',
      message: opts.message || '',
      input: true,
      inputValue: '',
      inputPlaceholder: opts.inputPlaceholder || '',
      okText: opts.okText || '确定',
      danger: !!opts.danger,
      _resolve: resolve
    })
  })
}

function close(result) {
  dialog.open = false
  const r = dialog._resolve
  dialog._resolve = null
  if (r) r(result)
}

export function dialogOk() {
  close(dialog.input ? dialog.inputValue : true)
}

export function dialogCancel() {
  close(dialog.input ? null : false)
}
