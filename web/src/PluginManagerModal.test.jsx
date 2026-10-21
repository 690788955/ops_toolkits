// @vitest-environment jsdom

import React from 'react'
import {act} from 'react'
import {createRoot} from 'react-dom/client'
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {PluginManagerModal} from './main.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

vi.mock('@xyflow/react', () => ({
  Background: () => <div />,
  Controls: () => <div />,
  Handle: () => <span />,
  MiniMap: () => <div />,
  Position: {Top: 'top', Right: 'right', Bottom: 'bottom', Left: 'left'},
  ReactFlow: () => <div className="react-flow" />
}))

const catalog = {
  plugins: [{id: 'vendor.demo', name: '演示插件', version: '1.2.0', description: '测试分享入口', disabled: false}],
  workflows: [],
  categories: []
}

function jsonResponse(body) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: 'OK',
    headers: {get: () => 'application/json'},
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body))
  })
}

async function renderManager(overrideCatalog = catalog) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      <PluginManagerModal
        catalog={overrideCatalog}
        state={{message: ''}}
        setState={() => {}}
        onClose={() => {}}
        onUploaded={() => {}}
        onChanged={() => {}}
      />
    )
  })
  return {host, root}
}

function typeInto(input, value) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
  setter.call(input, value)
  input.dispatchEvent(new Event('input', {bubbles: true}))
}

function shareButton(host) {
  const card = host.querySelector('.pluginExportItem')
  return [...(card?.querySelectorAll('button') || [])].find(button => button.textContent.trim() === '分享')
}

describe('PluginManagerModal share entry', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('disables the share button when only the local market exists', async () => {
    vi.stubGlobal('fetch', vi.fn(url => {
      if (String(url) === '/api/nexus/share/targets') {
        return jsonResponse({data: {targets: [], message: '未配置可用的远程插件市场（Nexus），只能使用本地插件市场。'}})
      }
      return jsonResponse({data: {}})
    }))
    const {host} = await renderManager()
    await vi.waitFor(() => expect(shareButton(host)).toBeTruthy())

    const button = shareButton(host)
    expect(button.disabled).toBe(true)
    expect(button.getAttribute('title')).toContain('未配置')
    expect(host.textContent).toContain('未配置可用的远程插件市场')
  })

  it('enables the share button and opens the share dialog when a remote market exists', async () => {
    vi.stubGlobal('fetch', vi.fn(url => {
      if (String(url) === '/api/nexus/share/targets') {
        return jsonResponse({data: {targets: [{id: 'nexus', name: 'Nexus 插件仓库', repository: 'plugins'}]}})
      }
      return jsonResponse({data: {}})
    }))
    const {host} = await renderManager()
    await vi.waitFor(() => {
      expect(shareButton(host)?.disabled).toBe(false)
    })

    await act(async () => {
      shareButton(host).dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })

    expect(host.textContent).toContain('分享插件')
    expect(host.textContent).toContain('Nexus 插件仓库（plugins）')
  })
})

describe('PluginManagerModal installed search', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  const installedCatalog = {
    plugins: [
      {id: 'vendor.alpha', name: '数据同步', version: '1.0.0', description: '同步数据到远端', disabled: false},
      {id: 'vendor.beta', name: '日志采集', version: '2.0.0', description: '采集主机日志', disabled: false}
    ],
    workflows: [],
    categories: []
  }

  it('filters installed plugins by keyword and restores them when cleared', async () => {
    vi.stubGlobal('fetch', vi.fn(() => jsonResponse({data: {targets: []}})))
    const {host} = await renderManager(installedCatalog)
    await vi.waitFor(() => expect(host.querySelectorAll('.pluginExportItem').length).toBe(2))

    const input = host.querySelector('.pluginInstalledToolbar input')
    expect(input).toBeTruthy()
    expect(input.getAttribute('placeholder')).toContain('搜索')

    await act(async () => { typeInto(input, '日志') })
    expect(host.querySelectorAll('.pluginExportItem').length).toBe(1)
    expect(host.textContent).toContain('日志采集')
    expect(host.textContent).not.toContain('数据同步')

    await act(async () => { typeInto(input, '不存在') })
    expect(host.querySelectorAll('.pluginExportItem').length).toBe(0)
    expect(host.textContent).toContain('没有匹配的插件。')

    await act(async () => { typeInto(input, '') })
    expect(host.querySelectorAll('.pluginExportItem').length).toBe(2)
  })
})

describe('PluginManagerModal download menu', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('collapses both zip downloads into one menu and closes on outside click', async () => {
    vi.stubGlobal('fetch', vi.fn(() => jsonResponse({data: {targets: [{id: 'nexus', name: 'Nexus 插件仓库'}]}})))
    const {host} = await renderManager()
    await vi.waitFor(() => expect(host.querySelector('.pluginExportItem')).toBeTruthy())

    // 旧的两个独立按钮已合并
    expect(host.querySelectorAll('.pluginExportLinks a').length).toBe(0)
    const toggle = [...host.querySelectorAll('.pluginItemActions button')].find(button => button.textContent.trim() === '下载')
    expect(toggle).toBeTruthy()
    expect(toggle.getAttribute('aria-haspopup')).toBe('menu')

    await act(async () => {
      toggle.dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })

    const menu = document.querySelector('.pluginDownloadPopover')
    expect(menu).toBeTruthy()
    const pluginLink = [...menu.querySelectorAll('a')].find(link => link.textContent.includes('插件 ZIP'))
    expect(pluginLink).toBeTruthy()
    expect(pluginLink.getAttribute('href')).toBe('/api/plugins/vendor.demo.zip')
    // 一级菜单只有类型选择，运行包需要再选平台
    expect(menu.querySelectorAll('.pluginDownloadPlatform').length).toBe(0)
    const next = [...menu.querySelectorAll('button')].find(button => button.textContent.includes('运行包 ZIP'))
    expect(next).toBeTruthy()

    // 二级：选择运行包平台
    await act(async () => {
      next.dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    const platforms = [...document.querySelectorAll('.pluginDownloadPopover .pluginDownloadPlatform')]
    expect(platforms).toHaveLength(6)
    const windows = platforms.find(link => link.textContent.includes('Windows amd64'))
    expect(windows.getAttribute('href')).toBe('/api/plugins/vendor.demo/runtime.zip?goos=windows&goarch=amd64')
    expect(windows.className).toContain('active')
    expect(windows.textContent).toContain('上次下载')

    // 返回一级
    await act(async () => {
      document.querySelector('.pluginDownloadBack').dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(document.querySelectorAll('.pluginDownloadPlatform').length).toBe(0)
    expect(document.querySelector('.pluginDownloadNext')).toBeTruthy()

    // 再次点击按钮收起
    await act(async () => {
      toggle.dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(document.querySelector('.pluginDownloadPopover')).toBeNull()

    // 打开后点击外部关闭
    await act(async () => {
      toggle.dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(document.querySelector('.pluginDownloadPopover')).toBeTruthy()
    await act(async () => {
      document.body.dispatchEvent(new MouseEvent('mousedown', {bubbles: true}))
    })
    expect(document.querySelector('.pluginDownloadPopover')).toBeNull()
  })
})
