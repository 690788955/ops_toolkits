export async function fetchJSON(path) {
  const res = await fetch(path, {credentials: 'same-origin'})
  return readJSONResponse(res)
}

export function fetchRunDetail(id) {
  return fetchJSON(`/api/runs/${id}`)
}

export function fetchRunSummary(id) {
  return fetchJSON(`/api/runs/${encodeURIComponent(id)}?summary=1`)
}

export function fetchPendingRuns(limit = 50) {
  return fetchJSON(`/api/runs?status=pending&limit=${encodeURIComponent(limit)}`)
}

export function retryPendingRunCheck(runID) {
  return postJSON(`/api/runs/${encodeURIComponent(runID)}/recovery/check`, {})
}

export function rollbackPendingRun(runID) {
  return postJSON(`/api/runs/${encodeURIComponent(runID)}/recovery/rollback`, {})
}

export function resolvePendingRun(runID, note = '') {
  return postJSON(`/api/runs/${encodeURIComponent(runID)}/recovery/resolve`, {note})
}

export function retryRunNotification(runID, index) {
  return postJSON(`/api/runs/${encodeURIComponent(runID)}/notifications/${encodeURIComponent(index)}/retry`, {})
}

export function fetchUIPreferences() {
  return fetchJSON('/api/ui/preferences')
}

// Vite serves the SPA itself in dev, so ?token= never reaches the Go middleware.
// Make one API call to let the backend set the HttpOnly session cookie.
export async function bootstrapAuthTokenFromURL() {
  if (typeof window === 'undefined') return
  const token = new URLSearchParams(window.location.search).get('token')
  if (!token) return
  try {
    await fetch(`/api/ui/preferences?token=${encodeURIComponent(token)}`, {credentials: 'same-origin'})
  } catch {
    // The app will surface the 401 on its own if the token is invalid.
  }
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

export function fetchConfigVersions() {
  return fetchJSON('/api/config/global/versions/')
}

export function saveConfigVersion(payload) {
  return postJSON('/api/config/global/versions/', payload)
}

export function fetchConfigVersion(version) {
  return fetchJSON(`/api/config/global/versions/${encodeURIComponent(version)}`)
}

export function deleteConfigVersion(version) {
  return deleteJSON(`/api/config/global/versions/${encodeURIComponent(version)}`)
}

export function setDefaultConfigVersion(version) {
  return putJSON(`/api/config/global/versions/${encodeURIComponent(version)}/default`, {})
}

export function fetchMarket(refresh = false) {
  return fetchJSON(`/api/market${refresh ? '?refresh=1' : ''}`)
}

export function postMarketInstall(payload) {
  return postJSON('/api/market/install', payload)
}

export function fetchMarketSources() {
  return fetchJSON('/api/market/sources')
}

export function createMarketSource(payload) {
  return postJSON('/api/market/sources', payload)
}

export function updateMarketSource(payload) {
  return putJSON('/api/market/sources', payload)
}

export function deleteMarketSource(id) {
  const query = new URLSearchParams({id})
  return deleteJSON(`/api/market/sources?${query.toString()}`)
}

export function testMarketSource(payload) {
  return postJSON('/api/market/sources/test', payload)
}

function sourceQuery(sourceID, values = {}) {
  const query = new URLSearchParams()
  if (sourceID) query.set('source', sourceID)
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && value !== null && value !== '') query.set(key, value)
  }
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

export function fetchNexusPlugins(sourceID = '') {
  return fetchJSON(`/api/nexus/plugins${sourceQuery(sourceID)}`)
}

export async function postNexusPluginZip(sourceID, file, replace, onProgress) {
  const form = new FormData()
  form.append('file', file)
  const path = `/api/nexus/upload${sourceQuery(sourceID, {replace: replace ? 'true' : ''})}`
  if (!onProgress) {
    const res = await uploadFetch(path, form)
    return readJSONResponse(res)
  }
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', path)
    xhr.withCredentials = true
    xhr.upload.onprogress = event => {
      if (event.lengthComputable) onProgress({loaded: event.loaded, total: event.total})
    }
    xhr.onload = () => {
      let body = {}
      try {
        body = JSON.parse(xhr.responseText || '{}')
      } catch {
        body = {error: xhr.statusText || '服务返回了非 JSON 响应'}
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(body)
        return
      }
      const err = new Error(body.error || xhr.statusText || '上传失败')
      err.status = xhr.status
      err.body = body
      reject(err)
    }
    xhr.onerror = () => reject(new Error('上传连接中断：文件可能超过平台或浏览器限制，或本地服务已断开。'))
    xhr.send(form)
  })
}

export function deleteNexusPlugin(sourceID, id, version) {
  return deleteJSON(`/api/nexus/plugin${sourceQuery(sourceID, {id, version})}`)
}

export function rebuildNexusIndex(sourceID) {
  return postJSON(`/api/nexus/rebuild${sourceQuery(sourceID)}`, {})
}

export function nexusDownloadURL(sourceID, id, version) {
  return `/api/nexus/download${sourceQuery(sourceID, {id, version})}`
}

export function fetchNexusShareTargets() {
  return fetchJSON('/api/nexus/share/targets')
}

export function postPluginShare(pluginID, sourceID, replace) {
  return postJSON(`/api/plugins/${encodeURIComponent(pluginID)}/share${sourceQuery(sourceID, {replace: replace ? 'true' : ''})}`, {})
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

export function rescanPlugins() {
  return postJSON('/api/plugins/rescan', {})
}

export async function downloadUserWorkflowExport(workflowIDs, includeDependencies) {
  const query = new URLSearchParams()
  for (const id of workflowIDs || []) {
    if (id) query.append('workflow', id)
  }
  if (includeDependencies) query.set('include_dependencies', '1')
  const res = await fetch(`/api/plugins/user-workflows.zip?${query.toString()}`, {credentials: 'same-origin'})
  return downloadResponse(res, includeDependencies ? 'user-workflows-bundle.zip' : 'user.workflows.zip')
}

async function downloadResponse(res, fallbackName) {
  if (!res.ok) {
    let body = {}
    try {
      body = await res.json()
    } catch {
      body = {error: res.statusText || '导出失败'}
    }
    const err = new Error(body.error || res.statusText || '导出失败')
    err.status = res.status
    err.body = body
    throw err
  }
  const filename = responseFilename(res) || fallbackName
  const url = URL.createObjectURL(await res.blob())
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  return filename
}

function responseFilename(res) {
  const disposition = res.headers.get('Content-Disposition') || ''
  const match = /filename="?([^";]+)"?/i.exec(disposition)
  return match ? match[1] : ''
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

export function bindWorkflowConfigPath(workflowID, payload) {
  return postJSON(`/api/workflows/${encodeURIComponent(workflowID)}/files`, payload)
}

export async function uploadWorkflowConfigFile(workflowID, file, subdir = '') {
  const form = new FormData()
  form.append('file', file)
  if (subdir) form.append('subdir', subdir)
  const res = await uploadFetch(`/api/workflows/${encodeURIComponent(workflowID)}/files/upload`, form)
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
