import React, {useEffect, useRef} from 'react'
import {useModalEscape} from '../../useModalEscape.js'
import RunLogPanel from './RunLogPanel.jsx'

// 保留同一份面板实例，放大/还原不会清空搜索、筛选和恢复操作。
export default function RunLogWindow({open = false, embedded = false, onClose, onExpand, onCollapse, title = '运行日志', ...panelProps}) {
  const containerRef = useRef(null)
  useModalEscape(() => onClose?.(), containerRef, open)
  useEffect(() => {
    if (!open) return undefined
    const previousFocus = document.activeElement
    containerRef.current?.querySelector('button')?.focus()
    return () => { if (previousFocus?.isConnected) previousFocus.focus?.() }
  }, [open])

  function trapFocus(event) {
    if (!open || event.key !== 'Tab') return
    const elements = Array.from(containerRef.current?.querySelectorAll('button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]') || [])
      .filter(element => element.getClientRects().length > 0)
    const first = elements[0]
    const last = elements[elements.length - 1]
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last?.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first?.focus()
    }
  }

  if (!open && !embedded) return null
  return (
    <div
      className={open ? 'workflowRunPanelHost workflowLogWindowBackdrop' : 'workflowRunPanelHost'}
      onMouseDown={event => { if (open && event.target === event.currentTarget) onClose?.() }}
    >
      <section className={open ? 'workflowLogWindow' : 'workflowRunPanelContainer'} role={open ? 'dialog' : undefined} aria-modal={open ? 'true' : undefined} aria-label={title} ref={containerRef} onKeyDown={trapFocus}>
        {open && <div className="workflowLogWindowBar">
          <span className="workflowLogWindowTitle">{title}</span>
          <span className="workflowLogWindowHint">Esc 还原到右栏，保留当前筛选</span>
          <button type="button" className="modalClose" aria-label="关闭日志窗口" onClick={() => onClose?.()}>×</button>
        </div>}
        <RunLogPanel {...panelProps} onExpand={open ? undefined : onExpand} onClose={open ? undefined : onCollapse} />
      </section>
    </div>
  )
}
