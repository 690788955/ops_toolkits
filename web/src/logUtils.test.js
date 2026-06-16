import {describe, expect, it} from 'vitest'
import {LOG_LEVELS, countLogsByLevel, detectLogLevel, exportLogText, formatLogLine, stripAnsiCodes} from './logUtils.js'

describe('log utils', () => {
  it('detects common execution log levels', () => {
    expect(detectLogLevel('fatal: host unreachable')).toBe('fatal')
    expect(detectLogLevel('failed: command returned error')).toBe('error')
    expect(detectLogLevel('TASK [install package]')).toBe('header')
    expect(detectLogLevel('changed: [node1]')).toBe('changed')
    expect(detectLogLevel('ok: [node1]')).toBe('ok')
    expect(detectLogLevel('✅ completed')).toBe('success')
  })

  it('formats log lines with ansi labels', () => {
    const line = formatLogLine('failed: boom', {item_id: 'step1', stream: 'stderr'})
    expect(stripAnsiCodes(line)).toContain('ERROR step1 stderr failed: boom')
  })

  it('counts and exports filtered logs', () => {
    const logs = [
      {text: 'ok: [node1]', level: 'ok'},
      {text: 'failed: boom', level: 'error'},
      {text: 'plain', level: 'log'}
    ]
    const counts = countLogsByLevel(logs)
    expect(counts.ok).toBe(1)
    expect(counts.error).toBe(1)
    expect(LOG_LEVELS.every(level => level in counts)).toBe(true)
    expect(exportLogText(logs, new Set(['error']))).toBe('failed: boom')
  })
})
