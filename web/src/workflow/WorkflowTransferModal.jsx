import React, {useRef, useState} from 'react'
import {importWorkflows} from '../api.js'
import {readableAPIError} from '../utils.js'
import {useModalEscape} from '../useModalEscape.js'

export default function WorkflowTransferModal({onClose, onImported}) {
  const modalRef = useRef(null)
  const fileRef = useRef(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [messageError, setMessageError] = useState(false)
  const [result, setResult] = useState(null)

  useModalEscape(onClose, modalRef)

  async function importFile(event) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setBusy(true)
    setMessageError(false)
    setResult(null)
    setMessage(`正在导入 ${file.name}...`)
    try {
      const body = await importWorkflows(file)
      const data = body?.data || body || {}
      const imported = data.imported || []
      const failed = data.failed || []
      setResult({imported, failed})
      setMessageError(imported.length === 0 && failed.length > 0)
      setMessage(`导入完成：成功 ${imported.length} 个，失败 ${failed.length} 个。`)
      if (onImported && imported.length > 0) await onImported()
    } catch (err) {
      setMessage(readableAPIError(err, '导入工作流失败。'))
      setMessageError(true)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modalBackdrop modalBackdropNested" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal pluginOperationModal workflowExportModal" ref={modalRef} role="dialog" aria-modal="true" aria-labelledby="workflow-transfer-title">
        <div className="modalHeader">
          <div>
            <h3 id="workflow-transfer-title">导入工作流</h3>
            <p>选择 .yaml 或 .zip 文件导入；.zip 中附带的工作流会一并导入。</p>
          </div>
          <button className="modalClose" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        <div className="workflowExportPanel">
          <div className="workflowImportDropzone">
            <button type="button" className="primary" disabled={busy} onClick={() => fileRef.current?.click()}>
              {busy ? '正在导入...' : '选择文件…'}
            </button>
            <small>支持 .yaml / .yml / .zip；导入的工作流保存为全局工作流，ID 冲突时自动改名。</small>
            <input ref={fileRef} type="file" accept=".yaml,.yml,.zip" style={{display: 'none'}} onChange={importFile} />
          </div>
          {message && <div className={messageError ? 'pluginOperationMessage error' : 'pluginOperationMessage'}>{message}</div>}
          {result && (
            <div className="workflowImportResults">
              {result.imported.length > 0 && (
                <div className="workflowImportResultGroup">
                  <strong>已导入</strong>
                  <div className="workflowImportResultTags">
                    {result.imported.map(item => (
                      <span className="marketTag" key={item.new_id || item.id}>{item.id}{item.renamed ? ` → ${item.new_id}` : ''}</span>
                    ))}
                  </div>
                </div>
              )}
              {result.failed.length > 0 && (
                <div className="workflowImportResultGroup">
                  <strong>失败</strong>
                  <ul className="workflowImportResultFailures">
                    {result.failed.map(item => <li key={item.id}><code>{item.id}</code>：{item.error}</li>)}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}