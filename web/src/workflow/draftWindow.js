// 独立草稿窗口：主窗口把当前草稿写进同源 localStorage，新窗口启动时读回；
// 草稿窗口保存后写回「已保存」标记，主窗口通过 storage 事件得知并提示重新加载。
export const DRAFT_WINDOW_PARAM = 'draftWindow'
export const DRAFT_WINDOW_TRANSFER_KEY = 'ops.workflowEditor.draftWindow.transfer'
export const DRAFT_WINDOW_SAVED_KEY = 'ops.workflowEditor.draftWindow.saved'

export function isDraftWindowLocation(search = '') {
  try {
    return new URLSearchParams(String(search || '')).get(DRAFT_WINDOW_PARAM) === '1'
  } catch {
    return false
  }
}

export function draftWindowURL(href) {
  const url = new URL(href)
  url.searchParams.set(DRAFT_WINDOW_PARAM, '1')
  url.hash = ''
  return url.toString()
}

function readStorage(key) {
  try {
    const raw = window.localStorage?.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function writeStorage(key, value) {
  try {
    window.localStorage?.setItem(key, JSON.stringify(value))
    return true
  } catch {
    return false
  }
}

function removeStorage(key) {
  try {
    window.localStorage?.removeItem(key)
  } catch {
    // 存储不可用（隐私模式等）时忽略即可。
  }
}

export function writeDraftWindowTransfer(payload = {}) {
  if (!payload.draft || typeof payload.draft !== 'object') return false
  return writeStorage(DRAFT_WINDOW_TRANSFER_KEY, {
    id: String(payload.id || payload.draft.id || ''),
    name: String(payload.name || payload.draft.name || ''),
    ownerID: String(payload.ownerID || 'user.workflows'),
    ownerLocked: Boolean(payload.ownerLocked),
    dirty: Boolean(payload.dirty),
    ts: Number(payload.ts) || Date.now(),
    draft: payload.draft
  })
}

export function readDraftWindowTransfer() {
  const transfer = readStorage(DRAFT_WINDOW_TRANSFER_KEY)
  if (!transfer || !transfer.draft || typeof transfer.draft !== 'object') return null
  return {
    id: String(transfer.id || transfer.draft.id || ''),
    name: String(transfer.name || transfer.draft.name || ''),
    ownerID: String(transfer.ownerID || 'user.workflows'),
    ownerLocked: Boolean(transfer.ownerLocked),
    dirty: Boolean(transfer.dirty),
    ts: Number(transfer.ts) || 0,
    draft: transfer.draft
  }
}

export function clearDraftWindowTransfer() {
  removeStorage(DRAFT_WINDOW_TRANSFER_KEY)
}

export function writeDraftWindowSaved(record = {}) {
  if (!record.id) return false
  return writeStorage(DRAFT_WINDOW_SAVED_KEY, {
    id: String(record.id),
    name: String(record.name || record.id),
    ts: Number(record.ts) || Date.now()
  })
}

export function readDraftWindowSaved() {
  const saved = readStorage(DRAFT_WINDOW_SAVED_KEY)
  if (!saved || !saved.id) return null
  return {id: String(saved.id), name: String(saved.name || saved.id), ts: Number(saved.ts) || 0}
}

export function clearDraftWindowSaved() {
  removeStorage(DRAFT_WINDOW_SAVED_KEY)
}

export function draftWindowSavedRefValue(saved) {
  return saved?.id ? `${saved.id}@${saved.ts}` : ''
}

export function formatDraftWindowTime(ts) {
  const value = Number(ts)
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = part => String(part).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}