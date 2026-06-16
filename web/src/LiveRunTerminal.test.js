import {describe, expect, it} from 'vitest'
import {normalizeLogFontSize} from './logUtils.js'

describe('LiveRunTerminal preferences', () => {
  it('normalizes log font size from UI preferences', () => {
    expect(normalizeLogFontSize(undefined)).toBe(14)
    expect(normalizeLogFontSize('abc')).toBe(14)
    expect(normalizeLogFontSize(11)).toBe(14)
    expect(normalizeLogFontSize(21)).toBe(14)
    expect(normalizeLogFontSize('16')).toBe(16)
    expect(normalizeLogFontSize(16.4)).toBe(16)
  })
})
