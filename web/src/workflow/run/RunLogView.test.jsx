// @vitest-environment jsdom
import {afterEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import RunLogView from './RunLogView.jsx'

let mounted = []

afterEach(() => {
  for (const {host, root} of mounted) {
    act(() => root.unmount())
    host.remove()
  }
  mounted = []
  vi.unstubAllGlobals()
})

const items = [
  {id: 'first', title: '第一步', status: 'succeeded', stdout: 'first-output'},
  {id: 'second', title: '第二步', status: 'running', stdout: 'ERROR 第二步失败', stderr: 'warning 重试中'}
]

async function mount(props) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => { root.render(<RunLogView {...props} />) })
  return host
}

function click(element) {
  return act(async () => { element.dispatchEvent(new MouseEvent('click', {bubbles: true})) })
}

describe('RunLogView 范围', () => {
  it('整份运行日志按节点分组，并高亮正在执行的节点', async () => {
    const host = await mount({items, nodeNames: {first: '第一步', second: '第二步'}, runningOwnerID: 'second', scopeToggle: false})
    expect(host.querySelectorAll('.runLogOwner')).toHaveLength(2)
    expect(host.querySelector('.runLogOwner.focus .runLogOwnerName').textContent).toBe('第二步')
    expect(host.querySelectorAll('.runLogLine.focus')).toHaveLength(2)
    // 取消节点范围时不再有「只看此节点」，避免出现没有意义的开关。
    expect(host.querySelector('.runLogToolbar .terminalToggle input')).toBeTruthy()
    expect(host.textContent).not.toContain('只看此节点')
  })

  it('节点范围默认只看该节点，取消后回到全局流', async () => {
    const host = await mount({items, nodeID: 'second', nodeNames: {first: '第一步', second: '第二步'}, scopeToggle: true})
    const texts = () => Array.from(host.querySelectorAll('.runLogLine')).map(line => line.textContent)
    expect(texts()).toEqual(['ERROR 第二步失败', 'warning 重试中'])

    const scopeToggle = host.querySelector('.runLogToolbar .terminalToggle input')
    await click(scopeToggle)
    expect(texts()).toEqual(['first-output', 'ERROR 第二步失败', 'warning 重试中'])
  })
})

describe('RunLogView 过滤与导出', () => {
  it('级别 chips 与「只看错误」按行级别过滤', async () => {
    const host = await mount({items, scopeToggle: false})
    expect(Array.from(host.querySelectorAll('.logFilterTags .tagChip')).map(chip => chip.textContent)).toEqual(['全部', 'error 1', 'warning 1', 'log 1'])

    await click(Array.from(host.querySelectorAll('button')).find(button => button.textContent === '只看错误'))
    expect(Array.from(host.querySelectorAll('.runLogLine')).map(line => line.textContent)).toEqual(['ERROR 第二步失败', 'warning 重试中'])
  })

  it('导出当前视图的日志行', async () => {
    const create = vi.fn(() => 'blob:fake')
    const revoke = vi.fn()
    const originalCreate = URL.createObjectURL
    const originalRevoke = URL.revokeObjectURL
    URL.createObjectURL = create
    URL.revokeObjectURL = revoke
    try {
      const host = await mount({items, nodeID: 'second', runID: 'run-77', scopeToggle: true})
      await click(Array.from(host.querySelectorAll('button')).find(button => button.textContent === '导出'))
      expect(create).toHaveBeenCalledTimes(1)
      expect(revoke).toHaveBeenCalledWith('blob:fake')
    } finally {
      URL.createObjectURL = originalCreate
      URL.revokeObjectURL = originalRevoke
    }
  })

  it('超过行数上限时提示被截断', async () => {
    const host = await mount({items, scopeToggle: false, lineLimit: 2})
    expect(host.querySelector('.runLogTruncated')).toBeTruthy()
    expect(host.querySelectorAll('.runLogLine')).toHaveLength(2)
  })

  it('搜索只留下匹配行', async () => {
    const host = await mount({items, scopeToggle: false})
    const search = host.querySelector('.runLogToolbar input:not([type="checkbox"])')
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
    await act(async () => {
      setter.call(search, 'first-output')
      search.dispatchEvent(new Event('input', {bubbles: true}))
    })
    expect(Array.from(host.querySelectorAll('.runLogLine')).map(line => line.textContent)).toEqual(['first-output'])
    expect(host.querySelector('.runLogCount').textContent).toBe('1 / 3')
  })
})

async function typeSearch(host, value) {
  const search = host.querySelector('.runLogToolbar input:not([type="checkbox"])')
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
  await act(async () => {
    setter.call(search, value)
    search.dispatchEvent(new Event('input', {bubbles: true}))
  })
  return search
}

const repeatedItems = [
  {id: 'only', title: '唯一节点', status: 'succeeded', stdout: 'hit one\nmiss\nhit two\nhit three'}
]

describe('RunLogView 搜索命中', () => {
  it('命中的词就地高亮，并显示第几处', async () => {
    const host = await mount({items: repeatedItems, scopeToggle: false})
    await typeSearch(host, 'hit')
    expect(host.querySelectorAll('.runLogHit')).toHaveLength(3)
    expect(Array.from(host.querySelectorAll('.runLogHit')).map(mark => mark.textContent)).toEqual(['hit', 'hit', 'hit'])
    expect(host.querySelector('.runLogHitPosition').textContent).toBe('1 / 3')
    expect(host.querySelectorAll('.runLogLine.current')).toHaveLength(1)
    expect(host.querySelector('.runLogLine.current').textContent).toBe('hit one')
  })

  it('上下按钮和回车在命中之间跳转，清除按钮收掉搜索', async () => {
    const host = await mount({items: repeatedItems, scopeToggle: false})
    const search = await typeSearch(host, 'hit')
    const next = host.querySelector('[aria-label="下一个匹配"]')
    const prev = host.querySelector('[aria-label="上一个匹配"]')

    await click(next)
    expect(host.querySelector('.runLogHitPosition').textContent).toBe('2 / 3')
    expect(host.querySelector('.runLogLine.current').textContent).toBe('hit two')

    await act(async () => {
      search.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true}))
    })
    expect(host.querySelector('.runLogHitPosition').textContent).toBe('3 / 3')

    await click(prev)
    expect(host.querySelector('.runLogHitPosition').textContent).toBe('2 / 3')

    await click(Array.from(host.querySelectorAll('button')).find(button => button.textContent === '清除'))
    expect(host.querySelector('.runLogHitPosition')).toBeNull()
    expect(host.querySelectorAll('.runLogHit')).toHaveLength(0)
    expect(host.querySelectorAll('.runLogLine')).toHaveLength(4)
  })

  it('没有命中时给出明确提示而不是空列表', async () => {
    const host = await mount({items: repeatedItems, scopeToggle: false})
    await typeSearch(host, '根本不存在的词')
    expect(host.querySelector('.runLogHitPosition').textContent).toBe('无匹配')
    expect(host.querySelector('.runLogBox').textContent).toContain('没有匹配的日志行')
  })
})

describe('RunLogView 阅读体验', () => {
  it('按级别给日志行打标记，错误和警告各有颜色类', async () => {
    const host = await mount({items, scopeToggle: false})
    const errorLine = Array.from(host.querySelectorAll('.runLogLine')).find(line => line.textContent === 'ERROR 第二步失败')
    const warningLine = Array.from(host.querySelectorAll('.runLogLine')).find(line => line.textContent === 'warning 重试中')
    expect(errorLine.className).toContain('level-error')
    expect(errorLine.dataset.level).toBe('error')
    expect(warningLine.className).toContain('level-warning')
  })

  it('分组标题带行数，一眼看出哪个节点输出最多', async () => {
    const host = await mount({items, nodeNames: {first: '第一步', second: '第二步'}, scopeToggle: false})
    expect(Array.from(host.querySelectorAll('.runLogOwnerMeta')).map(meta => meta.textContent)).toEqual(['1 行', '2 行'])
  })

  it('自动换行开关切换日志容器的排版方式', async () => {
    const host = await mount({items, scopeToggle: false})
    expect(host.querySelector('.runLogBox').dataset.wrap).toBe('on')
    const toggle = Array.from(host.querySelectorAll('.runLogToolbar .terminalToggle'))
      .find(label => label.textContent.includes('自动换行'))
    await click(toggle.querySelector('input'))
    expect(host.querySelector('.runLogBox').dataset.wrap).toBe('off')
  })
})