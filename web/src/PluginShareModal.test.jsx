// @vitest-environment jsdom

import React from 'react'
import {act} from 'react'
import {createRoot} from 'react-dom/client'
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import PluginShareModal from './PluginShareModal.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

function jsonResponse(body, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Conflict',
    headers: {get: () => 'application/json'},
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body))
  })
}

const plugin = {id: 'vendor.demo', name: '演示插件', version: '1.2.0', description: '用于测试分享的插件'}
const targets = [{id: 'nexus', name: 'Nexus 插件仓库', url: 'http://nexus.example.com', repository: 'plugins', index_url: 'http://nexus.example.com/repository/plugins/index.json'}]

async function renderModal(props) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(<PluginShareModal {...props} />)
  })
  return {host, root}
}

function findButton(host, label) {
  return [...host.querySelectorAll('button')].find(button => button.textContent.trim() === label)
}

describe('PluginShareModal', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(() => jsonResponse({
      status: 'shared',
      data: {
        source_id: 'nexus',
        source_name: 'Nexus 插件仓库',
        plugin_id: 'vendor.demo',
        version: '1.2.0',
        package_path: 'packages/vendor.demo-1.2.0.zip',
        sha256: 'a'.repeat(64),
        size: 2048,
        index_updated: true
      }
    })))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('shares the installed plugin to the selected remote market', async () => {
    const onShared = vi.fn()
    const {host} = await renderModal({plugin, targets, onClose: () => {}, onShared})
    const shareButton = findButton(host, '分享到远程市场')
    expect(shareButton).toBeTruthy()
    expect(shareButton.disabled).toBe(false)

    await act(async () => {
      shareButton.dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })

    const [url, options] = globalThis.fetch.mock.calls[0]
    expect(url).toBe('/api/plugins/vendor.demo/share?source=nexus')
    expect(options.method).toBe('POST')
    expect(onShared).toHaveBeenCalledTimes(1)
    expect(host.textContent).toContain('packages/vendor.demo-1.2.0.zip')
    expect(host.textContent).toContain('Nexus 插件仓库')
  })

  it('disables the share action when no remote market is configured', async () => {
    const {host} = await renderModal({plugin, targets: [], onClose: () => {}})
    const shareButton = findButton(host, '分享到远程市场')
    expect(shareButton).toBeTruthy()
    expect(shareButton.disabled).toBe(true)
    expect(host.textContent).toContain('未配置可用的远程插件市场')
  })

  it('asks for overwrite confirmation when the version already exists', async () => {
    globalThis.fetch = vi.fn(() => jsonResponse({
      error: 'Nexus 仓库中已存在插件 vendor.demo@1.2.0，是否覆盖？',
      data: {existing: true}
    }, 409))
    const {host} = await renderModal({plugin, targets, onClose: () => {}})

    await act(async () => {
      findButton(host, '分享到远程市场').dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })

    expect(host.textContent).toContain('已存在')
    const overwrite = findButton(host, '覆盖并分享')
    expect(overwrite).toBeTruthy()
    expect(overwrite.disabled).toBe(false)
  })
})
