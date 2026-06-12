import {afterEach, describe, expect, it, vi} from 'vitest'
import {fetchJSON, postJSON, postPluginZip} from './api.js'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('api helpers', () => {
  it('returns JSON body for successful requests', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({data: {ok: true}})})))

    await expect(fetchJSON('/api/catalog')).resolves.toEqual({data: {ok: true}})
    expect(fetch).toHaveBeenCalledWith('/api/catalog')
  })

  it('throws enriched errors for failed JSON responses', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: false, status: 400, statusText: 'Bad Request', json: async () => ({error: '参数错误'})})))

    await expect(fetchJSON('/api/fail')).rejects.toMatchObject({message: '参数错误', status: 400, body: {error: '参数错误'}})
  })

  it('posts JSON payloads with content-type header', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({status: 'saved'})})))

    await expect(postJSON('/api/workflows/demo/save', {workflow: {id: 'demo'}})).resolves.toEqual({status: 'saved'})
    expect(fetch).toHaveBeenCalledWith('/api/workflows/demo/save', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({workflow: {id: 'demo'}})
    })
  })

  it('uploads plugin zips through the replace endpoint when requested', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({status: 'uploaded'})})))

    await expect(postPluginZip(new Blob(['zip']), true)).resolves.toEqual({status: 'uploaded'})
    const [url, options] = fetch.mock.calls[0]
    expect(url).toBe('/api/plugins/upload?replace=true')
    expect(options.method).toBe('POST')
    expect(options.body).toBeInstanceOf(FormData)
  })
})
