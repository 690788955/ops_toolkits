import React, {useEffect, useMemo, useState} from 'react'
import {deleteNexusPlugin, fetchNexusPlugins, nexusDownloadURL, postNexusPluginZip, rebuildNexusIndex} from './api.js'
import {readableAPIError} from './utils.js'

const marketStatusLabels = {
  not_installed: '未安装',
  installed: '已安装',
  update_available: '可更新',
  local_newer: '本地较新',
  disabled: '已禁用'
}

function formatBytes(value) {
  const size = Number(value || 0)
  if (size <= 0) return '-'
  if (size >= 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`
  if (size >= 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${size} B`
}

function shortSHA(value) {
  const text = String(value || '')
  return text.length > 16 ? `${text.slice(0, 16)}…` : text
}

export default function NexusRepositoryManager({source, onChanged}) {
  const [plugins, setPlugins] = useState([])
  const [pluginsLoading, setPluginsLoading] = useState(false)
  const [indexInfo, setIndexInfo] = useState({index_exists: false, generated_at: '', index_url: ''})
  const [file, setFile] = useState(null)
  const [fileInputKey, setFileInputKey] = useState(0)
  const [uploading, setUploading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState(null)
  const [duplicate, setDuplicate] = useState(false)
  const [rebuilding, setRebuilding] = useState(false)
  const [rebuildResult, setRebuildResult] = useState(null)
  const [deletingKey, setDeletingKey] = useState('')
  const [pluginSearch, setPluginSearch] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const ready = Boolean(source?.ready)
  const filteredPlugins = useMemo(() => {
    const keyword = pluginSearch.trim().toLowerCase()
    if (!keyword) return plugins
    return plugins.filter(item => [item.id, item.name, item.version, item.description]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
      .includes(keyword))
  }, [plugins, pluginSearch])

  useEffect(() => {
    setPlugins([])
    setIndexInfo({index_exists: false, generated_at: '', index_url: source?.index_url || ''})
    setRebuildResult(null)
    setMessage('')
    setError('')
    if (source?.ready) loadPlugins()
  }, [source?.id, source?.ready])

  async function loadPlugins() {
    if (!source?.id) return
    setPluginsLoading(true)
    setError('')
    try {
      const body = await fetchNexusPlugins(source.id)
      const data = body.data || {}
      setPlugins(data.plugins || [])
      setIndexInfo({
        index_exists: Boolean(data.index_exists),
        generated_at: data.generated_at || '',
        index_url: data.index_url || source?.index_url || ''
      })
    } catch (err) {
      setError(readableAPIError(err, '读取 Nexus 仓库插件失败。'))
    } finally {
      setPluginsLoading(false)
    }
  }

  async function uploadPlugin(replace = false) {
    if (!file) {
      setError('请先选择插件 ZIP 包。')
      return
    }
    setUploading(true)
    setUploadProgress(0)
    setError('')
    setDuplicate(false)
    setMessage(replace ? '正在覆盖 Nexus 插件包...' : '正在上传插件到 Nexus...')
    try {
      const body = await postNexusPluginZip(source.id, file, replace, progress => {
        if (progress.total > 0) setUploadProgress(Math.round(progress.loaded / progress.total * 100))
      })
      const data = body.data || {}
      setMessage(`插件 ${data.plugin_id || ''}@${data.version || ''} 已${body.status === 'replaced' ? '覆盖' : '上传'}到 Nexus（${formatBytes(data.size)}）。`)
      setFile(null)
      setFileInputKey(current => current + 1)
      setRebuildResult(null)
      await loadPlugins()
      onChanged?.()
    } catch (err) {
      if (err.status === 409 && err.body?.data?.existing) {
        setDuplicate(true)
        setMessage(`${err.message}\n确认后会覆盖仓库中同版本的插件包。`)
      } else {
        setError(readableAPIError(err, '上传插件到 Nexus 失败。'))
        setMessage('')
      }
    } finally {
      setUploading(false)
      setUploadProgress(null)
    }
  }

  async function removePlugin(item) {
    const key = `${item.id}@${item.version}`
    if (!window.confirm(`确定从 Nexus 仓库删除插件 ${item.name || item.id}@${item.version}？\n\n只会删除仓库中的插件包和索引记录，不影响本机已安装的插件。`)) return
    setDeletingKey(key)
    setError('')
    setMessage(`正在删除 Nexus 插件 ${item.id}@${item.version}...`)
    try {
      const body = await deleteNexusPlugin(source.id, item.id, item.version)
      const data = body.data || {}
      setMessage(`已从 Nexus 删除插件 ${item.id}@${item.version}${data.asset_deleted ? '' : '（包文件原本已不存在）'}。`)
      await loadPlugins()
      onChanged?.()
    } catch (err) {
      setError(readableAPIError(err, '删除 Nexus 插件失败。'))
      setMessage('')
    } finally {
      setDeletingKey('')
    }
  }

  async function rebuildIndex() {
    if (!window.confirm('将扫描 Nexus 仓库中的全部插件包并重写 index.json，是否继续？\n\n无法解析的插件包会被跳过。')) return
    setRebuilding(true)
    setError('')
    setRebuildResult(null)
    setMessage('正在扫描 Nexus 仓库并重建索引...')
    try {
      const body = await rebuildNexusIndex(source.id)
      const data = body.data || {}
      setRebuildResult(data)
      setMessage(`索引重建完成：扫描 ${data.scanned || 0} 个插件包，写入 ${data.indexed || 0} 个，跳过 ${data.skipped || 0} 个。`)
      await loadPlugins()
      onChanged?.()
    } catch (err) {
      setError(readableAPIError(err, '重建 Nexus 索引失败。'))
      setRebuildResult(err.body?.data || null)
      setMessage('')
    } finally {
      setRebuilding(false)
    }
  }

  function pluginRow(item) {
    const key = `${item.id}@${item.version}`
    return (
      <article className="nexusPluginRow" key={key}>
        <div>
          <div className="marketPluginTitle">
            <strong>{item.name || item.id}</strong>
            <span className="marketPluginVersion">v{item.version}</span>
            <span className={`marketBadge ${item.install_status}`}>{marketStatusLabels[item.install_status] || item.install_status}</span>
          </div>
          <span className="marketPluginID">{item.id} · {item.package?.url}</span>
          <div className="marketPluginMeta">
            <span>{formatBytes(item.package?.size)}</span>
            <span>SHA256 {shortSHA(item.package?.sha256)}</span>
            {item.installed && item.installed_version && <span>本地版本：{item.installed_version}</span>}
            {item.tool_count > 0 && <span>{item.tool_count} 个工具</span>}
            {item.workflow_count > 0 && <span>{item.workflow_count} 个工作流</span>}
          </div>
          {item.description && <small>{item.description}</small>}
          {item.compatible === false && <small className="marketIncompatible">{item.compatibility_message || '当前版本不兼容此插件。'}</small>}
        </div>
        <div className="nexusPluginActions">
          <a className="secondary" href={nexusDownloadURL(source.id, item.id, item.version)}>下载</a>
          <button className="secondary danger" type="button" disabled={deletingKey === key} onClick={() => removePlugin(item)}>
            {deletingKey === key ? '删除中...' : '删除'}
          </button>
        </div>
      </article>
    )
  }

  if (!ready) {
    return (
      <section className="nexusRepositoryCard">
        <div className="empty small">保存并启用此 Nexus 来源后，可在这里上传、下载和删除仓库插件。</div>
      </section>
    )
  }

  return (
    <section className="nexusRepositoryCard">
      <div className="pluginInstalledHeader">
        <div>
          <strong>仓库插件</strong>
          <span>
            {source.name || source.id} · {source.repository} · {plugins.length} 个插件版本 · {indexInfo.index_exists ? `索引更新于 ${indexInfo.generated_at || '-'}` : '索引尚未创建'}
          </span>
        </div>
        <div className="pluginInstalledHeaderActions">
          <button className="secondary" type="button" disabled={pluginsLoading || rebuilding} onClick={loadPlugins}>{pluginsLoading ? '刷新中...' : '刷新'}</button>
          <button className="secondary" type="button" disabled={pluginsLoading || rebuilding} onClick={rebuildIndex}>{rebuilding ? '重建中...' : '重建索引'}</button>
        </div>
      </div>
      <div className="nexusIndexURL">{indexInfo.index_url || source.index_url}</div>
      <div className="pluginInstalledToolbar">
        <input value={pluginSearch} onChange={event => setPluginSearch(event.target.value)} placeholder="搜索仓库插件名称、ID、版本..." aria-label="搜索仓库插件" />
        <span className="marketHint">{pluginSearch.trim() ? `匹配 ${filteredPlugins.length} / ${plugins.length} 个插件版本` : `共 ${plugins.length} 个插件版本`}</span>
      </div>
      <div className="nexusUploadRow">
        <label>
          <span>上传插件 ZIP</span>
          <input
            key={fileInputKey}
            type="file"
            accept=".zip,application/zip"
            onChange={event => {
              setFile(event.target.files?.[0] || null)
              setDuplicate(false)
              setMessage('已选择插件 ZIP，点击上传开始写入 Nexus。')
            }}
          />
        </label>
        <button className="primary" type="button" disabled={!file || uploading} onClick={() => uploadPlugin(false)}>
          {uploading ? (uploadProgress === null ? '上传中...' : uploadProgress >= 100 ? '服务端处理中...' : `上传中 ${uploadProgress}%`) : '上传到 Nexus'}
        </button>
        {duplicate && <button className="secondary" type="button" disabled={uploading} onClick={() => uploadPlugin(true)}>确认覆盖</button>}
      </div>
      <small className="marketHint">上传前会校验插件清单和工具配置；同名同版本插件需要确认后覆盖。插件市场页可直接安装这些插件。</small>
      {rebuildResult?.warnings?.length > 0 && (
        <div className="nexusWarnings">
          <strong>重建时跳过 {rebuildResult.warnings.length} 个插件包：</strong>
          {rebuildResult.warnings.map(item => <small key={item.path}>{item.path}：{item.error}</small>)}
        </div>
      )}
      {error && <div className="marketSourceError">{error}</div>}
      {message && <small className="nexusMessage">{message}</small>}
      <div className="nexusPluginList">
        {filteredPlugins.map(pluginRow)}
        {!pluginsLoading && plugins.length === 0 && (
          <div className="empty small">
            {indexInfo.index_exists ? '仓库索引中还没有插件，上传一个 ZIP 或点击“重建索引”即可。' : '仓库中还没有索引，上传插件或点击“重建索引”后自动创建。'}
          </div>
        )}
        {!pluginsLoading && plugins.length > 0 && filteredPlugins.length === 0 && (
          <div className="empty small">没有匹配的仓库插件。</div>
        )}
      </div>
    </section>
  )
}
