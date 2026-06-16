import {forwardRef, useEffect, useImperativeHandle, useRef} from 'react'
import {Terminal} from '@xterm/xterm'
import {FitAddon} from '@xterm/addon-fit'
import {SearchAddon} from '@xterm/addon-search'
import {WebLinksAddon} from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

const DEFAULT_OPTIONS = {
  theme: {
    background: '#101113',
    foreground: '#f4f4f5',
    cursor: '#f5c542',
    selectionBackground: '#3b82f680',
    black: '#51525a',
    red: '#ff6b6b',
    green: '#2fb344',
    yellow: '#f5c542',
    blue: '#5b8def',
    magenta: '#b56cff',
    cyan: '#35c2c1',
    white: '#e5e7eb'
  },
  fontSize: 14,
  fontFamily: '"JetBrains Mono", ui-monospace, monospace',
  lineHeight: 1.45,
  cursorBlink: false,
  scrollback: 50000,
  disableStdin: true,
  convertEol: true
}

const XTerminal = forwardRef(function XTerminal({className = '', options = {}, onReady}, ref) {
  const containerRef = useRef(null)
  const terminalRef = useRef(null)
  const fitAddonRef = useRef(null)
  const searchAddonRef = useRef(null)

  useImperativeHandle(ref, () => ({
    writeln(value) {
      terminalRef.current?.writeln(value)
    },
    reset() {
      terminalRef.current?.reset()
    },
    scrollToBottom() {
      terminalRef.current?.scrollToBottom()
    },
    findNext(term, searchOptions = {}) {
      if (!term) return false
      return searchAddonRef.current?.findNext(term, {...searchOptions, incremental: true}) || false
    },
    findPrevious(term, searchOptions = {}) {
      if (!term) return false
      return searchAddonRef.current?.findPrevious(term, {...searchOptions, incremental: true}) || false
    },
    clearSearch() {
      searchAddonRef.current?.clearDecorations()
    },
    fit() {
      fitAddonRef.current?.fit()
    },
    getTerminal() {
      return terminalRef.current
    }
  }), [])

  useEffect(() => {
    if (!containerRef.current) return undefined
    const terminal = new Terminal({...DEFAULT_OPTIONS, ...options, theme: {...DEFAULT_OPTIONS.theme, ...(options.theme || {})}})
    const fitAddon = new FitAddon()
    const searchAddon = new SearchAddon()
    terminal.loadAddon(fitAddon)
    terminal.loadAddon(searchAddon)
    terminal.loadAddon(new WebLinksAddon())
    terminal.open(containerRef.current)
    terminalRef.current = terminal
    fitAddonRef.current = fitAddon
    searchAddonRef.current = searchAddon

    let fitTimer = 0
    const fit = () => {
      window.clearTimeout(fitTimer)
      fitTimer = window.setTimeout(() => {
        if (containerRef.current?.clientWidth && containerRef.current?.clientHeight) {
          fitAddon.fit()
        }
      }, 50)
    }
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(fit)
    observer?.observe(containerRef.current)
    window.addEventListener('resize', fit)
    window.requestAnimationFrame(fit)
    onReady?.(terminal)

    return () => {
      window.clearTimeout(fitTimer)
      observer?.disconnect()
      window.removeEventListener('resize', fit)
      terminal.dispose()
      terminalRef.current = null
      fitAddonRef.current = null
      searchAddonRef.current = null
    }
  }, [])

  return <div ref={containerRef} className={`xTerminal ${className}`} />
})

export default XTerminal
