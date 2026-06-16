import {afterEach, describe, expect, it, vi} from 'vitest'
import {fetchJSON, postJSON, postPluginZip, postRunUploadNode, postRunUploadNodeChunked} from './api.js'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('api helpers', () => {
  it('returns JSON body for successful requests', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({data: {ok: true}})})))

    await expect(fetchJSON('/api/catalog')).resolves.toEqual({data: {ok: true}})
    expect(fetch).toHaveBeenCalledWith('/api/catalog', {credentials: 'same-origin'})
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
      credentials: 'same-origin',
      body: JSON.stringify({workflow: {id: 'demo'}})
    })
  })

  it('uploads plugin zips through the replace endpoint when requested', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({status: 'uploaded'})})))

    await expect(postPluginZip(new Blob(['zip']), true)).resolves.toEqual({status: 'uploaded'})
    const [url, options] = fetch.mock.calls[0]
    expect(url).toBe('/api/plugins/upload?replace=true')
    expect(options.method).toBe('POST')
    expect(options.credentials).toBe('same-origin')
    expect(options.body).toBeInstanceOf(FormData)
  })

  it('uploads files to a waiting workflow upload node', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({status: 'uploaded'})})))

    await expect(postRunUploadNode('workflow-1', 'upload', [new Blob(['data'])], 'assets')).resolves.toEqual({status: 'uploaded'})
    const [url, options] = fetch.mock.calls[0]
    expect(url).toBe('/api/runs/workflow-1/uploads/upload')
    expect(options.method).toBe('POST')
    expect(options.credentials).toBe('same-origin')
    expect(options.body).toBeInstanceOf(FormData)
  })

  it('wraps browser upload network failures with readable errors', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('Failed to fetch')
    }))

    await expect(postRunUploadNode('workflow-1', 'upload', [new Blob(['data'])])).rejects.toThrow('上传连接中断')
  })

  it('uploads workflow files through chunked node upload endpoints', async () => {
    vi.stubGlobal('fetch', vi.fn(async url => {
      if (String(url).endsWith('/start')) return {ok: true, json: async () => ({data: {id: 'upload-1', chunk_size: 4}})}
      if (String(url).includes('/chunk?')) return {ok: true, json: async () => ({status: 'chunked'})}
      if (String(url).endsWith('/finish')) return {ok: true, json: async () => ({status: 'uploaded'})}
      return {ok: false, status: 404, statusText: 'Not Found', json: async () => ({error: 'not found'})}
    }))
    const file = new File(['abcdef'], 'a.txt')

    await expect(postRunUploadNodeChunked('workflow-1', 'upload', [file], 'assets')).resolves.toEqual({status: 'uploaded'})
    expect(fetch.mock.calls.map(call => call[0])).toEqual([
      '/api/runs/workflow-1/uploads/upload/start',
      '/api/runs/workflow-1/uploads/upload/chunk?session_id=upload-1&file_index=0&offset=0',
      '/api/runs/workflow-1/uploads/upload/chunk?session_id=upload-1&file_index=0&offset=4',
      '/api/runs/workflow-1/uploads/upload/finish'
    ])
  })
})
