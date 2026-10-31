// @vitest-environment jsdom
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import ResizeHandle, {clampWidth, readStoredWidth, useResizableWidth} from './ResizeHandle.jsx'

let mounted = []

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  for (const {host, root} of mounted) {
    act(() => root.unmount())
    host.remove()
  }
  mounted = []
  vi.useRealTimers()
  window.localStorage.clear()
})

async function mount(element) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => { root.render(element) })
  return host
}

function fire(element, type, init = {}) {
  return act(async () => {
    element.dispatchEvent(new MouseEvent(type, {bubbles: true, cancelable: true, ...init}))
  })
}

function press(element, key, init = {}) {
  return act(async () => {
    element.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...init}))
  })
}

describe('clampWidth', () => {
  it('把宽度夹在上下限之间，非法值退回下限', () => {
    expect(clampWidth(320, 180, 460)).toBe(320)
    expect(clampWidth(10, 180, 460)).toBe(180)
    expect(clampWidth(9999, 180, 460)).toBe(460)
    expect(clampWidth('not-a-number', 180, 460)).toBe(180)
    expect(clampWidth(300.6, 180, 460)).toBe(301)
  })
})

describe('readStoredWidth', () => {
  it('没存过或存的值不合法时返回 null，让默认宽度继续生效', () => {
    expect(readStoredWidth('ops.test.width', 180, 460)).toBeNull()
    window.localStorage.setItem('ops.test.width', 'abc')
    expect(readStoredWidth('ops.test.width', 180, 460)).toBeNull()
    window.localStorage.setItem('ops.test.width', '300')
    expect(readStoredWidth('ops.test.width', 180, 460)).toBe(300)
    window.localStorage.setItem('ops.test.width', '9999')
    expect(readStoredWidth('ops.test.width', 180, 460)).toBe(460)
  })
})

describe('ResizeHandle 拖动', () => {
  it('左右拖动按像素改宽度，并受上下限约束', async () => {
    const onChange = vi.fn()
    const host = await mount(<ResizeHandle label="调整栏宽" measure={() => 300} onChange={onChange} min={180} max={500} />)
    const handle = host.querySelector('.resizeHandle')

    await fire(handle, 'pointerdown', {clientX: 100, button: 0})
    await fire(handle, 'pointermove', {clientX: 160})
    expect(onChange).toHaveBeenLastCalledWith(360)

    await fire(handle, 'pointermove', {clientX: 2000})
    expect(onChange).toHaveBeenLastCalledWith(500)

    await fire(handle, 'pointermove', {clientX: -2000})
    expect(onChange).toHaveBeenLastCalledWith(180)
  })

  it('invert 用于右侧栏：往右拖是变窄', async () => {
    const onChange = vi.fn()
    const host = await mount(<ResizeHandle label="调整栏宽" invert measure={() => 400} onChange={onChange} min={320} max={820} />)
    const handle = host.querySelector('.resizeHandle')

    await fire(handle, 'pointerdown', {clientX: 600, button: 0})
    await fire(handle, 'pointermove', {clientX: 540})
    expect(onChange).toHaveBeenLastCalledWith(460)
  })

  it('没量到真实宽度时用 value / fallback 当起点', async () => {
    const onChange = vi.fn()
    const host = await mount(<ResizeHandle label="调整栏宽" value={null} fallback={240} measure={() => 0} onChange={onChange} min={180} max={500} />)
    const handle = host.querySelector('.resizeHandle')

    await fire(handle, 'pointerdown', {clientX: 100, button: 0})
    await fire(handle, 'pointermove', {clientX: 130})
    expect(onChange).toHaveBeenLastCalledWith(270)
  })

  it('拖动中不再响应非左键按下，松开后停止跟随', async () => {
    const onChange = vi.fn()
    const host = await mount(<ResizeHandle label="调整栏宽" measure={() => 300} onChange={onChange} />)
    const handle = host.querySelector('.resizeHandle')

    await fire(handle, 'pointerdown', {clientX: 100, button: 2})
    await fire(handle, 'pointermove', {clientX: 200})
    expect(onChange).not.toHaveBeenCalled()

    await fire(handle, 'pointerdown', {clientX: 100, button: 0})
    await fire(handle, 'pointerup', {clientX: 100})
    await fire(handle, 'pointermove', {clientX: 200})
    expect(onChange).toHaveBeenCalledTimes(0)
  })
})

describe('ResizeHandle 键盘与复位', () => {
  it('方向键微调，Shift 加速', async () => {
    const onChange = vi.fn()
    const host = await mount(<ResizeHandle label="调整栏宽" measure={() => 300} onChange={onChange} />)
    const handle = host.querySelector('.resizeHandle')

    await press(handle, 'ArrowRight')
    expect(onChange).toHaveBeenLastCalledWith(316)
    await press(handle, 'ArrowLeft', {shiftKey: true})
    expect(onChange).toHaveBeenLastCalledWith(236)
  })

  it('双击回到默认宽度', async () => {
    const onReset = vi.fn()
    const host = await mount(<ResizeHandle label="调整栏宽" measure={() => 300} onChange={vi.fn()} onReset={onReset} />)
    await fire(host.querySelector('.resizeHandle'), 'dblclick')
    expect(onReset).toHaveBeenCalledTimes(1)
  })

  it('带 aria 语义，键盘可聚焦', async () => {
    const host = await mount(<ResizeHandle label="调整运行记录栏宽度" value={320} measure={() => 0} onChange={vi.fn()} min={180} max={460} />)
    const handle = host.querySelector('.resizeHandle')
    expect(handle.getAttribute('role')).toBe('separator')
    expect(handle.getAttribute('aria-orientation')).toBe('vertical')
    expect(handle.getAttribute('aria-label')).toBe('调整运行记录栏宽度')
    expect(handle.getAttribute('aria-valuenow')).toBe('320')
    expect(handle.getAttribute('aria-valuemin')).toBe('180')
    expect(handle.getAttribute('aria-valuemax')).toBe('460')
    expect(handle.tabIndex).toBe(0)
  })
})

function WidthHarness({storageKey}) {
  const {width, setWidth, reset} = useResizableWidth({storageKey, min: 180, max: 460})
  return (
    <div>
      <span className="current">{width === null ? 'default' : width}</span>
      <button type="button" onClick={() => setWidth(320)}>设宽度</button>
      <button type="button" onClick={() => setWidth(9999)}>超上限</button>
      <button type="button" onClick={reset}>复位</button>
    </div>
  )
}

describe('useResizableWidth', () => {
  it('初始沿用默认值，设置后防抖写入 localStorage，复位后清空', async () => {
    vi.useFakeTimers()
    const host = await mount(<WidthHarness storageKey="ops.test.railWidth" />)
    expect(host.querySelector('.current').textContent).toBe('default')

    await act(async () => {
      host.querySelectorAll('button')[1].dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(host.querySelector('.current').textContent).toBe('460')
    await act(async () => { vi.advanceTimersByTime(300) })
    expect(window.localStorage.getItem('ops.test.railWidth')).toBe('460')

    await act(async () => {
      host.querySelectorAll('button')[2].dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(host.querySelector('.current').textContent).toBe('default')
    expect(window.localStorage.getItem('ops.test.railWidth')).toBeNull()
  })

  it('重新挂载时读回存过的宽度', async () => {
    window.localStorage.setItem('ops.test.railWidth', '360')
    const host = await mount(<WidthHarness storageKey="ops.test.railWidth" />)
    expect(host.querySelector('.current').textContent).toBe('360')
  })
})
