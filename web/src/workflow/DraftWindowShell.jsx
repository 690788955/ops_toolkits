import React, { useCallback, useEffect, useState } from 'react'
import {fetchJSON} from '../api.js'
import {readableAPIError} from '../utils.js'
import WorkflowEditor from './WorkflowEditor.jsx'

const EMPTY_CATALOG = {tools: [], workflows: [], plugins: [], categories: []}

// 独立草稿窗口：只承载工作流画布，用于运行期间继续编辑草稿；运行入口仍在主窗口。
export default function DraftWindowShell() {
  const [catalog, setCatalog] = useState(EMPTY_CATALOG)
  const [hosts, setHosts] = useState([])
  const [result, setResult] = useState({message: '独立草稿窗口已就绪。'})
  const [error, setError] = useState('')

  const refreshCatalog = useCallback(async () => {
    const body = await fetchJSON('/api/catalog')
    const data = body.data || {}
    setCatalog({tools: data.tools || [], workflows: data.workflows || [], plugins: data.plugins || [], categories: data.categories || []})
    return data
  }, [])

  useEffect(() => {
    let ignore = false
    refreshCatalog().catch(err => {
      if (!ignore) setError(readableAPIError(err, '加载插件目录失败，草稿窗口可能无法列出工具。'))
    })
    return () => { ignore = true }
  }, [refreshCatalog])

  useEffect(() => {
    let ignore = false
    fetchJSON('/api/ssh/hosts')
      .then(body => { if (!ignore) setHosts(body.data?.hosts || []) })
      .catch(() => {})
    return () => { ignore = true }
  }, [])

  return (
    <div className="draftWindowPage">
      <header className="draftWindowHeader">
        <div>
          <strong>独立草稿窗口</strong>
          <span>编辑和保存操作机上的工作流草稿；本次运行继续使用主窗口启动时的快照，两边互不覆盖。</span>
        </div>
        <a className="secondary small" href={window.location.href.replace(/\?.*$/, '')}>回到主窗口</a>
      </header>
      {error && <div className="workflowEditorNotice" role="alert">{error}</div>}
      <WorkflowEditor
        catalog={catalog}
        hosts={hosts}
        setResult={setResult}
        refreshCatalog={refreshCatalog}
        draftWindow
      />
      <section className="draftWindowStatus" role="status" aria-live="polite">{result?.message || ''}</section>
    </div>
  )
}