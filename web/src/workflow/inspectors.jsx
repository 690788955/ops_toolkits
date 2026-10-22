import React, {useState} from 'react'
import FileUploadInput from '../FileUploadInput.jsx'
import {sshHostStatusLabel} from '../SshHostsModal.jsx'
import {conditionOperators, controlNodeCategory, controlNodeHelp, controlNodeTitle} from './catalog.js'
import {
  defaultExtractConfig as modelDefaultExtractConfig,
  defaultCondition,
  defaultHttpRequest as modelDefaultHttpRequest,
  defaultLoop as modelDefaultLoop,
  defaultUpload as modelDefaultUpload,
  defaultParams,
  formatHTTPStatusList,
  httpBodyTypes,
  httpMethods,
  normalizeExtractConfig,
  normalizeHttpBodyType,
  normalizeHttpRequestConfig,
  normalizeHTTPStatusList,
  normalizeLoopConfig,
  normalizeUploadConfig
} from './model.js'

function NodeConfigModal({node, kindLabel, children, onClose, onSave}) {
  return (
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal nodeConfigModal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>节点配置</h3>
            <p>{kindLabel} · {node.data.name || node.id}</p>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        {children}
        <div className="modalFooter">
          <button type="button" className="secondary" onClick={onClose}>取消</button>
          <button type="button" className="primary" onClick={onSave}>保存配置</button>
        </div>
      </div>
    </div>
  )
}

function NodeConfigEditor({node, tool, loopTool, tools, nodes, hosts = [], sources, paramsText, setParamsText, onNameChange, onConditionChange, onLoopChange, onUploadChange, onExtractConfigChange, onHttpChange, onParamChange, onLoopParamChange, onApplyParams, onHostChange}) {
  if (!node) return null
  return (
    <div className="form nodeConfigEditor">
      {node.type === 'conditionNode' ? (
        <ConditionEditor node={node} sources={sources} onNameChange={onNameChange} onChange={onConditionChange} />
      ) : node.type === 'controlNode' ? (
        <ControlNodeInspector node={node} tools={tools} nodes={nodes} hosts={hosts} loopTool={loopTool} sources={sources} onNameChange={onNameChange} onLoopChange={onLoopChange} onUploadChange={onUploadChange} onExtractConfigChange={onExtractConfigChange} onHttpChange={onHttpChange} onLoopParamChange={onLoopParamChange} onHostChange={onHostChange} />
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

function ControlNodeInspector({node, tools, nodes, hosts, loopTool, sources, onNameChange, onLoopChange, onUploadChange, onExtractConfigChange, onHttpChange, onLoopParamChange, onHostChange}) {
  const isLoop = node.data.controlType === 'loop'
  const loop = normalizeLoopConfig(node.data.loop || modelDefaultLoop())
  const isUpload = node.data.controlType === 'upload'
  const isExtractConfig = node.data.controlType === 'extract_config'
  const isHTTP = node.data.controlType === 'http_request'
  const category = controlNodeCategory(node.data.controlType)
  const upload = normalizeUploadConfig(node.data.upload || modelDefaultUpload())
  const extract = normalizeExtractConfig(node.data.extract_config || modelDefaultExtractConfig())
  const httpRequest = normalizeHttpRequestConfig(node.data.http || modelDefaultHttpRequest())
  function updateLoop(patch) {
    onLoopChange(normalizeLoopConfig({...loop, ...patch}))
  }
  function updateExtract(patch) {
    onExtractConfigChange(normalizeExtractConfig({...extract, ...patch}))
  }
  function updateHttp(patch) {
    onHttpChange(normalizeHttpRequestConfig({...httpRequest, ...patch}))
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
            <span>目标子目录</span>
            <input value={upload.target_dir || ''} placeholder="可留空，例如 assets/release" onChange={event => onUploadChange({...upload, target_dir: event.target.value})} />
          </label>
          <div className="empty small">目标子目录只能是平台 uploads 下的相对路径；运行前在执行面板选择本地文件、多个文件或目录。</div>
          <div className="empty small">如需把上传包中的配置文件沉淀为工作流配置，请在该节点后连接“提取配置”节点，并使用上传节点输出的文件名、相对路径或目录。</div>
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
