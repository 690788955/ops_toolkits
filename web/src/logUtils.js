const ANSI = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  black: '\x1b[30m',
  white: '\x1b[37m',
  bgBlack: '\x1b[40m',
  bgRed: '\x1b[41m',
  bgGreen: '\x1b[42m',
  bgYellow: '\x1b[43m',
  bgBlue: '\x1b[44m',
  bgCyan: '\x1b[46m'
}

export const LOG_LEVELS = ['fatal', 'error', 'warning', 'changed', 'ok', 'success', 'header', 'info', 'log']
export const DEFAULT_LOG_FONT_SIZE = 14

const LABELS = {
  fatal: {label: 'FATAL', color: `${ANSI.bgRed}${ANSI.white}${ANSI.bold}`},
  error: {label: 'ERROR', color: `${ANSI.bgRed}${ANSI.white}${ANSI.bold}`},
  warning: {label: 'WARN', color: `${ANSI.bgYellow}${ANSI.black}${ANSI.bold}`},
  changed: {label: 'CHANGED', color: `${ANSI.bgYellow}${ANSI.black}${ANSI.bold}`},
  ok: {label: 'OK', color: `${ANSI.bgGreen}${ANSI.black}${ANSI.bold}`},
  success: {label: 'SUCCESS', color: `${ANSI.bgGreen}${ANSI.white}${ANSI.bold}`},
  header: {label: 'STEP', color: `${ANSI.bgBlue}${ANSI.white}${ANSI.bold}`},
  info: {label: 'INFO', color: `${ANSI.bgCyan}${ANSI.black}`},
  log: {label: 'LOG', color: `${ANSI.bgBlack}${ANSI.white}`}
}

export function stripAnsiCodes(text) {
  if (!text) return ''
  // eslint-disable-next-line no-control-regex
  return String(text).replace(/\x1b\[[0-9;]*m|\033\[[0-9;]*m|\[\d+(?:;\d+)*m/g, '')
}

export function detectLogLevel(text) {
  const clean = stripAnsiCodes(text)
  const lower = clean.toLowerCase()
  if (!lower) return 'log'
  if (lower.includes('play recap') || lower.includes('task [') || lower.includes('play [')) return 'header'
  if (lower.includes('fatal:') || lower.includes('fatal error') || lower.includes('fatal]')) return 'fatal'
  if (lower.includes('error') || lower.includes('failed:') || lower.includes('failed]') || lower.includes('❌')) return 'error'
  if (lower.includes('warning') || lower.includes('[warn]') || lower.includes('⚠')) return 'warning'
  if (lower.includes('changed:') || lower.includes('changed]')) return 'changed'
  if (lower.includes('ok:') || lower.includes('ok]') || lower.includes('skipping:') || lower.includes('skipped]')) return 'ok'
  if (lower.includes('success') || lower.includes('completed') || lower.includes('完成') || lower.includes('✅')) return 'success'
  if (/[🚀📋🏷📁📝]/.test(clean)) return 'info'
  return 'log'
}

export function formatLogLine(text, event = {}) {
  const level = detectLogLevel(text)
  const label = LABELS[level] || LABELS.log
  const prefix = event.item_id ? ` ${event.item_id}` : ''
  const stream = event.stream === 'stderr' ? ' stderr' : ''
  return `${label.color}${label.label}${ANSI.reset}${prefix}${stream} ${text}`
}

export function countLogsByLevel(logs) {
  const counts = Object.fromEntries(LOG_LEVELS.map(level => [level, 0]))
  ;(logs || []).forEach(log => {
    const level = log.level || detectLogLevel(log.text)
    counts[level] = (counts[level] || 0) + 1
  })
  return counts
}

export function exportLogText(logs, filters = new Set(LOG_LEVELS)) {
  return (logs || [])
    .filter(log => filters.has(log.level || detectLogLevel(log.text)))
    .map(log => log.text)
    .join('\n')
}

export function normalizeLogFontSize(value) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed < 12 || parsed > 20) return DEFAULT_LOG_FONT_SIZE
  return Math.round(parsed)
}
