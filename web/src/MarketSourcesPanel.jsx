import React, {useEffect, useState} from 'react'
import NexusRepositoryManager from './NexusPanel.jsx'
import {createMarketSource, deleteMarketSource, fetchMarketSources, testMarketSource, updateMarketSource} from './api.js'
import {readableAPIError} from './utils.js'

const typeLabels = {
  local: '本地市场',
  remote: '远程市场',
  nexus: 'Nexus'
}

const typeDescriptions = {
  local: '读取本机目录或 index.json 文件。',
  remote: '读取 HTTP/HTTPS 上的 index.json，可使用匿名或账号密码访问。',
  nexus: 'Nexus 3 Raw 仓库，上传/删除插件时自动维护 index.json。'
}

const emptyForm = {
  id: '',
  name: '',
  type: 'nexus',
  url: '',
  enabled: true,
  auth_type: 'anonymous',
  username: '',
  password: '',
  insecure_skip_verify: false,
  repository: '',
  index_path: 'index.json',
  package_dir: 'packages'
}

function defaultName(type) {
  if (type === 'nexus') return 'Nexus 插件仓库'
  if (type === 'remote') return '远程插件市场'
  return '本地插件市场'
}

function formFromSource(source) {
  return {
    id: source?.id || '',
    name: source?.name || '',
    type: source?.type || 'nexus',
    url: source?.url || '',
    enabled: source?.enabled !== false,
    auth_type: source?.auth_type || 'anonymous',
    username: source?.username || '',
    password: '',
    insecure_skip_verify: Boolean(source?.insecure_skip_verify),
    repository: source?.repository || '',
    index_path: source?.index_path || 'index.json',
    package_dir: source?.package_dir || 'packages'
  }
}

function payloadFromForm(form) {
  return {
    id: form.id.trim(),
    name: form.name.trim(),
    type: form.type,
    url: form.url.trim(),
    enabled: Boolean(form.enabled),
    auth_type: form.auth_type,
    username: form.auth_type === 'basic' ? form.username.trim() : '',
    password: form.auth_type === 'basic' ? form.password : '',
    insecure_skip_verify: form.type === 'nexus' ? Boolean(form.insecure_skip_verify) : false,
    repository: form.type === 'nexus' ? form.repository.trim() : '',
    index_path: form.type === 'nexus' ? form.index_path.trim() : '',
    package_dir: form.type === 'nexus' ? form.package_dir.trim() : ''
  }
}

export function MarketSourcesModal({onClose, onChanged}) {
  return (
    <div className="modalBackdrop modalBackdropNested" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal marketSourcesModal" role="dialog" aria-modal="true" aria-labelledby="market-sources-modal-title">
        <div className="modalHeader">
          <div>
            <h3 id="market-sources-modal-title">市场来源配置</h3>
            <p>维护本地、远程与 Nexus 来源，Nexus 支持匿名或账号密码访问。</p>
          </div>
          <button className="modalClose" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        <MarketSourcesPanel onChanged={onChanged} />
      </div>
    </div>
  )
}

export default function MarketSourcesPanel({onChanged}) {
  const [sources, setSources] = useState([])
  const [configPath, setConfigPath] = useState('')
  const [loading, setLoading] = useState(true)
  const [view, setView] = useState('list')
  const [editing, setEditing] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState(null)
  const [deletingID, setDeletingID] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    loadSources()
  }, [])

  async function loadSources() {
    setLoading(true)
    setError('')
    try {
      const body = await fetchMarketSources()
      const data = body.data || {}
      setSources(data.sources || [])
      setConfigPath(data.config_path || '')
    } catch (err) {
      setError(readableAPIError(err, '加载市场来源配置失败。'))
    } finally {
      setLoading(false)
    }
  }

  function updateField(field, value) {
    setForm(current => ({...current, [field]: value}))
  }

  function changeType(type) {
    setForm(current => ({
      ...current,
      type,
      name: current.name && current.name !== defaultName(current.type) ? current.name : defaultName(type),
      auth_type: type === 'local' ? 'anonymous' : current.auth_type,
      username: type === 'local' ? '' : current.username,
      password: type === 'local' ? '' : current.password,
      repository: type === 'nexus' ? current.repository : '',
      index_path: type === 'nexus' ? current.index_path || 'index.json' : '',
      package_dir: type === 'nexus' ? current.package_dir || 'packages' : '',
      insecure_skip_verify: type === 'nexus' ? current.insecure_skip_verify : false
    }))
  }

  function startCreate() {
    setEditing(null)
    setForm({...emptyForm, name: defaultName(emptyForm.type)})
    setTestResult(null)
    setMessage('')
    setError('')
    setView('editor')
  }

  function startEdit(source) {
    setEditing(source)
    setForm(formFromSource(source))
    setTestResult(null)
    setMessage('')
    setError('')
    setView('editor')
  }

  function backToList() {
    setView('list')
    setEditing(null)
    setTestResult(null)
    setError('')
    setMessage('')
  }

  function validateForm() {
    if (!form.id.trim()) return '请填写市场来源 ID。'
    if (!form.url.trim()) return form.type === 'nexus' ? '请填写 Nexus 地址。' : form.type === 'remote' ? '请填写市场索引地址。' : '请填写本地市场目录。'
    if (form.type === 'nexus' && !form.repository.trim()) return '请填写 Nexus Raw 仓库名。'
    if (form.auth_type === 'basic' && form.type !== 'local' && !form.username.trim()) return '请填写访问用户名。'
    return ''
  }

  async function saveSource() {
    const validation = validateForm()
    if (validation) {
      setError(validation)
      return
    }
    setSaving(true)
    setError('')
    setTestResult(null)
    setMessage(editing ? '正在保存市场来源...' : '正在新增市场来源...')
    try {
      const payload = payloadFromForm(form)
      const body = editing ? await updateMarketSource(payload) : await createMarketSource(payload)
      const data = body.data || {}
      setEditing(data)
      setForm(formFromSource(data))
      setMessage(`市场来源 ${data.name || data.id} 已保存。`)
      await loadSources()
      onChanged?.()
    } catch (err) {
      setError(readableAPIError(err, '保存市场来源失败。'))
      setMessage('')
    } finally {
      setSaving(false)
    }
  }

  async function testConnection() {
    const validation = validateForm()
    if (validation) {
      setError(validation)
      return
    }
    setTesting(true)
    setError('')
    setTestResult(null)
    setMessage('正在测试市场来源连接...')
    try {
      const body = await testMarketSource(payloadFromForm(form))
      setTestResult(body.data || null)
      setMessage('')
    } catch (err) {
      setError(readableAPIError(err, '测试市场来源失败。'))
      setMessage('')
    } finally {
      setTesting(false)
    }
  }

  async function removeSource(source) {
    if (!window.confirm(`确定删除市场来源「${source.name || source.id}」？\n\n只会移除来源配置，不影响本机已安装的插件。`)) return
    setDeletingID(source.id)
    setError('')
    setMessage(`正在删除市场来源 ${source.name || source.id}...`)
    try {
      await deleteMarketSource(source.id)
      if (editing?.id === source.id) backToList()
      setMessage(`市场来源 ${source.name || source.id} 已删除。`)
      await loadSources()
      onChanged?.()
    } catch (err) {
      setError(readableAPIError(err, '删除市场来源失败。'))
      setMessage('')
    } finally {
      setDeletingID('')
    }
  }

  function sourceCard(source) {
    return (
      <article className="marketSourceCard" key={source.id}>
        <div className="marketSourceMain">
          <div className="marketSourceTitle">
            <strong>{source.name || source.id}</strong>
            <span className={`marketSourceType ${source.type}`}>{typeLabels[source.type] || source.type}</span>
            {!source.enabled && <span className="marketBadge">已禁用</span>}
            <span className={`marketBadge ${source.ready ? 'installed' : ''}`}>{source.ready ? '可用' : '待完善'}</span>
          </div>
          <span className="marketSourceURL">{source.url}</span>
          <div className="marketSourceMeta">
            <span>ID：{source.id}</span>
            {source.type === 'nexus' && <span>仓库：{source.repository || '-'}</span>}
            <span>{source.auth_type === 'basic' ? `账号：${source.username || '-'}` : '匿名访问'}</span>
            {source.type === 'nexus' && source.index_url && <span>{source.index_url}</span>}
          </div>
        </div>
        <div className="marketSourceActions">
          <button className="secondary" type="button" onClick={() => startEdit(source)}>编辑</button>
          <button className="secondary danger" type="button" disabled={deletingID === source.id} onClick={() => removeSource(source)}>{deletingID === source.id ? '删除中...' : '删除'}</button>
        </div>
      </article>
    )
  }

  const authFields = form.type !== 'local' && (
    <>
      <label>
        <span>访问方式</span>
        <select value={form.auth_type} onChange={event => updateField('auth_type', event.target.value)}>
          <option value="anonymous">匿名访问</option>
          <option value="basic">账号密码</option>
        </select>
      </label>
      {form.auth_type === 'basic' && (
        <>
          <label>
            <span>用户名</span>
            <input value={form.username} autoComplete="off" placeholder="ops" onChange={event => updateField('username', event.target.value)} />
          </label>
          <label>
            <span>密码 / Token</span>
            <input
              type="password"
              value={form.password}
              autoComplete="new-password"
              placeholder={editing?.password_set ? '已保存，留空保持不变' : '输入密码或访问 Token'}
              onChange={event => updateField('password', event.target.value)}
            />
          </label>
        </>
      )}
    </>
  )

  return (
    <div className="marketSourcePanel">
      {view === 'list' ? (
        <div className="marketSourceToolbar">
          <div>
            <strong>来源列表</strong>
            <span>启用后会出现在插件市场中，可以同时配置多个来源。</span>
          </div>
          <div className="marketSourceToolbarActions">
            <button className="primary" type="button" onClick={startCreate}>新增来源</button>
          </div>
        </div>
      ) : (
        <div className="marketSourceToolbar">
          <button className="secondary" type="button" onClick={backToList}>← 返回来源列表</button>
        </div>
      )}

      {error && <div className="marketSourceError">{error}</div>}
      {message && <small className="nexusMessage">{message}</small>}

      {view === 'list' ? (
        <>
          <small className="marketHint">配置文件：{configPath || 'configs/market_sources.yaml'}。密码只保存在本地，接口不会回显。</small>
          {loading ? (
            <div className="empty small">正在加载市场来源...</div>
          ) : sources.length === 0 ? (
            <div className="empty small">还没有市场来源，点击“新增来源”添加本地目录、远程索引或 Nexus 仓库。</div>
          ) : (
            <div className="marketSourceList">{sources.map(sourceCard)}</div>
          )}
        </>
      ) : (
        <>
          <section className="nexusConfigCard marketSourceEditor">
            <div className="nexusSectionHeader">
              <div>
                <strong>{editing ? '编辑市场来源' : '新增市场来源'}</strong>
                <span>{typeDescriptions[form.type]}</span>
              </div>
              <span className={editing?.ready ? 'marketBadge installed' : 'marketBadge'}>{editing?.ready ? '已保存' : '未保存'}</span>
            </div>
            <div className="nexusConfigGrid">
              <label>
                <span>来源 ID</span>
                <input value={form.id} disabled={Boolean(editing)} placeholder="nexus" onChange={event => updateField('id', event.target.value)} />
              </label>
              <label>
                <span>来源名称</span>
                <input value={form.name} placeholder={defaultName(form.type)} onChange={event => updateField('name', event.target.value)} />
              </label>
              <label>
                <span>来源类型</span>
                <select value={form.type} onChange={event => changeType(event.target.value)}>
                  <option value="nexus">Nexus 3 Raw</option>
                  <option value="remote">远程 index.json</option>
                  <option value="local">本地目录</option>
                </select>
              </label>
              <label>
                <span>{form.type === 'nexus' ? 'Nexus 地址' : form.type === 'remote' ? '索引地址' : '本地目录 / 索引文件'}</span>
                <input
                  value={form.url}
                  placeholder={form.type === 'nexus' ? 'http://nexus.example.com:8081' : form.type === 'remote' ? 'https://plugins.example.com/index.json' : 'market'}
                  onChange={event => updateField('url', event.target.value)}
                />
              </label>
              {form.type === 'nexus' && (
                <>
                  <label>
                    <span>Raw 仓库名</span>
                    <input value={form.repository} placeholder="ops-plugins" onChange={event => updateField('repository', event.target.value)} />
                  </label>
                  <label>
                    <span>索引路径</span>
                    <input value={form.index_path} placeholder="index.json" onChange={event => updateField('index_path', event.target.value)} />
                  </label>
                  <label>
                    <span>插件包目录</span>
                    <input value={form.package_dir} placeholder="packages" onChange={event => updateField('package_dir', event.target.value)} />
                  </label>
                </>
              )}
              {authFields}
            </div>
            <div className="nexusOptions">
              <label className="nexusCheck">
                <input type="checkbox" checked={form.enabled} onChange={event => updateField('enabled', event.target.checked)} />
                <span>在插件市场中展示此来源</span>
              </label>
              {form.type === 'nexus' && (
                <label className="nexusCheck">
                  <input type="checkbox" checked={form.insecure_skip_verify} onChange={event => updateField('insecure_skip_verify', event.target.checked)} />
                  <span>跳过 HTTPS 证书校验（自签证书）</span>
                </label>
              )}
            </div>
            <div className="nexusConfigActions">
              <button className="primary" type="button" disabled={saving || testing} onClick={saveSource}>{saving ? '保存中...' : '保存来源'}</button>
              <button className="secondary" type="button" disabled={saving || testing} onClick={testConnection}>{testing ? '测试中...' : '测试连接'}</button>
              {editing?.index_url && <span className="nexusIndexURL">{editing.index_url}</span>}
            </div>
            {testResult && (
              <div className={`nexusTestResult ${testResult.reachable ? 'ok' : 'error'}`}>
                <strong>{testResult.reachable ? '连接成功' : '连接失败'}</strong>
                <span>{testResult.message}</span>
                {testResult.reachable && (
                  <small>
                    类型：{typeLabels[testResult.type] || testResult.type} · 插件版本：{testResult.plugin_count || 0}
                    {testResult.location ? ` · ${testResult.location}` : ''}
                  </small>
                )}
              </div>
            )}
          </section>

          {editing?.type === 'nexus' && (
            <NexusRepositoryManager source={editing} onChanged={onChanged} />
          )}
        </>
      )}
    </div>
  )
}
