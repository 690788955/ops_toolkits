export async function fetchJSON(path) {
  const res = await fetch(path)
  return readJSONResponse(res)
}

export function fetchRunDetail(id) {
  return fetchJSON(`/api/runs/${id}`)
}

export async function postJSON(path, payload) {
  const res = await fetch(path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload)
  })
  return readJSONResponse(res)
}

export async function putJSON(path, payload) {
  const res = await fetch(path, {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload)
  })
  return readJSONResponse(res)
}

export async function deleteJSON(path) {
  const res = await fetch(path, {method: 'DELETE'})
  return readJSONResponse(res)
}

export async function postPluginZip(file, replace) {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`/api/plugins/upload${replace ? '?replace=true' : ''}`, {
    method: 'POST',
    body: form
  })
  return readJSONResponse(res)
}

async function readJSONResponse(res) {
  const body = await res.json()
  if (!res.ok) {
    const err = new Error(body.error || res.statusText)
    err.status = res.status
    err.body = body
    throw err
  }
  return body
}
