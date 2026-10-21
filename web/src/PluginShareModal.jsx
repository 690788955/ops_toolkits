import React, {useEffect, useMemo, useState} from 'react'
import {postPluginShare} from './api.js'
import {readableAPIError} from './utils.js'

function formatBytes(value) {
  const size = Number(value || 0)
  if (size <= 0) return '-'
  if (size >= 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`
  if (size >= 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${size} B`
}

function shortSHA(value) {
  const text = String(value || '')
  return text.length > 20 ? `${text.slice(0, 20)}…` : text
}

export default function PluginShareModal({plugin, targets, onClose, onShared}) {
  const availableTargets = useMemo(() => (targets || []).filter(item => item?.id), [targets])
  const [sourceID, setSourceID] = useState(availableTargets[0]?.id || '')
  const [replace, setReplace] = useState(false)
  const [sharing, setSharing] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [result, setResult] = useState(null)

  useEffect(() => {
    if (sourceID && availableTargets.some(item => item.id === sourceID)) return
    setSourceID(availableTargets[0]?.id || '')
  }, [availableTargets, sourceID])

  const target = availableTargets.find(item => item.id === sourceID) || availableTargets[0] || null
  const noTarget = !target

  async function share(overwrite = replace) {
    if (!plugin?.id || sharing) return
    setSharing(true)
    setError('')
    setMessage(replace || overwrite ? '正在覆盖远程插件市场中的同名版本...' : '正在上传插件到远程插件市场...')
    try {
      const body = await postPluginShare(plugin.id, target?.id || '', overwrite)
      const data = body.data || {}
      setResult(data)
      setReplace(false)
      setMessage(`插件 ${data.plugin_id || plugin.id}@${data.version || plugin.version || ''} 已${body.status === 'updated' ? '更新到' : '分享到'}「${data.source_name || data.source_id || target?.id || ''}」。`)
      onShared?.(body)
    } catch (err) {
      if (err.status === 409) {
        setReplace(true)
        setError(`${err.message}\n远程市场已存在同名同版本插件包，勾选“覆盖同名版本”后可重新分享。`)
      } else {
        setError(readableAPIError(err, '分享插件到远程插件市场失败。'))
      }
      setMessage('')
    } finally {
      setSharing(false)
    }
  }

  return (
    <div className="modalBackdrop modalBackdropNested" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal pluginOperationModal pluginShareModal" role="dialog" aria-modal="true" aria-labelledby="plugin-share-title">
        <div className="modalHeader">
          <div>
            <h3 id="plugin-share-title">分享插件</h3>
            <p>把本机插件打包后直接上传到远程插件市场（Nexus），其他环境即可在市场里安装。</p>
          </div>
          <button className="modalClose" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        <div className="pluginUploadForm">
          <div className="pluginShareTargetCard">
            <strong>{plugin?.name || plugin?.id}</strong>
            <span>{plugin?.id}@{plugin?.version || '-'}</span>
            {plugin?.description && <small>{plugin?.description}</small>}
          </div>
          {noTarget ? (
            <div className="pluginOperationMessage error">
              未配置可用的远程插件市场（Nexus），当前只能使用本地插件市场，无法分享插件。
              请先在「插件市场 → 来源配置」中新增并启用 Nexus 来源。
            </div>
          ) : (
            <>
              <label>
                <span>分享到</span>
                <select value={target.id} disabled={sharing || availableTargets.length <= 1} onChange={event => setSourceID(event.target.value)}>
                  {availableTargets.map(item => (
                    <option key={item.id} value={item.id}>
                      {item.name || item.id}（{item.repository || '仓库未命名'}）
                    </option>
                  ))}
                </select>
                <small className="pluginShareTargetHint">{target.index_url || `${target.url || ''}/${target.repository || ''}`}</small>
              </label>
              <label className="nexusCheck pluginShareCheck">
                <input type="checkbox" checked={replace} disabled={sharing} onChange={event => setReplace(event.target.checked)} />
                <span>覆盖远程市场中已存在的同名同版本插件包</span>
              </label>
            </>
          )}
          {error && <div className="pluginOperationMessage error">{error}</div>}
          {message && !error && <div className="pluginOperationMessage">{message}</div>}
          {result && (
            <div className="pluginShareResult">
              <small>插件包：{result.package_path || '-'}</small>
              <small>大小：{formatBytes(result.size)} · SHA256：{shortSHA(result.sha256)}</small>
              <small>索引已更新：{result.index_updated ? '是' : '否'}{result.warnings?.length ? ` · ${result.warnings.length} 条提醒` : ''}</small>
            </div>
          )}
          <div className="modalFooter pluginShareFooter">
            <button className="secondary" type="button" onClick={onClose}>{result ? '完成' : '取消'}</button>
            <button
              className="primary"
              type="button"
              disabled={noTarget || sharing}
              title={noTarget ? '未配置远程插件市场（Nexus），无法分享' : undefined}
              onClick={() => share()}
            >
              {sharing ? '分享中...' : replace ? '覆盖并分享' : '分享到远程市场'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
