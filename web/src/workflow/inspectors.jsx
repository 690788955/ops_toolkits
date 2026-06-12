import React, {useState} from 'react'

const conditionOperators = [
  {value: 'eq', label: '等于'},
  {value: 'neq', label: '不等于'},
  {value: 'contains', label: '包含'},
  {value: 'not_contains', label: '不包含'},
  {value: 'in', label: '在列表中'},
  {value: 'not_in', label: '不在列表中'},
  {value: 'exists', label: '存在'},
  {value: 'empty', label: '为空'}
]

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

function NodeConfigEditor({node, tool, loopTool, tools, sources, paramsText, setParamsText, onNameChange, onConditionChange, onLoopChange, onParamChange, onLoopParamChange, onApplyParams}) {
  if (!node) return null
  return (
    <div className="form nodeConfigEditor">
      {node.type === 'conditionNode' ? (
        <ConditionEditor node={node} sources={sources} onNameChange={onNameChange} onChange={onConditionChange} />
      ) : node.type === 'controlNode' ? (
        <ControlNodeInspector node={node} tools={tools} loopTool={loopTool} sources={sources} onNameChange={onNameChange} onLoopChange={onLoopChange} onLoopParamChange={onLoopParamChange} />
      ) : (
        <ToolNodeConfigEditor
          node={node}
          tool={tool}
          sources={sources}
          paramsText={paramsText}
          setParamsText={setParamsText}
          onParamChange={onParamChange}
          onApplyParams={onApplyParams}
        />
      )}
    </div>
  )
}

function ToolNodeConfigEditor({node, tool, sources, paramsText, setParamsText, onParamChange, onApplyParams}) {
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
      <ParamMappingEditor tool={tool} params={node.data.params || {}} sources={sources} onChange={onParamChange} />
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

function ControlNodeInspector({node, tools, loopTool, sources, onNameChange, onLoopChange, onLoopParamChange}) {
  const isLoop = node.data.controlType === 'loop'
  const loop = normalizeLoopConfig(node.data.loop || defaultLoop())
  function updateLoop(patch) {
    onLoopChange(normalizeLoopConfig({...loop, ...patch}))
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
        <input value={controlNodeTitle(node.data.controlType)} disabled />
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
          <ParamMappingEditor tool={loopTool} params={loop.params || {}} sources={sources} onChange={onLoopParamChange} emptyMessage="请先选择循环工具，再配置该工具的输入参数。" />
          <div className="empty small">循环节点会重复执行上方选择的工具，不需要在画布上额外添加目标工具节点；普通连线仍保持 DAG。</div>
        </>
      ) : (
        <div className="empty small">{controlNodeHelp(node.data.controlType)}。该节点不需要配置工具或参数，运行时自身记录为成功。</div>
      )}
    </div>
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
          <input value={params[param.name] || ''} placeholder={param.default || param.name} onChange={event => onChange(param.name, event.target.value)} />
        </div>
      ))}
    </div>
  )
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