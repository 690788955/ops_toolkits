// @vitest-environment jsdom

import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {
  DRAFT_WINDOW_SAVED_KEY,
  DRAFT_WINDOW_TRANSFER_KEY,
  clearDraftWindowSaved,
  clearDraftWindowTransfer,
  draftWindowSavedRefValue,
  draftWindowURL,
  formatDraftWindowTime,
  isDraftWindowLocation,
  readDraftWindowSaved,
  readDraftWindowTransfer,
  writeDraftWindowSaved,
  writeDraftWindowTransfer
} from './draftWindow.js'

beforeEach(() => {
  window.localStorage?.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('draftWindow 位置参数', () => {
  it('只把 draftWindow=1 当作独立草稿窗口', () => {
    expect(isDraftWindowLocation('?draftWindow=1')).toBe(true)
    expect(isDraftWindowLocation('?token=abc&draftWindow=1')).toBe(true)
    expect(isDraftWindowLocation('?draftWindow=0')).toBe(false)
    expect(isDraftWindowLocation('?token=abc')).toBe(false)
    expect(isDraftWindowLocation('')).toBe(false)
  })

  it('构造窗口地址时保留 token 并去掉主控制台的 hash', () => {
    const url = draftWindowURL('http://127.0.0.1:5173/?token=abc#workflows')
    const parsed = new URL(url)
    expect(parsed.origin).toBe('http://127.0.0.1:5173')
    expect(parsed.pathname).toBe('/')
    expect(parsed.searchParams.get('token')).toBe('abc')
    expect(parsed.searchParams.get('draftWindow')).toBe('1')
    expect(parsed.hash).toBe('')
  })
})

describe('draftWindow 草稿交接', () => {
  const draft = {id: 'demo.flow', name: '演示流程', nodes: [{id: 'first', tool: 'demo.hello'}], edges: []}

  it('写入并读回主窗口草稿，缺草稿内容时拒绝写入', () => {
    expect(writeDraftWindowTransfer({id: 'demo.flow', name: '演示流程', draft, dirty: true})).toBe(true)
    const transfer = readDraftWindowTransfer()
    expect(transfer.id).toBe('demo.flow')
    expect(transfer.name).toBe('演示流程')
    expect(transfer.dirty).toBe(true)
    expect(transfer.draft.nodes).toHaveLength(1)

    expect(writeDraftWindowTransfer({id: 'demo.flow'})).toBe(false)
    clearDraftWindowTransfer()
    expect(readDraftWindowTransfer()).toBeNull()
  })

  it('损坏的交接数据按不存在处理', () => {
    window.localStorage.setItem(DRAFT_WINDOW_TRANSFER_KEY, '{oops')
    expect(readDraftWindowTransfer()).toBeNull()
  })

  it('存储不可用时读写都返回失败而不是抛出', () => {
    // 浏览器禁用本地存储时访问 localStorage 本身就会抛错，读写都要兜住。
    vi.spyOn(window, 'localStorage', 'get').mockImplementation(() => { throw new Error('storage blocked') })
    expect(writeDraftWindowTransfer({draft})).toBe(false)
    expect(writeDraftWindowSaved({id: 'demo.flow'})).toBe(false)
    expect(readDraftWindowTransfer()).toBeNull()
    expect(readDraftWindowSaved()).toBeNull()
    expect(() => clearDraftWindowTransfer()).not.toThrow()
  })
})

describe('draftWindow 保存标记', () => {
  it('记录并读回草稿窗口的保存结果', () => {
    expect(writeDraftWindowSaved({id: 'demo.flow', name: '演示流程', ts: 1700000000000})).toBe(true)
    expect(readDraftWindowSaved()).toEqual({id: 'demo.flow', name: '演示流程', ts: 1700000000000})
    expect(draftWindowSavedRefValue(readDraftWindowSaved())).toBe('demo.flow@1700000000000')
    clearDraftWindowSaved()
    expect(readDraftWindowSaved()).toBeNull()
    expect(draftWindowSavedRefValue(null)).toBe('')
  })

  it('缺少工作流 ID 时不写入标记', () => {
    expect(writeDraftWindowSaved({name: '演示流程'})).toBe(false)
    expect(window.localStorage.getItem(DRAFT_WINDOW_SAVED_KEY)).toBeNull()
  })

  it('格式化带入时间，非法值返回空串', () => {
    expect(formatDraftWindowTime(0)).toBe('')
    expect(formatDraftWindowTime('nope')).toBe('')
    expect(formatDraftWindowTime(new Date(2026, 0, 2, 3, 4).getTime())).toBe('01-02 03:04')
  })
})