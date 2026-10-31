// @vitest-environment jsdom
import {afterEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import RunHistoryRail from './RunHistoryRail.jsx'

const runs = [
  {id: 'run-2', kind: 'workflow', target: 'demo.flow', status: 'succeeded', started_at: '2026-10-30T12:00:00Z', ended_at: '2026-10-30T12:00:12Z'},
  {id: 'run-1', kind: 'workflow', target: 'demo.flow', status: 'failed', started_at: '2026-10-30T11:00:00Z', ended_at: '2026-10-30T11:00:04Z'}
]

let mounted = []

afterEach(() => {
  for (const {host, root} of mounted) {
    act(() => root.unmount())
    host.remove()
  }
  mounted = []
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function response(body) {
  return {ok: true, status: 200, json: async () => body, text: async () => ''}
}

async function mountRail({workflowID = 'demo.flow', items = runs, total = items.length, onSelectRun = vi.fn(), onOpenAllRuns = vi.fn(), activeRunID = '', activeRunStatus = '', selectionLocked = false, request} = {}) {
  const urls = []
  vi.stubGlobal('fetch', request || vi.fn(async path => {
    urls.push(path)
    return response({status: 'ok', data: {runs: items, total}})
  }))
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => {
    root.render(<RunHistoryRail workflowID={workflowID} activeRunID={activeRunID} activeRunStatus={activeRunStatus} selectionLocked={selectionLocked} onSelectRun={onSelectRun} onOpenAllRuns={onOpenAllRuns} />)
  })
  return {host, root, urls, onSelectRun, onOpenAllRuns}
}

describe('RunHistoryRail', () => {
  it('默认只列本工作流的运行记录', async () => {
    const {host, urls} = await mountRail()
    expect(urls[0]).toContain('/api/runs/?')
    expect(urls[0]).toContain('kind=workflow')
    expect(urls[0]).toContain(`target=${encodeURIComponent('demo.flow')}`)

    const items = Array.from(host.querySelectorAll('.workflowRunRailItem'))
    expect(items).toHaveLength(2)
    expect(items[0].textContent).toContain('成功')
    expect(items[0].textContent).toContain('12.0s')
    expect(items[1].textContent).toContain('失败')
  })

  it('左栏只列当前工作流：始终按 kind + target 精确过滤，没有范围切换', async () => {
    const {host, urls} = await mountRail()
    expect(host.querySelector('.workflowRunRailScope')).toBeNull()
    expect(urls[0]).toContain('kind=workflow')
    expect(urls[0]).toContain(`target=${encodeURIComponent('demo.flow')}`)
    // 反复刷新也只会拉本工作流的记录
    await act(async () => {
      Array.from(host.querySelectorAll('.workflowRunRailHeader button')).find(button => button.textContent === '刷新').dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    const last = urls[urls.length - 1]
    expect(last).toContain('kind=workflow')
    expect(last).toContain(`target=${encodeURIComponent('demo.flow')}`)
  })

  it('点击记录把运行 ID 抛给画布', async () => {
    const {host, onSelectRun} = await mountRail()
    await act(async () => {
      host.querySelectorAll('.workflowRunRailItem')[1].dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(onSelectRun).toHaveBeenCalledWith('run-1', expect.objectContaining({id: 'run-1'}))
  })

  it('高亮当前正在查看的运行', async () => {
    const {host} = await mountRail({activeRunID: 'run-1'})
    expect(host.querySelector('.workflowRunRailItem.active').textContent).toContain('失败')
  })

  it('选中已结束的运行时不会继续定时刷新', async () => {
    vi.useFakeTimers()
    const {urls} = await mountRail({activeRunID: 'run-1', activeRunStatus: 'failed'})
    expect(urls).toHaveLength(1)
    await act(async () => { await vi.advanceTimersByTimeAsync(9000) })
    expect(urls).toHaveLength(1)
  })

  it('新运行尚未出现在列表时继续刷新，结束后停止', async () => {
    vi.useFakeTimers()
    const {root, urls} = await mountRail({activeRunID: 'run-new', activeRunStatus: 'running'})
    await act(async () => { await vi.advanceTimersByTimeAsync(3000) })
    expect(urls).toHaveLength(2)
    await act(async () => {
      root.render(<RunHistoryRail workflowID="demo.flow" activeRunID="run-new" activeRunStatus="succeeded" />)
    })
    await act(async () => { await vi.advanceTimersByTimeAsync(9000) })
    expect(urls).toHaveLength(2)
  })

  it('切换工作流后丢弃旧列表的迟到响应', async () => {
    let resolveOld
    const oldResponse = new Promise(resolve => { resolveOld = resolve })
    const request = vi.fn(path => path.includes('target=old.flow')
      ? oldResponse
      : Promise.resolve(response({data: {runs: [{...runs[0], id: 'new-run', target: 'new.flow'}], total: 1}})))
    const {host, root} = await mountRail({workflowID: 'old.flow', request})
    await act(async () => {
      root.render(<RunHistoryRail workflowID="new.flow" />)
    })
    expect(host.querySelector('.workflowRunRailItem').textContent).toContain('new.flow')
    await act(async () => {
      resolveOld(response({data: {runs: [{...runs[1], target: 'old.flow'}], total: 20}}))
    })
    expect(host.querySelector('.workflowRunRailItem').textContent).toContain('new.flow')
    expect(host.querySelector('.workflowRunRailFooter').textContent).toContain('共 1 条')
  })

  it('运行期间保留当前入口并说明其他记录暂不能切换', async () => {
    const {host, onSelectRun} = await mountRail({activeRunID: 'run-1', activeRunStatus: 'running', selectionLocked: true})
    const buttons = host.querySelectorAll('.workflowRunRailItem')
    expect(buttons[0].disabled).toBe(true)
    expect(buttons[0].title).toContain('完成或取消后')
    expect(buttons[1].disabled).toBe(false)
    await act(async () => { buttons[0].click() })
    expect(onSelectRun).not.toHaveBeenCalled()
  })

  it('没有记录时给出下一步提示', async () => {
    const {host} = await mountRail({items: [], total: 0})
    expect(host.querySelector('.workflowRunRailList').textContent).toContain('还没有运行记录')
  })

  it('完整运行记录入口回调给上层切页', async () => {
    const {host, onOpenAllRuns} = await mountRail()
    await act(async () => {
      Array.from(host.querySelectorAll('.workflowRunRailFooter button')).find(button => button.textContent === '完整运行记录').dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(onOpenAllRuns).toHaveBeenCalled()
  })
})
