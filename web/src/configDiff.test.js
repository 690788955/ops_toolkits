import {describe, expect, it} from 'vitest'
import {buildTextDiff} from './configDiff.js'

describe('config diff', () => {
  it('summarizes added, removed, and unchanged lines', () => {
    const diff = buildTextDiff('name=old\nkeep=true\nremove=1\n', 'name=new\nkeep=true\nadd=1\n')

    expect(diff.changed).toBe(true)
    expect(diff.stats).toEqual({added: 2, removed: 2, unchanged: 1})
    expect(diff.rows.map(row => row.type)).toEqual(['add', 'remove', 'context', 'add', 'remove'])
  })

  it('treats equal loaded and draft content as unchanged', () => {
    const diff = buildTextDiff('a=1\nb=2\n', 'a=1\nb=2\n')

    expect(diff.changed).toBe(false)
    expect(diff.stats).toEqual({added: 0, removed: 0, unchanged: 2})
  })
})
