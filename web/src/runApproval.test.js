import {describe, expect, it} from 'vitest'
import {formatApprovalWait, pendingRunApproval, runAwaitingApproval} from './runApproval.js'

describe('runApproval', () => {
  it('pendingRunApproval 只把最后一条仍在 waiting 的审批当成待处理项', () => {
    expect(pendingRunApproval(null)).toBeNull()
    expect(pendingRunApproval({approvals: [{node_id: 'a', status: 'approved'}]})).toBeNull()
    const record = {
      approvals: [
        {node_id: 'a', status: 'waiting', message: '旧审批'},
        {node_id: 'b', status: 'waiting', message: '新审批'},
        {node_id: '', status: 'waiting'}
      ]
    }
    expect(pendingRunApproval(record)).toMatchObject({node_id: 'b', message: '新审批'})
  })

  it('runAwaitingApproval 同时认列表项摘要和详情 approvals', () => {
    expect(runAwaitingApproval(null)).toBeNull()
    expect(runAwaitingApproval({status: 'succeeded'})).toBeNull()
    expect(runAwaitingApproval({
      awaiting_approval: true,
      approval: {node_id: 'approval_2', message: '请修改主机清单', since: '2026-10-30T12:27:31.123Z'}
    })).toEqual({node_id: 'approval_2', message: '请修改主机清单', since: '2026-10-30T12:27:31.123Z'})
    expect(runAwaitingApproval({approvals: [{node_id: 'gate', status: 'waiting'}]})).toMatchObject({node_id: 'gate'})
  })

  it('formatApprovalWait 把等待时长换算成易读文案，非法时间返回空串', () => {
    const base = new Date('2026-10-30T12:00:00Z').getTime()
    expect(formatApprovalWait('2026-10-30T11:59:30Z', base)).toBe('不到 1 分钟')
    expect(formatApprovalWait('2026-10-30T11:50:00Z', base)).toBe('10 分钟')
    expect(formatApprovalWait('2026-10-30T09:00:00Z', base)).toBe('3 小时')
    expect(formatApprovalWait('2026-10-30T10:15:00Z', base)).toBe('1 小时 45 分')
    expect(formatApprovalWait('2026-10-28T09:00:00Z', base)).toBe('2 天 3 小时')
    expect(formatApprovalWait('', base)).toBe('')
    expect(formatApprovalWait('not-a-time', base)).toBe('')
    // 时钟回退时按 0 处理，不能出现负数等待
    expect(formatApprovalWait('2026-10-30T12:00:00Z', base - 5000)).toBe('不到 1 分钟')
  })
})