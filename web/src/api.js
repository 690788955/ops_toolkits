export async function fetchJSON(path) {
  const res = await fetch(path, {credentials: 'same-origin'})
  return readJSONResponse(res)
}

export function fetchRunDetail(id) {
  return fetchJSON(`/api/runs/${id}`)
}

export function fetchUIPreferences() {
  return fetchJSON('/api/ui/preferences')
}

export async function postJSON(path, payload) {
  const res = await fetch(path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    credentials: 'same-origin',
    body: JSON.stringify(payload)
  })
  return readJSONResponse(res)
}

export async function putJSON(path, payload) {
  const res = await fetch(path, {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    credentials: 'same-origin',
    body: JSON.stringify(payload)
  })
  return readJSONResponse(res)
}

export async function deleteJSON(path) {
  const res = await fetch(path, {method: 'DELETE', credentials: 'same-origin'})
  return readJSONResponse(res)
}

export async function postPluginZip(file, replace) {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`/api/plugins/upload${replace ? '?replace=true' : ''}`, {
    method: 'POST',
    credentials: 'same-origin',
    body: form
  })
  return readJSONResponse(res)
}

function appendUploadFiles(form, file) {
  const files = Array.isArray(file) ? file : Array.from(file?.length !== undefined && typeof file !== 'string' ? file : [file]).filter(Boolean)
  files.forEach(item => {
    form.append('relative_path', item.webkitRelativePath || '')
    form.append('file', item)
  })
}

export async function postPlatformFile(file, targetDir = '') {
  const form = new FormData()
  appendUploadFiles(form, file)
  if (targetDir) form.append('target_dir', targetDir)
  const res = await uploadFetch('/api/files/upload', form)
  return readJSONResponse(res)
}

export async function postRunUploadNode(runID, nodeID, file, targetDir = '') {
  const form = new FormData()
  appendUploadFiles(form, file)
  if (targetDir) form.append('target_dir', targetDir)
  const res = await uploadFetch(`/api/runs/${encodeURIComponent(runID)}/uploads/${encodeURIComponent(nodeID)}`, form)
  return readJSONResponse(res)
}

export async function postRunUploadNodeChunked(runID, nodeID, files, targetDir = '', onProgress = null) {
  const list = Array.isArray(files) ? files : Array.from(files || []).filter(Boolean)
  const start = await postJSON(`/api/runs/${encodeURIComponent(runID)}/uploads/${encodeURIComponent(nodeID)}/start`, {
    target_dir: targetDir,
    files: list.map(file => ({
      name: file.name || 'file',
      relative_path: file.webkitRelativePath || '',
      size: Number(file.size || 0)
    }))
  })
  const sessionID = start?.data?.id
  const serverChunkSize = Number(start?.data?.chunk_size || 0)
  const chunkSize = Math.min(serverChunkSize || 64 * 1024 * 1024, 16 * 1024 * 1024)
  let uploaded = 0
  const total = list.reduce((sum, file) => sum + Number(file.size || 0), 0)
  for (let fileIndex = 0; fileIndex < list.length; fileIndex += 1) {
    const file = list[fileIndex]
    let offset = 0
    while (offset < file.size) {
      const end = Math.min(offset + chunkSize, file.size)
      const chunk = file.slice(offset, end)
      const path = `/api/runs/${encodeURIComponent(runID)}/uploads/${encodeURIComponent(nodeID)}/chunk?session_id=${encodeURIComponent(sessionID)}&file_index=${fileIndex}&offset=${offset}`
      const res = await uploadFetch(path, chunk)
      await readJSONResponse(res)
      uploaded += chunk.size
      offset = end
      if (onProgress) onProgress({uploaded, total, fileIndex, fileName: file.name || 'file'})
    }
  }
  return postJSON(`/api/runs/${encodeURIComponent(runID)}/uploads/${encodeURIComponent(nodeID)}/finish`, {session_id: sessionID})
}

async function uploadFetch(path, form) {
  try {
    return await fetch(path, {
      method: 'POST',
      credentials: 'same-origin',
      body: form
    })
  } catch (err) {
    const message = String(err?.message || err || '')
    const wrapped = new Error(message === 'Failed to fetch' ? '上传连接中断：文件可能超过平台或浏览器限制，或本地服务已断开。' : `上传请求失败：${message}`)
    wrapped.cause = err
    throw wrapped
  }
}

async function readJSONResponse(res) {
  let body = {}
  try {
    body = await res.json()
  } catch {
    body = {error: res.statusText || '服务返回了非 JSON 响应'}
  }
  if (!res.ok) {
    const err = new Error(body.error || res.statusText)
    err.status = res.status
    err.body = body
    throw err
  }
  return body
}
