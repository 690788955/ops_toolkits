import React, {useState} from 'react'
import FileUploadInput from '../FileUploadInput.jsx'
import {sshHostStatusLabel} from '../SshHostsModal.jsx'
import {conditionOperators, controlNodeCategory, controlNodeHelp, controlNodeTitle} from './catalog.js'
import {
  archiveFormatFromPath,
  archiveFormats,
  checksumAlgorithms,
  defaultArchiveConfig as modelDefaultArchiveConfig,
  defaultApprovalConfig as modelDefaultApprovalConfig,
  defaultChecksumConfig as modelDefaultChecksumConfig,
  defaultExtractConfig as modelDefaultExtractConfig,
  defaultCondition,
  defaultHttpRequest as modelDefaultHttpRequest,
  defaultLoop as modelDefaultLoop,
  defaultScriptConfig as modelDefaultScriptConfig,
  defaultNotifyConfig as modelDefaultNotifyConfig,
  defaultUpload as modelDefaultUpload,
  defaultWaitConfig as modelDefaultWaitConfig,
  defaultParams,
  formatHTTPStatusList,
  httpBodyTypes,
  httpMethods,
  normalizeArchiveConfig,
  normalizeApprovalConfig,
  normalizeChecksumConfig,
  normalizeExtractConfig,
  normalizeHttpBodyType,
  normalizeHttpRequestConfig,
  normalizeHTTPStatusList,
  normalizeLoopConfig,
  normalizeNotifyConfig,
  normalizeScriptConfig,
  normalizeUploadConfig,
  normalizeWaitConfig,
  notifyChannels,
  notifyDefaultPort,
  notifyTLSModes,
  splitMailRecipients,
  waitModes
} from './model.js'

function NodeConfigModal({node, kindLabel, children, onClose, onSave}) {
  return (
    <div className="modalBackdrop drawerBackdrop" onClick={onClose}>
      <aside className="nodeConfigDrawer" role="dialog" aria-modal="true" aria-label="节点配置" onClick={event => event.stopPropagation()}>
        <div className="modalHeader nodeConfigDrawerHeader">
          <div>
            <h3>节点配置</h3>
            <p>{kindLabel} · {node.data.name || node.id}</p>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        <div className="nodeConfigDrawerBody">{children}</div>
        <div className="modalFooter nodeConfigDrawerFooter">
          <button type="button" className="secondary" onClick={onClose}>取消</button>
          <button type="button" className="primary" onClick={onSave}>保存配置</button>
        </div>
      </aside>
    </div>
  )
}

function StageAssignmentEditor({node, stages = [], nodes = [], disabled = false, onStageChange, onStageMove}) {
  if (!stages.length) return null
  const stageId = node.data.stageId || ''
  const stageNodes = (nodes || [])
    .filter(item => (item.data?.stageId || '') === stageId)
    .sort((left, right) => (Number(left.data?.order) || 0) - (Number(right.data?.order) || 0))
  const index = stageNodes.findIndex(item => item.id === node.id)
  return (
    <div className="nodeStageAssignment">
      <label>
        <span>所属阶段</span>
        <select value={stageId} disabled={disabled} onChange={event => onStageChange?.(event.target.value)}>
          {stages.map(stage => (
            <option key={stage.id} value={stage.id}>{stage.name || stage.id}</option>
          ))}
        </select>
      </label>
      <div className="nodeStageOrder">
        <span>阶段内第 <strong>{index >= 0 ? index + 1 : '-'}</strong> / {stageNodes.length || 1} 步</span>
        <div className="buttonRow">
          <button type="button" className="secondary" disabled={disabled || index <= 0} onClick={() => onStageMove?.(-1)}>上移</button>
          <button type="button" className="secondary" disabled={disabled || index < 0 || index >= stageNodes.length - 1} onClick={() => onStageMove?.(1)}>下移</button>
        </div>
      </div>
      <small className="workflowSelectionHint">阶段归属和阶段内顺序会立即写回画布，其他配置保存后生效。</small>
    </div>
  )
}

function NodeConfigEditor({node, tool, loopTool, tools, nodes, stages = [], hosts = [], sources, paramsText, setParamsText, onNameChange, onConditionChange, onLoopChange, onUploadChange, onExtractConfigChange, onHttpChange, onScriptChange, onChecksumChange, onArchiveChange, onWaitChange, onNotifyChange, onApprovalChange, onParamChange, onLoopParamChange, onApplyParams, onHostChange, onStageChange, onStageMove, stageDisabled = false}) {
  if (!node) return null
  return (
    <div className="form nodeConfigEditor">
      <StageAssignmentEditor
        node={node}
        stages={stages}
        nodes={nodes}
        disabled={stageDisabled}
        onStageChange={onStageChange}
        onStageMove={onStageMove}
      />
      {node.type === 'conditionNode' ? (
        <ConditionEditor node={node} sources={sources} onNameChange={onNameChange} onChange={onConditionChange} />
      ) : node.type === 'controlNode' ? (
        <ControlNodeInspector node={node} tools={tools} nodes={nodes} hosts={hosts} loopTool={loopTool} sources={sources} onNameChange={onNameChange} onLoopChange={onLoopChange} onUploadChange={onUploadChange} onExtractConfigChange={onExtractConfigChange} onHttpChange={onHttpChange} onScriptChange={onScriptChange} onChecksumChange={onChecksumChange} onArchiveChange={onArchiveChange} onWaitChange={onWaitChange} onNotifyChange={onNotifyChange} onApprovalChange={onApprovalChange} onLoopParamChange={onLoopParamChange} onHostChange={onHostChange} />
      ) : (
        <ToolNodeConfigEditor
          node={node}
          tool={tool}
          hosts={hosts}
          sources={sources}
          paramsText={paramsText}
          setParamsText={setParamsText}
          onParamChange={onParamChange}
          onApplyParams={onApplyParams}
          onHostChange={onHostChange}
        />
      )}
    </div>
  )
}

function HostTargetEditor({node, hosts = [], onChange}) {
  const hostID = node.data.host || ''
  const selectedHost = (hosts || []).find(host => host.id === hostID) || null
  return (
    <label>
      <span>执行目标</span>
      <select value={hostID} onChange={event => onChange(event.target.value)}>
        <option value="">本机执行</option>
        {(hosts || []).map(host => (
          <option key={host.id} value={host.id} disabled={!host.enabled}>
            {host.name || host.id} · {host.user}@{host.address}:{host.port}（{sshHostStatusLabel(host.status)}{host.enabled ? '' : '，已停用'}）
          </option>
        ))}
      </select>
      {hostID && !selectedHost && <small className="targetWarning">所选操作机已不存在，请重新选择。</small>}
      {selectedHost && selectedHost.status !== 'online' && <small className="targetWarning">操作机当前{sshHostStatusLabel(selectedHost.status)}，运行时将实时建立连接。</small>}
      <small>选择操作机后，工具会随执行包通过 SSH 下发并在目标机运行；留空则在本机执行。</small>
    </label>
  )
}

function ToolNodeConfigEditor({node, tool, hosts, sources, paramsText, setParamsText, onParamChange, onApplyParams, onHostChange}) {
  return (
    <>
      <label>
        <span>节点标识</span>
        <input value={node.id} disabled />
      </label>
      <label>
        <span>工具标识</span>
        <input value={node.data.tool} disabled />
      </label>
      <HostTargetEditor node={node} hosts={hosts} onChange={onHostChange} />
      <ParamMappingEditor tool={tool} params={node.data.params || {}} sources={sources} onChange={onParamChange} />
      <ToolOutputsView tool={tool} />
      <details className="advancedParams">
        <summary>高级 JSON 编辑</summary>
        <label>
          <span>参数（JSON）</span>
          <textarea value={paramsText} onChange={event => setParamsText(event.target.value)} />
        </label>
        <div className="empty small">可引用上游节点：{"{{ .steps.节点ID.stdout }}"} 或 {"{{ .steps.节点ID.params.参数名 }}"}</div>
        <button type="button" className="secondary" onClick={() => onApplyParams()}>应用参数</button>
      </details>
    </>
  )
}

function EdgeConfigModal({edge, sourceNode, onCaseChange, onClose}) {
  return (
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal edgeConfigModal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>连线配置</h3>
            <p>{edge.source} → {edge.target}</p>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        <EdgeInspector edge={edge} sourceNode={sourceNode} onCaseChange={onCaseChange} />
        <div className="modalFooter">
          <button type="button" className="primary" onClick={onClose}>完成</button>
        </div>
      </div>
    </div>
  )
}

function EdgeInspector({edge, sourceNode, onCaseChange}) {
  if (sourceNode?.type !== 'conditionNode') {
    return <div className="empty small">已选择依赖线，可使用左侧工具栏“删除依赖”。</div>
  }
  const condition = sourceNode.data.condition || defaultCondition()
  const currentCase = edge.data?.case || edge.sourceHandle || ''
  const showDefaultOption = condition.default_case === 'default' || currentCase === 'default'
  return (
    <div className="edgeInspector">
      <strong>条件分支连线</strong>
      <select value={currentCase} onChange={event => onCaseChange(event.target.value)}>
        <option value="">请选择 case...</option>
        {(condition.cases || []).map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}
        {showDefaultOption && <option value="default">默认分支 default</option>}
      </select>
      <div className="empty small">从条件节点发出的连线必须选择 case，标签会显示在连线上。</div>
    </div>
  )
}

function ConditionEditor({node, sources, onNameChange, onChange}) {
  const condition = node.data.condition || defaultCondition()
  function updateCase(index, patch) {
    const cases = (condition.cases || []).map((item, current) => current === index ? {...item, ...patch} : item)
    onChange({...condition, cases})
  }
  function addCase() {
    const index = (condition.cases || []).length + 1
    onChange({...condition, cases: [...(condition.cases || []), {id: `case_${index}`, name: `分支 ${index}`, operator: 'contains', values: ['']}]})
  }
  function removeCase(index) {
    onChange({...condition, cases: (condition.cases || []).filter((_, current) => current !== index)})
  }
  return (
    <div className="conditionEditor">
      <label>
        <span>节点标识</span>
        <input value={node.id} disabled />
      </label>
      <label>
        <span>显示名称</span>
        <input value={node.data.name || ''} placeholder="按巡检结果分支" onChange={event => onNameChange(event.target.value)} />
      </label>
      <label>
        <span>条件输入</span>
        <select value={condition.input || ''} onChange={event => onChange({...condition, input: event.target.value})}>
          <option value="">选择上游输出或工作流参数...</option>
          {sources.map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
        </select>
        <input value={condition.input || ''} placeholder="{{ .steps.inspect.stdout }}" onChange={event => onChange({...condition, input: event.target.value})} />
      </label>
      <label>
        <span>默认分支</span>
        <select value={condition.default_case || ''} onChange={event => onChange({...condition, default_case: event.target.value})}>
          <option value="default">启用 default 分支</option>
          <option value="">不启用</option>
        </select>
      </label>
      <div className="caseList">
        <strong>Case 分支</strong>
        {(condition.cases || []).map((item, index) => (
          <div className="caseEditor" key={`${item.id}-${index}`}>
            <input value={item.id || ''} placeholder="case_id" onChange={event => updateCase(index, {id: event.target.value})} />
            <input value={item.name || ''} placeholder="分支名称" onChange={event => updateCase(index, {name: event.target.value})} />
            <select value={item.operator || 'contains'} onChange={event => updateCase(index, {operator: event.target.value})}>
              {conditionOperators.map(operator => <option key={operator.value} value={operator.value}>{operator.label}</option>)}
            </select>
            <textarea className="smallTextarea" value={(item.values || []).join('\n')} placeholder="匹配值；多个值可换行或用逗号分隔" onChange={event => updateCase(index, {values: updateCaseValuesText(event.target.value)})} />
            <button className="secondary danger" onClick={() => removeCase(index)}>删除 case</button>
          </div>
        ))}
        <button className="secondary" onClick={addCase}>添加 case</button>
      </div>
    </div>
  )
}

function ControlNodeInspector({node, tools, nodes, hosts, loopTool, sources, onNameChange, onLoopChange, onUploadChange, onExtractConfigChange, onHttpChange, onScriptChange, onChecksumChange, onArchiveChange, onWaitChange, onNotifyChange, onApprovalChange, onLoopParamChange, onHostChange}) {
  const isLoop = node.data.controlType === 'loop'
  const loop = normalizeLoopConfig(node.data.loop || modelDefaultLoop())
  const isUpload = node.data.controlType === 'upload'
  const isExtractConfig = node.data.controlType === 'extract_config'
  const isHTTP = node.data.controlType === 'http_request'
  const isScript = node.data.controlType === 'shell_script'
  const isChecksum = node.data.controlType === 'file_checksum'
  const isArchive = node.data.controlType === 'archive'
  const isWait = node.data.controlType === 'wait'
  const isNotify = node.data.controlType === 'notify'
  const isApproval = node.data.controlType === 'approval'
  const category = controlNodeCategory(node.data.controlType)
  const upload = normalizeUploadConfig(node.data.upload || modelDefaultUpload())
  const extract = normalizeExtractConfig(node.data.extract_config || modelDefaultExtractConfig())
  const httpRequest = normalizeHttpRequestConfig(node.data.http || modelDefaultHttpRequest())
  const script = normalizeScriptConfig(node.data.script || modelDefaultScriptConfig())
  const checksum = normalizeChecksumConfig(node.data.checksum || modelDefaultChecksumConfig())
  const archive = normalizeArchiveConfig(node.data.archive || modelDefaultArchiveConfig())
  const wait = normalizeWaitConfig(node.data.wait || modelDefaultWaitConfig())
  const notify = normalizeNotifyConfig(node.data.notify || modelDefaultNotifyConfig())
  const approval = normalizeApprovalConfig(node.data.approval || modelDefaultApprovalConfig())
  function updateLoop(patch) {
    onLoopChange(normalizeLoopConfig({...loop, ...patch}))
  }
  function updateExtract(patch) {
    onExtractConfigChange(normalizeExtractConfig({...extract, ...patch}))
  }
  function updateHttp(patch) {
    onHttpChange(normalizeHttpRequestConfig({...httpRequest, ...patch}))
  }
  function updateScript(patch) {
    onScriptChange(normalizeScriptConfig({...script, ...patch}))
  }
  function updateChecksum(patch) {
    onChecksumChange(normalizeChecksumConfig({...checksum, ...patch}))
  }
  function updateArchive(patch) {
    onArchiveChange(normalizeArchiveConfig({...archive, ...patch}))
  }
  function updateWait(patch) {
    onWaitChange(normalizeWaitConfig({...wait, ...patch}))
  }
  function updateNotify(patch) {
    onNotifyChange(normalizeNotifyConfig({...notify, ...patch}))
  }
  function updateApproval(patch) {
    onApprovalChange(normalizeApprovalConfig({...approval, ...patch}))
  }
  function changeLoopTool(toolID) {
    const tool = (tools || []).find(item => item.id === toolID)
    updateLoop({tool: toolID, params: defaultParams(tool?.parameters || [])})
  }
  return (
    <div className="controlNodeInspector">
      <label>
        <span>节点标识</span>
        <input value={node.id} disabled />
      </label>
      <label>
        <span>节点类型</span>
        <input value={`${category.label} · ${controlNodeTitle(node.data.controlType)}`} disabled />
      </label>
      <label>
        <span>显示名称</span>
        <input value={node.data.name || ''} placeholder={controlNodeTitle(node.data.controlType)} onChange={event => onNameChange(event.target.value)} />
      </label>
      {isLoop ? (
        <>
          <label>
            <span>循环工具</span>
            <select value={loop.tool || ''} onChange={event => changeLoopTool(event.target.value)}>
              <option value="">选择要重复执行的插件工具...</option>
              {(tools || []).map(tool => <option key={tool.id} value={tool.id}>{tool.name || tool.id}（{tool.id}）</option>)}
            </select>
          </label>
          <label>
            <span>最大循环次数</span>
            <input type="number" min="1" max="20" value={loop.max_iterations || 1} onChange={event => updateLoop({max_iterations: event.target.value})} />
          </label>
          <HostTargetEditor node={node} hosts={hosts} onChange={onHostChange} />
          <ParamMappingEditor tool={loopTool} params={loop.params || {}} sources={sources} onChange={onLoopParamChange} emptyMessage="请先选择循环工具，再配置该工具的输入参数。" />
          <div className="empty small">循环节点会重复执行上方选择的工具，不需要在画布上额外添加目标工具节点；普通连线仍保持 DAG。</div>
        </>
      ) : isUpload ? (
        <>
          <label>
            <span>目标目录</span>
            <input value={upload.target_dir || ''} placeholder="可留空；如 assets/release 或 /opt/app/release" onChange={event => onUploadChange({...upload, target_dir: event.target.value})} />
          </label>
          <div className="empty small">相对路径会写到平台 runs/uploads 下；绝对路径需要在「平台设置 → 上传目录白名单」里配置过，且文件会直接放入该目录。</div>
          <div className="empty small">运行前在执行面板选择本地文件、多个文件或目录；如需把上传包中的配置文件沉淀为工作流配置，请在该节点后连接“提取配置”节点。</div>
        </>
      ) : isExtractConfig ? (
        <>
          <label>
            <span>来源类型</span>
            <select value={extract.source_type || 'file'} onChange={event => updateExtract({source_type: event.target.value})}>
              <option value="file">文件</option>
              <option value="directory">目录</option>
            </select>
          </label>
          {extract.source_type === 'directory' ? (
            <DirectoryExtractEditor extract={extract} sources={sources} onChange={updateExtract} />
          ) : (
            <FileExtractEditor extract={extract} sources={sources} onChange={updateExtract} />
          )}
          <div className="empty small">建议把“提取配置”接在“上传文件”节点之后；源文件可选上传节点输出，也可手动填写上传目录内的相对路径。目标配置路径始终是工作流配置中心内的相对路径。</div>
        </>
      ) : isHTTP ? (
        <HttpRequestEditor http={httpRequest} sources={sources} onChange={updateHttp} />
      ) : isScript ? (
        <>
          <HostTargetEditor node={node} hosts={hosts} onChange={onHostChange} />
          <ScriptEditor script={script} sources={sources} onChange={updateScript} />
        </>
      ) : isChecksum ? (
        <>
          <HostTargetEditor node={node} hosts={hosts} onChange={onHostChange} />
          <ChecksumEditor checksum={checksum} sources={sources} onChange={updateChecksum} />
        </>
      ) : isArchive ? (
        <ArchiveEditor archive={archive} sources={sources} onChange={updateArchive} />
      ) : isWait ? (
        <WaitEditor wait={wait} onChange={updateWait} />
      ) : isNotify ? (
        <NotifyEditor notify={notify} onChange={updateNotify} />
      ) : isApproval ? (
        <ApprovalEditor approval={approval} onChange={updateApproval} />
      ) : (
        <div className="empty small">{controlNodeHelp(node.data.controlType)}。该节点不需要配置工具或参数，运行时自身记录为成功。</div>
      )}
    </div>
  )
}

function HttpRequestEditor({http, sources = [], onChange}) {
  const headers = http.headers || []
  function updateHeader(index, patch) {
    onChange({headers: headers.map((item, current) => current === index ? {...item, ...patch} : item)})
  }
  function removeHeader(index) {
    onChange({headers: headers.filter((item, current) => current !== index)})
  }
  function addHeader() {
    onChange({headers: [...headers, {name: '', value: ''}]})
  }
  const bodyDisabled = http.body_type === 'none'
  const downstreamHint = (sources || []).find(source => source.value)?.value || '{{ .steps.节点ID.stdout }}'
  return (
    <>
      <div className="httpRequestLine">
        <label className="httpMethodField">
          <span>方法</span>
          <select value={http.method} onChange={event => onChange({method: event.target.value})}>
            {httpMethods.map(method => <option key={method} value={method}>{method}</option>)}
          </select>
        </label>
        <label className="httpUrlField">
          <span>请求 URL</span>
          <input value={http.url} placeholder="https://api.example.com/deploy 或 {{ .base_url }}/health" onChange={event => onChange({url: event.target.value})} />
        </label>
      </div>
      <div className="empty small">URL、Header 与 Body 都支持运行期模板，例如 {downstreamHint}；内置 HTTP 节点在本机发起请求，不受操作机设置影响。</div>

      <div className="httpSectionHead">
        <span>请求 Header</span>
        <button type="button" className="secondary small" onClick={addHeader}>添加 Header</button>
      </div>
      {headers.length === 0 && <div className="empty small">没有自定义 Header；JSON 请求体默认补 Content-Type: application/json。</div>}
      {headers.map((item, index) => (
        <div className="httpHeaderRow" key={`header-${index}`}>
          <input value={item.name} placeholder="Authorization" onChange={event => updateHeader(index, {name: event.target.value})} />
          <input value={item.value} placeholder="Bearer {{ .token }}" onChange={event => updateHeader(index, {value: event.target.value})} />
          <button type="button" className="secondary danger small" onClick={() => removeHeader(index)}>删除</button>
        </div>
      ))}

      <label>
        <span>请求体类型</span>
        <select value={http.body_type} onChange={event => {
          const nextType = event.target.value
          onChange({
            body_type: nextType,
            method: nextType !== 'none' && http.method === 'GET' ? 'POST' : http.method
          })
        }}>
          {httpBodyTypes.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
      </label>
      {!bodyDisabled && (
        <label>
          <span>请求体</span>
          <textarea className="smallTextarea" value={http.body} placeholder={`{"env":"{{ .env }}"}`} onChange={event => onChange({body: event.target.value, body_type: normalizeHttpBodyType(http.body_type, event.target.value)})} />
        </label>
      )}
      {!bodyDisabled && http.method === 'GET' && <small className="targetWarning">GET 请求不能携带请求体，请改用 POST/PUT/PATCH。</small>}

      <label>
        <span>期望状态码</span>
        <input value={formatHTTPStatusList(http.expect_status)} placeholder="留空表示 2xx，例如 200, 201" onChange={event => onChange({expect_status: normalizeHTTPStatusList(event.target.value)})} />
      </label>
      <label className="checkboxLabel">
        <input type="checkbox" checked={http.fail_on_error !== false} onChange={event => onChange({fail_on_error: event.target.checked})} />
        <span>状态码不符时判定节点失败</span>
      </label>
      <label>
        <span>响应保存路径</span>
        <input value={http.save_response} placeholder="可留空，例如 response/deploy.json" onChange={event => onChange({save_response: event.target.value})} />
      </label>
      <label>
        <span>请求超时</span>
        <input value={http.timeout} placeholder="可留空，默认 30s，例如 10s、2m" onChange={event => onChange({timeout: event.target.value})} />
      </label>
      <div className="empty small">响应体写入节点 stdout，可在下游用 {"{{ .steps.节点ID.stdout }}"} 引用；状态码与耗时可用 {"{{ .steps.节点ID.http.status }}"}、{"{{ .steps.节点ID.http.duration_ms }}"} 引用。</div>
    </>
  )
}

function ScriptEditor({script, sources = [], onChange}) {
  const env = script.env || []
  const downstreamHint = (sources || []).find(source => source.value)?.value || '{{ .steps.节点ID.stdout }}'
  function updateEnvRow(index, patch) {
    onChange({env: env.map((item, current) => current === index ? {...item, ...patch} : item)})
  }
  function addEnvRow() {
    onChange({env: [...env, {name: '', value: ''}]})
  }
  function removeEnvRow(index) {
    onChange({env: env.filter((_, current) => current !== index)})
  }
  return (
    <>
      <label>
        <span>脚本内容</span>
        <textarea className="smallTextarea codeTextarea" value={script.script} placeholder={"set -eu\necho \"deploy {{ .env }}\""} onChange={event => onChange({script: event.target.value})} />
      </label>
      <div className="empty small">脚本在所选执行目标上通过 bash 运行，支持模板变量，例如 {downstreamHint}；输出写入节点 stdout，可直接被下游引用。</div>
      <label>
        <span>工作目录</span>
        <input value={script.workdir} placeholder="可留空，默认执行目录；支持 {{ .path }}" onChange={event => onChange({workdir: event.target.value})} />
      </label>
      <label>
        <span>超时</span>
        <input value={script.timeout} placeholder="可留空，默认 30m，例如 30s、2m" onChange={event => onChange({timeout: event.target.value})} />
      </label>
      <label className="checkboxLabel">
        <input type="checkbox" checked={script.fail_on_exit !== false} onChange={event => onChange({fail_on_exit: event.target.checked})} />
        <span>非 0 退出码判定节点失败</span>
      </label>
      <div className="httpSectionHead">
        <span>环境变量</span>
        <button type="button" className="secondary small" onClick={addEnvRow}>添加变量</button>
      </div>
      {env.length === 0 && <div className="empty small">没有额外环境变量。</div>}
      {env.map((item, index) => (
        <div className="httpHeaderRow" key={`env-${index}`}>
          <input value={item.name} placeholder="ENV_NAME" onChange={event => updateEnvRow(index, {name: event.target.value})} />
          <input value={item.value} placeholder="{{ .env }}" onChange={event => updateEnvRow(index, {value: event.target.value})} />
          <button type="button" className="secondary danger small" onClick={() => removeEnvRow(index)}>删除</button>
        </div>
      ))}
    </>
  )
}

function ChecksumEditor({checksum, sources = [], onChange}) {
  return (
    <>
      <label>
        <span>文件路径</span>
        <select value={(sources || []).some(source => source.value === checksum.path) ? checksum.path : ''} onChange={event => onChange({path: event.target.value})}>
          <option value="">手动输入文件路径</option>
          {(sources || []).map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
        </select>
        <input value={checksum.path} placeholder="/tmp/release.tar.gz 或 {{ .package_path }}" onChange={event => onChange({path: event.target.value})} />
      </label>
      <label>
        <span>算法</span>
        <select value={checksum.algorithm} onChange={event => onChange({algorithm: event.target.value, expected: ''})}>
          {checksumAlgorithms.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
      </label>
      <label>
        <span>期望摘要</span>
        <input value={checksum.expected} placeholder={checksum.algorithm === 'md5' ? '可留空，32 位十六进制' : '可留空，64 位十六进制'} onChange={event => onChange({expected: event.target.value})} />
      </label>
      <div className="empty small">留空只记录摘要；填写后摘要不一致会判定节点失败。结果写入 checksum.json，可用 {"{{ .steps.节点ID.stdout }}"} 引用实际摘要。</div>
    </>
  )
}

function ArchiveEditor({archive, sources = [], onChange}) {
  const detected = archiveFormatFromPath(archive.path)
  return (
    <>
      <label>
        <span>压缩包路径</span>
        <select value={(sources || []).some(source => source.value === archive.path) ? archive.path : ''} onChange={event => onChange({path: event.target.value})}>
          <option value="">手动输入压缩包路径</option>
          {(sources || []).map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
        </select>
        <input value={archive.path} placeholder="/tmp/release.tar.gz 或 {{ .package_path }}" onChange={event => onChange({path: event.target.value})} />
      </label>
      <label>
        <span>目标目录</span>
        <input value={archive.target_dir} placeholder="/opt/app/release 或 {{ .deploy_dir }}" onChange={event => onChange({target_dir: event.target.value})} />
      </label>
      <label>
        <span>压缩格式</span>
        <select value={archive.format} onChange={event => onChange({format: event.target.value})}>
          {archiveFormats.map(item => <option key={item.value || 'auto'} value={item.value}>{item.value ? item.label : detected ? `自动识别（${detected}）` : item.label}</option>)}
        </select>
      </label>
      <label>
        <span>去掉顶层目录层数</span>
        <input type="number" min="0" max="6" value={archive.strip_components} onChange={event => onChange({strip_components: event.target.value})} />
      </label>
      <label className="checkboxLabel">
        <input type="checkbox" checked={archive.overwrite} onChange={event => onChange({overwrite: event.target.checked})} />
        <span>覆盖已存在的同名文件</span>
      </label>
      <div className="empty small">解压在本机执行；压缩包中包含绝对路径或 .. 的条目会被拒绝。解压清单写入 archive.json 与节点 stdout。</div>
    </>
  )
}

function WaitEditor({wait, onChange}) {
  const polling = wait.mode === 'poll'
  return (
    <>
      <label>
        <span>等待方式</span>
        <select value={wait.mode} onChange={event => onChange({mode: event.target.value})}>
          {waitModes.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
      </label>
      {polling ? (
        <>
          <label>
            <span>轮询 URL</span>
            <input value={wait.poll_url} placeholder="https://api.example.com/health 或留空改用文件" onChange={event => onChange({poll_url: event.target.value})} />
          </label>
          <label>
            <span>轮询文件</span>
            <input value={wait.poll_file} placeholder="/tmp/deploy.done 或 {{ .steps.notify.stdout }}" onChange={event => onChange({poll_file: event.target.value})} />
          </label>
          {wait.poll_url && (
            <label>
              <span>期望状态码</span>
              <input value={formatHTTPStatusList(wait.expect_status)} placeholder="留空表示 2xx" onChange={event => onChange({expect_status: normalizeHTTPStatusList(event.target.value)})} />
            </label>
          )}
          <label>
            <span>轮询间隔</span>
            <input value={wait.interval} placeholder="可留空，默认 5s" onChange={event => onChange({interval: event.target.value})} />
          </label>
          <label>
            <span>最大尝试次数</span>
            <input type="number" min="1" value={wait.max_attempts || ''} placeholder="可留空" onChange={event => onChange({max_attempts: event.target.value})} />
          </label>
          <label>
            <span>最大等待时长</span>
            <input value={wait.max_wait} placeholder="可留空，默认 5m" onChange={event => onChange({max_wait: event.target.value})} />
          </label>
          <div className="empty small">URL 与文件只能填写一个；尝试次数与等待时长同时生效，哪个先到就结束。运行取消会立即中止等待。</div>
        </>
      ) : (
        <>
          <label>
            <span>等待秒数</span>
            <input type="number" min="1" max="3600" step="0.5" value={wait.seconds || ''} onChange={event => onChange({seconds: event.target.value})} />
          </label>
          <div className="empty small">固定等待期间可以随时取消运行；最长 3600 秒。</div>
        </>
      )}
    </>
  )
}

function ApprovalEditor({approval, onChange}) {
  return (
    <>
      <label>
        <span>审批提示</span>
        <textarea
          className="smallTextarea"
          value={approval.message}
          placeholder="可留空，例如：确认配置文件已改好后继续；支持 {{ .参数 }} 运行期模板"
          onChange={event => onChange({message: event.target.value})}
        />
      </label>
      <div className="empty small">这是停机点：运行到这里会中断流水线，运行状态保持 running，直到有人在运行日志里点确认才继续；提示文案支持 {`{{ .参数 }}`} 与上游步骤输出。</div>
      <label>
        <span>等待超时</span>
        <input value={approval.timeout} placeholder="可留空=一直等待，例如 30m、2h" onChange={event => onChange({timeout: event.target.value})} />
      </label>
      <div className="empty small">确认与驳回的入口在运行日志面板的审批条上，节点配置里没有动作按钮；超时未处理会按失败结束本次运行。</div>
    </>
  )
}

function NotifyEditor({notify, onChange}) {
  const headers = notify.headers || []
  const email = notify.channel === 'email'
  const templateHint = '支持运行期模板，例如 {{ .steps.节点ID.stdout }}'
  function updateHeader(index, patch) {
    onChange({headers: headers.map((item, current) => current === index ? {...item, ...patch} : item)})
  }
  return (
    <>
      <label>
        <span>通知通道</span>
        <select value={notify.channel} onChange={event => onChange({channel: event.target.value})}>
          {notifyChannels.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
      </label>
      <label>
        <span>通知标题</span>
        <input value={notify.title} placeholder="可留空，例如 部署完成 {{ .env }}" onChange={event => onChange({title: event.target.value})} />
      </label>
      <label>
        <span>通知正文</span>
        <textarea className="smallTextarea" value={notify.message} placeholder={email ? '邮件正文，可写多行' : '消息内容，建议引用上游输出摘要'} onChange={event => onChange({message: event.target.value})} />
      </label>
      {email ? (
        <>
          <div className="httpRequestLine">
            <label>
              <span>SMTP 服务器</span>
              <input value={notify.smtp_host} placeholder="smtp.example.com" onChange={event => onChange({smtp_host: event.target.value})} />
            </label>
            <label>
              <span>端口</span>
              <input type="number" min="1" max="65535" value={notify.smtp_port || ''} placeholder={String(notifyDefaultPort(notify.smtp_tls))} onChange={event => onChange({smtp_port: event.target.value})} />
            </label>
          </div>
          <label>
            <span>加密方式</span>
            <select value={notify.smtp_tls} onChange={event => {
              const smtp_tls = event.target.value
              onChange({smtp_tls, smtp_port: notify.smtp_port && notify.smtp_port === notifyDefaultPort(notify.smtp_tls) ? '' : notify.smtp_port})
            }}>
              {notifyTLSModes.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
            </select>
          </label>
          <label>
            <span>SMTP 用户名</span>
            <input value={notify.smtp_user} placeholder="ops@example.com，可留空表示匿名中继" onChange={event => onChange({smtp_user: event.target.value})} />
          </label>
          <label>
            <span>SMTP 密码 / 授权码</span>
            <input type="password" value={notify.smtp_pass} placeholder="会以明文写入工作流 YAML，注意共享范围" onChange={event => onChange({smtp_pass: event.target.value})} />
          </label>
          <label>
            <span>发件人</span>
            <input value={notify.from} placeholder="留空时使用 SMTP 用户名" onChange={event => onChange({from: event.target.value})} />
          </label>
          <label>
            <span>收件人</span>
            <input value={notify.to} placeholder="多人用逗号分隔：oncall@example.com, dev@example.com" onChange={event => onChange({to: event.target.value})} />
          </label>
          <label>
            <span>邮件主题</span>
            <input value={notify.subject} placeholder="留空时使用通知标题" onChange={event => onChange({subject: event.target.value})} />
          </label>
          <div className="empty small">收件人、主题和正文{templateHint}；SMTP 凭据按字面量保存，不会渲染模板。当前已识别 {splitMailRecipients(notify.to).length} 个收件人。</div>
        </>
      ) : (
        <>
          <label>
            <span>{notify.channel === 'wecom' ? '企业微信机器人 Webhook' : 'Webhook 地址'}</span>
            <input value={notify.webhook} placeholder={notify.channel === 'wecom' ? 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...' : 'https://hooks.example.com/ops'} onChange={event => onChange({webhook: event.target.value})} />
          </label>
          <div className="httpSectionHead">
            <span>请求 Header</span>
            <button type="button" className="secondary small" onClick={() => onChange({headers: [...headers, {name: '', value: ''}]})}>添加 Header</button>
          </div>
          {headers.length === 0 && <div className="empty small">没有自定义 Header；默认发送 JSON。</div>}
          {headers.map((item, index) => (
            <div className="httpHeaderRow" key={`notify-header-${index}`}>
              <input value={item.name} placeholder="Authorization" onChange={event => updateHeader(index, {name: event.target.value})} />
              <input value={item.value} placeholder="Bearer {{ .token }}" onChange={event => updateHeader(index, {value: event.target.value})} />
              <button type="button" className="secondary danger small" onClick={() => onChange({headers: headers.filter((item, current) => current !== index)})}>删除</button>
            </div>
          ))}
          <div className="empty small">
            {notify.channel === 'wecom'
              ? '企业微信通道按 markdown 消息发送，返回 errcode 非 0 时节点失败。'
              : <>POST JSON：{"title、message、text（标题+正文）与 timestamp"}；Header 值{templateHint}。</>}
          </div>
        </>
      )}
      <label>
        <span>发送超时</span>
        <input value={notify.timeout} placeholder="可留空，默认 10s" onChange={event => onChange({timeout: event.target.value})} />
      </label>
      <div className="empty small">通知失败会让该节点失败，可在运行详情中看到错误；需要时把节点放在流程末尾，避免阻断后续变更。</div>
    </>
  )
}

function FileExtractEditor({extract, sources, onChange}) {
  return (
    <>
      <label>
        <span>源文件来源</span>
        <select value={sources.some(source => source.value === extract.file_name) ? extract.file_name : ''} onChange={event => onChange({file_name: event.target.value})}>
              <option value="">手动输入上传文件名或相对路径</option>
              {sources.map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
        </select>
        <input value={extract.file_name || ''} placeholder="app.yaml、conf/app.yaml 或 {{ .steps.upload.file.relative_path }}" onChange={event => onChange({file_name: event.target.value})} />
      </label>
      <label>
        <span>目标配置路径</span>
        <input value={extract.target_path || ''} placeholder="app.yaml 或 conf/app.yaml" onChange={event => onChange({target_path: event.target.value})} />
      </label>
      <label>
        <span>显示标签</span>
        <input value={extract.label || ''} placeholder="应用配置" onChange={event => onChange({label: event.target.value})} />
      </label>
      <label className="checkboxLabel">
        <input type="checkbox" checked={extract.replace === true} onChange={event => onChange({replace: event.target.checked})} />
            <span>覆盖同名配置文件</span>
      </label>
    </>
  )
}

function DirectoryExtractEditor({extract, sources, onChange}) {
  const files = Array.isArray(extract.files) ? extract.files : []
  function updateFile(index, patch) {
    const nextFiles = files.map((item, current) => current === index ? {...item, ...patch} : item)
    onChange({files: nextFiles})
  }
  function addFile() {
    onChange({files: [...files, {source_path: '', target_path: '', label: '', replace: false}]})
  }
  function removeFile(index) {
    onChange({files: files.filter((_, current) => current !== index)})
  }
  return (
    <>
      <label>
        <span>源目录来源</span>
        <select value={sources.some(source => source.value === extract.source_dir) ? extract.source_dir : ''} onChange={event => onChange({source_dir: event.target.value})}>
          <option value="">手动输入上传目录来源</option>
          {sources.map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
        </select>
        <input value={extract.source_dir || ''} placeholder="{{ .steps.upload.file.relative_dir }}" onChange={event => onChange({source_dir: event.target.value})} />
      </label>
      <div className="caseList">
        <strong>目录下文件映射</strong>
        {files.map((file, index) => (
          <div className="caseEditor" key={`${file.source_path || 'file'}-${index}`}>
            <input value={file.source_path || ''} placeholder="conf/app.yaml" onChange={event => updateFile(index, {source_path: event.target.value, target_path: event.target.value})} />
            <input value={file.target_path || file.source_path || ''} placeholder="默认跟随源相对路径" onChange={event => updateFile(index, {target_path: event.target.value})} />
            <input value={file.label || ''} placeholder="应用配置" onChange={event => updateFile(index, {label: event.target.value})} />
            <label className="checkboxLabel">
              <input type="checkbox" checked={file.replace === true} onChange={event => updateFile(index, {replace: event.target.checked})} />
              <span>覆盖</span>
            </label>
            <button className="secondary danger" onClick={() => removeFile(index)}>删除</button>
          </div>
        ))}
        <button className="secondary" onClick={addFile}>添加目录文件</button>
      </div>
    </>
  )
}

function WorkflowTagsEditor({tags, availableTags, onChange}) {
  const [draftTag, setDraftTag] = useState('')
  function addTag(value) {
    const tag = String(value || '').trim()
    if (!tag) return
    onChange([...(tags || []), tag])
    setDraftTag('')
  }
  function removeTag(tag) {
    onChange((tags || []).filter(item => item !== tag))
  }
  const candidates = (availableTags || []).filter(tag => !(tags || []).includes(tag))
  return (
    <div className="workflowTagsEditor">
      <span>标签</span>
      <div className="tagList editable">
        {(tags || []).map(tag => (
          <button key={tag} type="button" className="tagChip active" onClick={() => removeTag(tag)} title="点击移除标签">{tag} ×</button>
        ))}
        {(tags || []).length === 0 && <small>暂无标签，可选择已有标签或输入新标签。</small>}
      </div>
      <div className="tagFilters selectableTags">
        {candidates.map(tag => <button key={tag} type="button" className="tagChip" onClick={() => addTag(tag)}>{tag}</button>)}
      </div>
      <div className="tagInputRow">
        <input value={draftTag} placeholder="输入新标签，回车添加" onChange={event => setDraftTag(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); addTag(draftTag) } }} />
        <button type="button" className="secondary" onClick={() => addTag(draftTag)}>添加标签</button>
      </div>
    </div>
  )
}

function ValidationSummary({status}) {
  return (
    <div className="validationSummary">
      <strong>{status.title || '检查未通过'}</strong>
      <ul>
        {(status.errors || []).map(error => <li key={error}>{error}</li>)}
      </ul>
    </div>
  )
}

function ParamMappingEditor({tool, params, sources, onChange, emptyMessage}) {
  const parameters = tool?.parameters || []
  if (!tool) return <div className="empty small">{emptyMessage || '未找到当前节点工具定义。'}</div>
  if (parameters.length === 0) return <div className="empty small">当前工具没有声明输入参数。</div>
  return (
    <div className="paramMappings">
      <strong>输入参数映射</strong>
      {parameters.map(param => (
        <div key={param.name} className="mappingRow">
          <div>
            <span>{param.description || param.name}{param.required ? ' *' : ''}</span>
            <em>{param.name}</em>
          </div>
          <select value={params[param.name] || ''} onChange={event => onChange(param.name, event.target.value)}>
            <option value="">手动输入 / 不设置</option>
            {sources.map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
          </select>
          {param.type === 'bool' && !isTemplateValue(params[param.name]) ? (
            <input type="checkbox" checked={parseBoolParamValue(params[param.name])} onChange={event => onChange(param.name, event.target.checked)} />
          ) : param.type === 'file' && !isTemplateValue(params[param.name]) ? (
            <FileUploadInput value={params[param.name] || ''} onChange={value => onChange(param.name, value)} />
          ) : (
            <input value={params[param.name] || ''} placeholder={param.default || param.name} onChange={event => onChange(param.name, event.target.value)} />
          )}
        </div>
      ))}
    </div>
  )
}

function ToolOutputsView({tool}) {
  const outputs = tool?.outputs || []
  if (outputs.length === 0) return <div className="empty small">当前工具没有声明输出参数。</div>
  return (
    <div className="toolOutputsView">
      <strong>输出参数</strong>
      {outputs.map(output => (
        <div className="toolOutputRow" key={output.name}>
          <span>{output.description || output.name}{output.required ? ' *' : ''}</span>
          <code>{output.name}</code>
          <small>{output.type || 'string'} · {output.json_path}</small>
        </div>
      ))}
    </div>
  )
}

function isTemplateValue(value) {
  return typeof value === 'string' && value.includes('{{')
}

function parseBoolParamValue(value) {
  return value === true || value === 'true' || value === '1' || value === 'yes' || value === 'on'
}

function updateCaseValuesText(value) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}

export {
  EdgeConfigModal,
  NodeConfigEditor,
  NodeConfigModal,
  ParamMappingEditor,
  ValidationSummary,
  WorkflowTagsEditor
}
