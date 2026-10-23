import {afterEach, describe, expect, it, vi} from 'vitest'
import {bindWorkflowConfigPath, bootstrapAuthTokenFromURL, createMarketSource, deleteMarketSource, deleteNexusPlugin, downloadUserWorkflowExport, fetchJSON, fetchMarket, fetchMarketSources, fetchNexusPlugins, nexusDownloadURL, postJSON, postMarketInstall, postNexusPluginZip, postPluginZip, postRunUploadNode, postRunUploadNodeChunked, rebuildNexusIndex, testMarketSource, updateMarketSource, uploadWorkflowConfigFile} from './api.js'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
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

  it('exchanges a URL token for the session cookie', async () => {
    vi.stubGlobal('window', {location: {search: '?token=abc123'}})
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({data: {}})})))

    await bootstrapAuthTokenFromURL()

    expect(fetch).toHaveBeenCalledWith('/api/ui/preferences?token=abc123', {credentials: 'same-origin'})
  })

  it('skips token bootstrap when the URL has no token', async () => {
    vi.stubGlobal('window', {location: {search: '?tab=workflows'}})
    vi.stubGlobal('fetch', vi.fn())

    await bootstrapAuthTokenFromURL()

    expect(fetch).not.toHaveBeenCalled()
  })

  it('fetches market catalog with optional cache refresh', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({data: {sources: []}})})))

    await expect(fetchMarket()).resolves.toEqual({data: {sources: []}})
    await fetchMarket(true)
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/market', '/api/market?refresh=1'])
  })

  it('posts market install requests', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({status: 'installed'})})))

    await expect(postMarketInstall({source_id: 'internal', plugin_id: 'demo.sample', version: '1.0.0', replace: false})).resolves.toEqual({status: 'installed'})
    const [url, options] = fetch.mock.calls[0]
    expect(url).toBe('/api/market/install')
    expect(options.method).toBe('POST')
    expect(JSON.parse(options.body)).toEqual({source_id: 'internal', plugin_id: 'demo.sample', version: '1.0.0', replace: false})
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

  it('downloads selected user workflows with optional dependency plugins', async () => {
    const blob = new Blob(['zip'])
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      headers: {get: () => 'attachment; filename="user-workflows-bundle.zip"'},
      blob: async () => blob
    })))
    const createObjectURL = vi.fn(() => 'blob:test')
    vi.stubGlobal('URL', {createObjectURL, revokeObjectURL: vi.fn()})
    const link = {href: '', download: '', click: vi.fn(), remove: vi.fn()}
    vi.stubGlobal('document', {createElement: () => link, body: {appendChild: vi.fn()}})
    vi.stubGlobal('window', {setTimeout: vi.fn()})

    await expect(downloadUserWorkflowExport(['demo.one', 'demo.two'], true)).resolves.toBe('user-workflows-bundle.zip')
    expect(fetch.mock.calls[0][0]).toBe('/api/plugins/user-workflows.zip?workflow=demo.one&workflow=demo.two&include_dependencies=1')
    expect(createObjectURL).toHaveBeenCalledWith(blob)
    expect(link.download).toBe('user-workflows-bundle.zip')
    expect(link.click).toHaveBeenCalled()
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

  it('binds and uploads workflow config files', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({status: 'bound'})})))

    await expect(bindWorkflowConfigPath('demo.flow', {path: 'runs/uploads/upload-1/app.conf', label: '应用配置'})).resolves.toEqual({status: 'bound'})
    const [bindURL, bindOptions] = fetch.mock.calls[0]
    expect(bindURL).toBe('/api/workflows/demo.flow/files')
    expect(bindOptions.method).toBe('POST')
    expect(JSON.parse(bindOptions.body)).toEqual({path: 'runs/uploads/upload-1/app.conf', label: '应用配置'})

    await expect(uploadWorkflowConfigFile('demo.flow', new Blob(['app: true\n']), 'release')).resolves.toEqual({status: 'bound'})
    const [uploadURL, uploadOptions] = fetch.mock.calls[1]
    expect(uploadURL).toBe('/api/workflows/demo.flow/files/upload')
    expect(uploadOptions.method).toBe('POST')
    expect(uploadOptions.body).toBeInstanceOf(FormData)
    expect(uploadOptions.body.get('subdir')).toBe('release')
    expect(uploadOptions.body.get('file')).toBeTruthy()
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

  it('manages market sources', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({data: {sources: []}})})))

    await expect(fetchMarketSources()).resolves.toEqual({data: {sources: []}})
    await expect(createMarketSource({id: 'nexus', type: 'nexus', url: 'http://nexus.example.com'})).resolves.toEqual({data: {sources: []}})
    await expect(updateMarketSource({id: 'nexus', type: 'nexus', url: 'http://nexus.example.com'})).resolves.toEqual({data: {sources: []}})
    await expect(deleteMarketSource('nexus')).resolves.toEqual({data: {sources: []}})
    await expect(testMarketSource({type: 'nexus', url: 'http://nexus.example.com'})).resolves.toEqual({data: {sources: []}})

    expect(fetch.mock.calls.map(call => [call[0], call[1]?.method])).toEqual([
      ['/api/market/sources', undefined],
      ['/api/market/sources', 'POST'],
      ['/api/market/sources', 'PUT'],
      ['/api/market/sources?id=nexus', 'DELETE'],
      ['/api/market/sources/test', 'POST']
    ])
  })

  it('uploads and lists nexus plugins by market source', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({data: {plugins: []}})})))

    await expect(fetchNexusPlugins('nexus')).resolves.toEqual({data: {plugins: []}})
    await expect(postNexusPluginZip('nexus', new Blob(['zip']), false)).resolves.toEqual({data: {plugins: []}})
    await expect(postNexusPluginZip('nexus', new Blob(['zip']), true)).resolves.toEqual({data: {plugins: []}})
    await expect(fetchNexusPlugins()).resolves.toEqual({data: {plugins: []}})

    expect(fetch.mock.calls.map(call => call[0])).toEqual([
      '/api/nexus/plugins?source=nexus',
      '/api/nexus/upload?source=nexus',
      '/api/nexus/upload?source=nexus&replace=true',
      '/api/nexus/plugins'
    ])
    const uploadOptions = fetch.mock.calls[1][1]
    expect(uploadOptions.method).toBe('POST')
    expect(uploadOptions.credentials).toBe('same-origin')
    expect(uploadOptions.body).toBeInstanceOf(FormData)
  })

  it('reports nexus upload progress and rejects failed XHR uploads', async () => {
    class FakeXHR {
      constructor() {
        FakeXHR.instance = this
        this.upload = {}
        this.status = 0
        this.statusText = ''
        this.responseText = ''
      }
      open(method, path) {
        this.method = method
        this.path = path
      }
      send(body) {
        this.body = body
      }
    }
    vi.stubGlobal('XMLHttpRequest', FakeXHR)

    const onProgress = vi.fn()
    const pending = postNexusPluginZip('nexus', new Blob(['zip']), false, onProgress)
    const xhr = FakeXHR.instance
    expect(xhr.method).toBe('POST')
    expect(xhr.path).toBe('/api/nexus/upload?source=nexus')
    expect(xhr.withCredentials).toBe(true)
    expect(xhr.body).toBeInstanceOf(FormData)

    xhr.upload.onprogress({lengthComputable: true, loaded: 40, total: 100})
    xhr.upload.onprogress({lengthComputable: false, loaded: 0, total: 0})
    expect(onProgress).toHaveBeenCalledTimes(1)
    expect(onProgress).toHaveBeenCalledWith({loaded: 40, total: 100})

    xhr.status = 200
    xhr.responseText = JSON.stringify({status: 'uploaded'})
    xhr.onload()
    await expect(pending).resolves.toEqual({status: 'uploaded'})

    const failed = postNexusPluginZip('nexus', new Blob(['zip']), true, () => {})
    const failedXHR = FakeXHR.instance
    failedXHR.status = 409
    failedXHR.responseText = JSON.stringify({error: '插件已存在'})
    failedXHR.onload()
    await expect(failed).rejects.toMatchObject({message: '插件已存在', status: 409})
  })

  it('deletes nexus plugins and builds download links', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({status: 'deleted'})})))

    await expect(deleteNexusPlugin('nexus', 'demo.plugin', '1.0.0')).resolves.toEqual({status: 'deleted'})
    expect(fetch.mock.calls[0][0]).toBe('/api/nexus/plugin?source=nexus&id=demo.plugin&version=1.0.0')
    expect(fetch.mock.calls[0][1].method).toBe('DELETE')
    expect(nexusDownloadURL('nexus', 'demo.plugin', '1.0.0')).toBe('/api/nexus/download?source=nexus&id=demo.plugin&version=1.0.0')
  })

  it('rebuilds the nexus index', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({data: {scanned: 1, indexed: 1}})})))

    await expect(rebuildNexusIndex('nexus')).resolves.toEqual({data: {scanned: 1, indexed: 1}})
    expect(fetch.mock.calls[0][0]).toBe('/api/nexus/rebuild?source=nexus')
    expect(fetch.mock.calls[0][1].method).toBe('POST')
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({})
  })
})
