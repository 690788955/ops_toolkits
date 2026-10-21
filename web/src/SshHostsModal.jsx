import React, { useEffect, useState } from 'react'
import { deleteJSON, postJSON, putJSON } from './api.js'
import { readableAPIError } from './utils.js'

const STATUS_LABELS = {
  online: '在线',
  offline: '离线',
  checking: '检测中',
  unknown: '未检测'
}

export function sshHostStatusLabel(status) {
  return STATUS_LABELS[status] || STATUS_LABELS.unknown
}

export function summarizeSshHosts(data) {
  const hosts = (data?.hosts || []).filter(host => host.enabled)
  const total = hosts.length
  const online = hosts.filter(host => host.status === 'online').length
  const checking = hosts.some(host => host.status === 'checking')
  const offline = hosts.some(host => host.status === 'offline')
  const unknown = hosts.some(host => host.status === 'unknown')
  let state = 'unknown'
  if (total === 0) state = 'unknown'
  else if (checking) state = 'checking'
  else if (offline) state = 'offline'
  else if (online === total) state = 'online'
  else if (unknown) state = 'unknown'
  return {total, online, checking, offline, unknown, state}
}

function emptyForm() {
  return {
    id: '',
    name: '',
    address: '',
    port: 22,
    user: 'root',
    auth: 'key',
    key_path: '',
    password: '',
    passphrase: '',
    known_hosts_path: '',
    bundle_base_dir: '',
    strict_host_key: true,
    enabled: true,
    description: '',
    has_password: false,
    has_passphrase: false
  }
}

function hostToForm(host) {
  return {
    id: host.id,
    name: host.name || '',
    address: host.address || '',
    port: host.port || 22,
    user: host.user || '',
    auth: host.auth || 'key',
    key_path: host.key_path || '',
    password: '',
    passphrase: '',
    known_hosts_path: host.known_hosts_path || '',
    bundle_base_dir: host.bundle_base_dir || '',
    strict_host_key: host.strict_host_key !== false,
    enabled: host.enabled !== false,
    description: host.description || '',
    has_password: Boolean(host.has_password),
    has_passphrase: Boolean(host.has_passphrase)
  }
}

function formatCheckedAt(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = number => String(number).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

export default function SshHostsModal({data, error, onClose, onRefresh}) {
  const [message, setMessage] = useState('')
  const [settings, setSettings] = useState({interval: '60', timeout: '8'})
  const [savingSettings, setSavingSettings] = useState(false)
  const [checkingAll, setCheckingAll] = useState(false)
  const [form, setForm] = useState(null)
  const [saving, setSaving] = useState(false)
  const [checkingId, setCheckingId] = useState('')

  useEffect(() => {
    if (!data) return
    setSettings({
      interval: String(data.interval_seconds || 60),
      timeout: String(data.timeout_seconds || 8)
    })
  }, [data?.interval_seconds, data?.timeout_seconds])

  const hosts = data?.hosts || []

  function updateField(field, value) {
    setForm(current => current ? {...current, [field]: value} : current)
  }

  function startCreate() {
    setMessage('')
    setForm(emptyForm())
  }

  function startEdit(host) {
    setMessage('')
    setForm(hostToForm(host))
  }

  async function handleRefresh() {
    try {
      await onRefresh()
    } catch {
      // 错误信息由父组件展示。
    }
  }

  async function saveSettings() {
    const interval = Number(settings.interval)
    const timeout = Number(settings.timeout)
    if (!Number.isInteger(interval) || interval < 5 || interval > 3600) {
      setMessage('巡检间隔必须是 5-3600 秒之间的整数。')
      return
    }
    if (!Number.isInteger(timeout) || timeout < 2 || timeout > 60) {
      setMessage('连接超时必须是 2-60 秒之间的整数。')
      return
    }
    setSavingSettings(true)
    setMessage('正在保存巡检设置...')
    try {
      const body = await putJSON('/api/ssh/settings', {interval_seconds: interval, timeout_seconds: timeout})
      if (body?.data) {
        setSettings({
          interval: String(body.data.interval_seconds || interval),
          timeout: String(body.data.timeout_seconds || timeout)
        })
      }
      setMessage('巡检设置已保存，新的巡检周期会立即生效。')
      await handleRefresh()
    } catch (err) {
      setMessage(readableAPIError(err, '保存巡检设置失败。'))
    } finally {
      setSavingSettings(false)
    }
  }

  async function checkAllNow() {
    setCheckingAll(true)
    setMessage('已触发巡检，状态会在检测完成后更新。')
    try {
      await postJSON('/api/ssh/check', {})
      window.setTimeout(() => { handleRefresh() }, 2000)
    } catch (err) {
      setMessage(readableAPIError(err, '触发巡检失败。'))
    } finally {
      setCheckingAll(false)
    }
  }

  async function saveHost() {
    if (!form) return
    const address = form.address.trim()
    const user = form.user.trim()
    if (!address) {
      setMessage('请填写主机地址。')
      return
    }
    if (!user) {
      setMessage('请填写用户名。')
      return
    }
    const port = Number(form.port)
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      setMessage('端口必须是 1-65535 之间的整数。')
      return
    }
    if (form.auth === 'password' && !form.password && !form.has_password) {
      setMessage('密码认证需要填写登录密码。')
      return
    }
    const payload = {
      name: form.name.trim(),
      address,
      port,
      user,
      auth: form.auth,
      key_path: form.key_path.trim(),
      password: form.password,
      passphrase: form.passphrase,
      strict_host_key: Boolean(form.strict_host_key),
      known_hosts_path: form.known_hosts_path.trim(),
      bundle_base_dir: form.bundle_base_dir.trim(),
      enabled: Boolean(form.enabled),
      description: form.description.trim()
    }
    setSaving(true)
    setMessage(form.id ? '正在保存操作机...' : '正在添加操作机...')
    try {
      const body = form.id
        ? await putJSON(`/api/ssh/hosts/${encodeURIComponent(form.id)}`, payload)
        : await postJSON('/api/ssh/hosts', payload)
      setMessage(body?.data?.name ? `操作机 ${body.data.name} 已保存。` : '操作机已保存。')
      setForm(null)
      await handleRefresh()
    } catch (err) {
      setMessage(readableAPIError(err, '保存操作机失败。'))
    } finally {
      setSaving(false)
    }
  }

  async function removeHost(host) {
    if (!window.confirm(`确认删除操作机 ${host.name || host.address}？`)) return
    setMessage(`正在删除 ${host.name || host.address}...`)
    try {
      await deleteJSON(`/api/ssh/hosts/${encodeURIComponent(host.id)}`)
      setMessage('操作机已删除。')
      if (form?.id === host.id) setForm(null)
      await handleRefresh()
    } catch (err) {
      setMessage(readableAPIError(err, '删除操作机失败。'))
    }
  }

  async function checkHost(host) {
    setCheckingId(host.id)
    setMessage(`正在检测 ${host.name || host.address}...`)
    try {
      const body = await postJSON(`/api/ssh/hosts/${encodeURIComponent(host.id)}/check`, {})
      const view = body?.data
      if (view?.status === 'online') {
        setMessage(`${host.name || host.address} 连接成功，耗时 ${view.latency_ms || 0} ms。`)
      } else {
        setMessage(`${host.name || host.address} 连接失败：${view?.last_error || '未知错误'}`)
      }
      await handleRefresh()
    } catch (err) {
      setMessage(readableAPIError(err, '检测操作机失败。'))
    } finally {
      setCheckingId('')
    }
  }

  return (
    <div className="modalBackdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal sshModal" role="dialog" aria-modal="true" aria-labelledby="ssh-hosts-title">
        <div className="modalHeader">
          <div>
            <h3 id="ssh-hosts-title">操作机</h3>
            <p>通过 SSH 管理可下发命令的远程机器。后台按巡检间隔自动探活，并把连接状态同步到右上角指示灯。</p>
          </div>
          <button className="modalClose" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>

        {error && <div className="sshAlert danger">{error}</div>}

        <div className="sshToolbar">
          <div className="sshSettingsPanel">
            <label>
              <span>巡检间隔</span>
              <input type="number" min="5" max="3600" value={settings.interval} disabled={savingSettings} onChange={event => setSettings(current => ({...current, interval: event.target.value}))} />
              <small>秒</small>
            </label>
            <label>
              <span>连接超时</span>
              <input type="number" min="2" max="60" value={settings.timeout} disabled={savingSettings} onChange={event => setSettings(current => ({...current, timeout: event.target.value}))} />
              <small>秒</small>
            </label>
            <button className="secondary" type="button" disabled={savingSettings || !data} onClick={saveSettings}>{savingSettings ? '保存中...' : '保存巡检设置'}</button>
            <button className="secondary" type="button" disabled={checkingAll || !data} onClick={checkAllNow}>{checkingAll ? '触发中...' : '立即巡检'}</button>
          </div>
          <button className="primary" type="button" disabled={Boolean(form)} onClick={startCreate}>新增操作机</button>
        </div>

        {form && (
          <section className="sshHostForm">
            <div className="sshHostFormHeader">
              <strong>{form.id ? '编辑操作机' : '新增操作机'}</strong>
              <span>密码和私钥口令会保存到 {data?.config_path || 'configs/ssh_hosts.yaml'}，请限制该文件的访问权限。</span>
            </div>
            <div className="sshFormGrid">
              <label>
                <span>名称</span>
                <input value={form.name} disabled={saving} placeholder="例如：生产 Web 01" onChange={event => updateField('name', event.target.value)} />
              </label>
              <label>
                <span>主机地址</span>
                <input value={form.address} disabled={saving} placeholder="10.0.0.10 或 host.example.com" onChange={event => updateField('address', event.target.value)} />
              </label>
              <label>
                <span>端口</span>
                <input type="number" min="1" max="65535" value={form.port} disabled={saving} onChange={event => updateField('port', event.target.value)} />
              </label>
              <label>
                <span>用户名</span>
                <input value={form.user} disabled={saving} autoComplete="off" placeholder="root" onChange={event => updateField('user', event.target.value)} />
              </label>
              <label>
                <span>认证方式</span>
                <select value={form.auth} disabled={saving} onChange={event => updateField('auth', event.target.value)}>
                  <option value="key">私钥</option>
                  <option value="password">密码</option>
                </select>
              </label>
              {form.auth === 'key' ? (
                <>
                  <label>
                    <span>私钥路径</span>
                    <input value={form.key_path} disabled={saving} placeholder="~/.ssh/id_rsa（留空自动查找）" onChange={event => updateField('key_path', event.target.value)} />
                  </label>
                  <label>
                    <span>私钥口令</span>
                    <input type="password" value={form.passphrase} disabled={saving} autoComplete="new-password" placeholder={form.has_passphrase ? '已保存，留空保持不变' : '未加密私钥可留空'} onChange={event => updateField('passphrase', event.target.value)} />
                  </label>
                </>
              ) : (
                <label>
                  <span>登录密码</span>
                  <input type="password" value={form.password} disabled={saving} autoComplete="new-password" placeholder={form.has_password ? '已保存，留空保持不变' : 'SSH 登录密码'} onChange={event => updateField('password', event.target.value)} />
                </label>
              )}
              <label className="sshFormWide">
                <span>known_hosts 路径</span>
                <input value={form.known_hosts_path} disabled={saving || !form.strict_host_key} placeholder="~/.ssh/known_hosts（留空使用默认）" onChange={event => updateField('known_hosts_path', event.target.value)} />
              </label>
              <label className="sshFormWide">
                <span>下发根目录</span>
                <input value={form.bundle_base_dir} disabled={saving} placeholder="/tmp 或 /data/opsctl（留空使用 TMPDIR 或 /tmp）" onChange={event => updateField('bundle_base_dir', event.target.value)} />
                <small className="sshFormHint">远程执行时工具包会解压到此目录下的临时子目录，执行结束自动清理；需保证 SSH 用户有写权限。</small>
              </label>
              <label className="checkboxLabel sshFormWide">
                <input type="checkbox" checked={form.strict_host_key} disabled={saving} onChange={event => updateField('strict_host_key', event.target.checked)} />
                <span>严格校验主机密钥（推荐；首次连接需要先把主机指纹加入 known_hosts）</span>
              </label>
              <label className="checkboxLabel sshFormWide">
                <input type="checkbox" checked={form.enabled} disabled={saving} onChange={event => updateField('enabled', event.target.checked)} />
                <span>启用自动巡检</span>
              </label>
              <label className="sshFormWide">
                <span>描述</span>
                <input value={form.description} disabled={saving} placeholder="用途、环境或负责人" onChange={event => updateField('description', event.target.value)} />
              </label>
            </div>
            <div className="buttonRow compactButtonRow">
              <button className="primary" type="button" disabled={saving} onClick={saveHost}>{saving ? '保存中...' : '保存操作机'}</button>
              <button className="secondary" type="button" disabled={saving} onClick={() => setForm(null)}>取消</button>
            </div>
          </section>
        )}

        <div className="sshHostList">
          {hosts.length === 0 && (
            <div className="empty">
              {data ? '还没有配置操作机。点击“新增操作机”填写 SSH 地址、用户名和私钥或密码。' : '正在加载操作机配置...'}
            </div>
          )}
          {hosts.map(host => {
            const state = host.enabled ? (host.status || 'unknown') : 'unknown'
            const busy = checkingId === host.id
            return (
              <article className={`sshHostCard${host.enabled ? '' : ' disabled'}`} key={host.id}>
                <div className="sshHostMain">
                  <span className={`sshDot ${state}`} aria-hidden="true" />
                  <div className="sshHostInfo">
                    <div className="sshHostTitle">
                      <strong>{host.name || host.address}</strong>
                      <span className={`sshStatePill ${host.enabled ? state : 'disabled'}`}>
                        {host.enabled ? (STATUS_LABELS[state] || state) : '已停用巡检'}
                      </span>
                    </div>
                    <code>{host.user}@{host.address}:{host.port}</code>
                    <small>
                      {host.auth === 'key' ? `私钥认证${host.key_path ? ` · ${host.key_path}` : ' · 自动查找'}` : '密码认证'}
                      {host.strict_host_key ? ' · 严格主机密钥' : ' · 跳过主机密钥校验'}
                      {host.bundle_base_dir ? ` · 下发至 ${host.bundle_base_dir}` : ''}
                      {host.last_checked ? ` · 最近检测 ${formatCheckedAt(host.last_checked)}` : ''}
                      {state === 'online' && host.latency_ms ? ` · ${host.latency_ms} ms` : ''}
                    </small>
                    {host.description && <small>{host.description}</small>}
                    {host.last_error && <p className="sshHostError" title={host.last_error}>{host.last_error}</p>}
                    {host.fingerprint && <small className="sshFingerprint">主机指纹 {host.fingerprint}</small>}
                  </div>
                </div>
                <div className="sshHostActions">
                  <button className="secondary" type="button" disabled={busy} onClick={() => checkHost(host)}>{busy ? '检测中...' : '检测'}</button>
                  <button className="secondary" type="button" onClick={() => startEdit(host)}>编辑</button>
                  <button className="secondary danger" type="button" onClick={() => removeHost(host)}>删除</button>
                </div>
              </article>
            )
          })}
        </div>

        <div className="sshModalFooter">
          <span className="empty small">配置文件：{data?.config_path || 'configs/ssh_hosts.yaml'}。状态由后端巡检维护，页面每 5 秒同步一次。</span>
          <pre className="modalResult sshModalMessage" aria-live="polite">{message}</pre>
        </div>
      </div>
    </div>
  )
}
