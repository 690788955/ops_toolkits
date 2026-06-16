// @vitest-environment jsdom
import React, {createRef} from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import {describe, expect, it, vi} from 'vitest'
import XTerminal from './XTerminal.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

const terminal = {
  loadAddon: vi.fn(),
  open: vi.fn(),
  writeln: vi.fn(),
  reset: vi.fn(),
  scrollToBottom: vi.fn(),
  dispose: vi.fn()
}
const searchAddon = {
  findNext: vi.fn(() => true),
  findPrevious: vi.fn(() => true),
  clearDecorations: vi.fn()
}
const fitAddon = {fit: vi.fn()}

vi.mock('@xterm/xterm', () => ({Terminal: vi.fn(function Terminal() { return terminal })}))
vi.mock('@xterm/addon-fit', () => ({FitAddon: vi.fn(function FitAddon() { return fitAddon })}))
vi.mock('@xterm/addon-search', () => ({SearchAddon: vi.fn(function SearchAddon() { return searchAddon })}))
vi.mock('@xterm/addon-web-links', () => ({WebLinksAddon: vi.fn(function WebLinksAddon() { return {} })}))
vi.mock('@xterm/xterm/css/xterm.css', () => ({}))

describe('XTerminal', () => {
  it('exposes terminal methods through ref', async () => {
    const ref = createRef()
    const element = document.createElement('div')
    document.body.appendChild(element)
    const root = createRoot(element)
    await act(async () => {
      root.render(<XTerminal ref={ref} />)
    })

    ref.current.writeln('hello')
    ref.current.reset()
    ref.current.scrollToBottom()
    expect(ref.current.findNext('hello')).toBe(true)
    expect(ref.current.findPrevious('hello')).toBe(true)
    ref.current.clearSearch()

    expect(terminal.writeln).toHaveBeenCalledWith('hello')
    expect(terminal.reset).toHaveBeenCalled()
    expect(terminal.scrollToBottom).toHaveBeenCalled()
    expect(searchAddon.findNext).toHaveBeenCalled()
    expect(searchAddon.findPrevious).toHaveBeenCalled()
    expect(searchAddon.clearDecorations).toHaveBeenCalled()

    await act(async () => root.unmount())
    document.body.removeChild(element)
  })
})
