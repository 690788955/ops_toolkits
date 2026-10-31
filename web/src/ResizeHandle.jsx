import React, {useCallback, useEffect, useRef, useState} from 'react'

// 分栏宽度是“用户自定义”的东西：拖出来的值记在 localStorage，刷新、切页、换一条运行后都还在。
// 没拖过就返回 null，让样式里的默认值（响应式 clamp）继续生效，不会把默认布局写死。
export const RESIZE_STEP = 16
const PERSIST_DELAY_MS = 240

export function clampWidth(value, min, max) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return Number.isFinite(min) ? min : 0
  const lower = Number.isFinite(min) ? min : parsed
  const upper = Number.isFinite(max) ? Math.max(lower, max) : parsed
  return Math.round(Math.min(Math.max(parsed, lower), upper))
}

function storage() {
  try {
    if (typeof window === 'undefined' || !window.localStorage) return null
    return window.localStorage
  } catch (err) {
    return null
  }
}

export function readStoredWidth(key, min, max) {
  const store = storage()
  if (!store || !key) return null
  try {
    const raw = store.getItem(key)
    if (raw === null || raw === undefined || raw === '') return null
    const parsed = Number(raw)
    if (!Number.isFinite(parsed)) return null
    return clampWidth(parsed, min, max)
  } catch (err) {
    return null
  }
}

export function writeStoredWidth(key, width) {
  const store = storage()
  if (!store || !key) return false
  try {
    store.setItem(key, String(width))
    return true
  } catch (err) {
    return false
  }
}

export function clearStoredWidth(key) {
  const store = storage()
  if (!store || !key) return false
  try {
    store.removeItem(key)
    return true
  } catch (err) {
    return false
  }
}

// 一个可拖动分栏的宽度状态：setWidth 立即生效，落盘做防抖，避免拖动时每帧都写 localStorage。
export function useResizableWidth({storageKey, min, max}) {
  const [width, setWidthState] = useState(() => readStoredWidth(storageKey, min, max))
  const timerRef = useRef(0)

  const setWidth = useCallback(next => {
    const clamped = clampWidth(next, min, max)
    setWidthState(clamped)
    if (timerRef.current) window.clearTimeout(timerRef.current)
    timerRef.current = window.setTimeout(() => {
      timerRef.current = 0
      writeStoredWidth(storageKey, clamped)
    }, PERSIST_DELAY_MS)
    return clamped
  }, [storageKey, min, max])

  const reset = useCallback(() => {
    if (timerRef.current) {
      window.clearTimeout(timerRef.current)
      timerRef.current = 0
    }
    setWidthState(null)
    clearStoredWidth(storageKey)
  }, [storageKey])

  useEffect(() => () => {
    if (timerRef.current) window.clearTimeout(timerRef.current)
  }, [])

  return {width, setWidth, reset}
}

// 竖分隔条：透明地盖在两栏之间的间隙上，拖动改宽度；双击回到默认，方向键微调（Shift 加速）。
// invert=true 表示被调整的是分隔条右边的栏，往右拖反而变窄。
export default function ResizeHandle({
  label = '调整栏宽',
  invert = false,
  min = 160,
  max = 900,
  step = RESIZE_STEP,
  value = null,
  fallback = 320,
  measure,
  onChange,
  onReset,
  disabled = false
}) {
  const [dragging, setDragging] = useState(false)
  const dragRef = useRef(null)

  // 拖动时锁住整页的文本选择和光标，否则鼠标一快就会选中旁边的文字。
  useEffect(() => {
    if (!dragging || typeof document === 'undefined') return undefined
    const body = document.body
    const previousCursor = body.style.cursor
    const previousSelect = body.style.userSelect
    body.style.cursor = 'col-resize'
    body.style.userSelect = 'none'
    return () => {
      body.style.cursor = previousCursor
      body.style.userSelect = previousSelect
    }
  }, [dragging])

  const currentWidth = useCallback(() => {
    const measured = typeof measure === 'function' ? Number(measure()) : 0
    if (Number.isFinite(measured) && measured > 0) return measured
    return Number.isFinite(Number(value)) && Number(value) > 0 ? Number(value) : fallback
  }, [measure, value, fallback])

  const applyWidth = useCallback(next => {
    const clamped = clampWidth(next, min, max)
    onChange?.(clamped)
    return clamped
  }, [onChange, min, max])

  function handlePointerDown(event) {
    if (disabled) return
    if (typeof event.button === 'number' && event.button !== 0) return
    event.preventDefault()
    dragRef.current = {startX: event.clientX, startWidth: currentWidth()}
    setDragging(true)
    try { event.currentTarget.setPointerCapture?.(event.pointerId) } catch (err) { /* jsdom 没有指针捕获 */ }
  }

  function handlePointerMove(event) {
    const drag = dragRef.current
    if (!drag) return
    const delta = (event.clientX - drag.startX) * (invert ? -1 : 1)
    applyWidth(drag.startWidth + delta)
  }

  function endDrag(event) {
    if (!dragRef.current) return
    dragRef.current = null
    setDragging(false)
    try { event.currentTarget.releasePointerCapture?.(event.pointerId) } catch (err) { /* jsdom 没有指针捕获 */ }
  }

  function handleKeyDown(event) {
    if (disabled) return
    const direction = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0
    if (!direction) return
    event.preventDefault()
    const distance = (event.shiftKey ? step * 4 : step) * direction * (invert ? -1 : 1)
    applyWidth(currentWidth() + distance)
  }

  const ariaValue = Number.isFinite(Number(value)) && Number(value) > 0 ? Math.round(Number(value)) : Math.round(currentWidth())

  return (
    <div
      className="resizeHandle"
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={ariaValue}
      aria-disabled={disabled || undefined}
      tabIndex={disabled ? -1 : 0}
      data-dragging={dragging ? 'true' : 'false'}
      title={`${label}：拖动调整宽度，双击恢复默认，方向键微调`}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onDoubleClick={event => {
        event.preventDefault()
        onReset?.()
      }}
      onKeyDown={handleKeyDown}
    >
      <span className="resizeHandleGrip" aria-hidden="true" />
    </div>
  )
}
