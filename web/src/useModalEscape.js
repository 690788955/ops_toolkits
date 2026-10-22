import { useEffect, useRef } from 'react'

const handlers = []
let listening = false

function elementDepth(element) {
  let depth = 0
  let current = element
  while (current) {
    depth += 1
    current = current.parentElement
  }
  return depth
}

function handleKeyDown(event) {
  if (event.key !== 'Escape' || event.isComposing || handlers.length === 0) return
  let target = null
  let bestDepth = -1
  for (const entry of handlers) {
    const depth = entry.element ? elementDepth(entry.element) : 0
    if (depth >= bestDepth) {
      bestDepth = depth
      target = entry
    }
  }
  if (!target) return
  event.preventDefault()
  target.handler()
}

export function useModalEscape(handler, containerRef, enabled = true) {
  const handlerRef = useRef(handler)
  handlerRef.current = handler
  useEffect(() => {
    if (!enabled) return
    const entry = {
      element: containerRef?.current || null,
      handler: () => handlerRef.current?.()
    }
    handlers.push(entry)
    if (!listening) {
      document.addEventListener('keydown', handleKeyDown)
      listening = true
    }
    return () => {
      const index = handlers.indexOf(entry)
      if (index >= 0) handlers.splice(index, 1)
      if (handlers.length === 0 && listening) {
        document.removeEventListener('keydown', handleKeyDown)
        listening = false
      }
    }
  }, [containerRef, enabled])
}
