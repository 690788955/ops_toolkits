export function buildTextDiff(before, after) {
  const beforeLines = splitDiffLines(before)
  const afterLines = splitDiffLines(after)
  const table = buildLcsTable(beforeLines, afterLines)
  const rows = []
  let beforeIndex = 0
  let afterIndex = 0

  while (beforeIndex < beforeLines.length || afterIndex < afterLines.length) {
    if (beforeIndex < beforeLines.length && afterIndex < afterLines.length && beforeLines[beforeIndex] === afterLines[afterIndex]) {
      rows.push({
        type: 'context',
        oldLine: beforeIndex + 1,
        newLine: afterIndex + 1,
        text: beforeLines[beforeIndex]
      })
      beforeIndex += 1
      afterIndex += 1
      continue
    }

    if (afterIndex < afterLines.length && (beforeIndex === beforeLines.length || table[beforeIndex][afterIndex + 1] >= table[beforeIndex + 1][afterIndex])) {
      rows.push({
        type: 'add',
        oldLine: '',
        newLine: afterIndex + 1,
        text: afterLines[afterIndex]
      })
      afterIndex += 1
      continue
    }

    rows.push({
      type: 'remove',
      oldLine: beforeIndex + 1,
      newLine: '',
      text: beforeLines[beforeIndex]
    })
    beforeIndex += 1
  }

  const stats = rows.reduce((current, row) => ({
    added: current.added + (row.type === 'add' ? 1 : 0),
    removed: current.removed + (row.type === 'remove' ? 1 : 0),
    unchanged: current.unchanged + (row.type === 'context' ? 1 : 0)
  }), {added: 0, removed: 0, unchanged: 0})

  return {
    changed: stats.added > 0 || stats.removed > 0,
    rows,
    stats
  }
}

export function countTextMatches(content, findText) {
  const source = String(content || '')
  const needle = String(findText || '')
  if (!needle) return 0
  let count = 0
  let index = 0
  while (index <= source.length) {
    const nextIndex = source.indexOf(needle, index)
    if (nextIndex === -1) break
    count += 1
    index = nextIndex + needle.length
  }
  return count
}

export function replaceTextDraft(content, findText, replacement, mode = 'all') {
  const source = String(content || '')
  const needle = String(findText || '')
  const nextValue = String(replacement || '')
  if (!needle) return {content: source, replacements: 0}
  if (mode === 'first') {
    const index = source.indexOf(needle)
    if (index === -1) return {content: source, replacements: 0}
    return {
      content: source.slice(0, index) + nextValue + source.slice(index + needle.length),
      replacements: 1
    }
  }
  const replacements = countTextMatches(source, needle)
  if (replacements === 0) return {content: source, replacements: 0}
  return {
    content: source.split(needle).join(nextValue),
    replacements
  }
}

function splitDiffLines(value) {
  const lines = String(value || '').replace(/\r\n/g, '\n').split('\n')
  if (lines.length === 1 && lines[0] === '') return []
  if (lines[lines.length - 1] === '') lines.pop()
  return lines
}

function buildLcsTable(beforeLines, afterLines) {
  const table = Array.from({length: beforeLines.length + 1}, () => Array(afterLines.length + 1).fill(0))
  for (let beforeIndex = beforeLines.length - 1; beforeIndex >= 0; beforeIndex -= 1) {
    for (let afterIndex = afterLines.length - 1; afterIndex >= 0; afterIndex -= 1) {
      table[beforeIndex][afterIndex] = beforeLines[beforeIndex] === afterLines[afterIndex]
        ? table[beforeIndex + 1][afterIndex + 1] + 1
        : Math.max(table[beforeIndex + 1][afterIndex], table[beforeIndex][afterIndex + 1])
    }
  }
  return table
}
