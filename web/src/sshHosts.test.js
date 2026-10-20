import {describe, expect, it} from 'vitest'
import {summarizeSshHosts} from './SshHostsModal.jsx'

describe('summarizeSshHosts', () => {
  it('returns unknown when no hosts are configured', () => {
    expect(summarizeSshHosts(null)).toMatchObject({total: 0, online: 0, state: 'unknown'})
    expect(summarizeSshHosts({hosts: []})).toMatchObject({total: 0, state: 'unknown'})
  })

  it('returns online when every enabled host is online', () => {
    const summary = summarizeSshHosts({
      hosts: [
        {enabled: true, status: 'online'},
        {enabled: true, status: 'online'}
      ]
    })
    expect(summary).toMatchObject({total: 2, online: 2, state: 'online'})
  })

  it('prefers checking while a scan is running', () => {
    const summary = summarizeSshHosts({
      hosts: [
        {enabled: true, status: 'online'},
        {enabled: true, status: 'checking'}
      ]
    })
    expect(summary.state).toBe('checking')
  })

  it('returns offline when any enabled host is offline', () => {
    const summary = summarizeSshHosts({
      hosts: [
        {enabled: true, status: 'online'},
        {enabled: true, status: 'offline'}
      ]
    })
    expect(summary).toMatchObject({online: 1, offline: true, state: 'offline'})
  })

  it('ignores disabled hosts', () => {
    const summary = summarizeSshHosts({
      hosts: [
        {enabled: true, status: 'online'},
        {enabled: false, status: 'offline'}
      ]
    })
    expect(summary).toMatchObject({total: 1, online: 1, state: 'online'})
  })
})
