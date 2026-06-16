import {describe, expect, it} from 'vitest'
import {
  autoLayoutNodes,
  buildWorkflowDraft,
  defaultCondition,
  defaultUpload,
  normalizeLoopConfig,
  validateConditionDraft,
  validateControlDraft
} from './model.js'

describe('workflow model', () => {
  it('serializes condition case edges and strips display-only node data', () => {
    const condition = {
      ...defaultCondition(),
      input: '{{ .steps.inspect.stdout }}',
      cases: [{id: 'ok', name: '正常', operator: 'contains', values: ['OK']}]
    }
    const nodes = [
      {id: 'route', type: 'conditionNode', data: {name: '按结果分支', condition, run: {status: 'succeeded'}}},
      {id: 'notify', type: 'toolNode', data: {name: '通知', tool: 'demo.notify', params: {message: '{{ .steps.inspect.stdout }}'}, paramStatus: {total: 1}}}
    ]
    const edges = [{id: 'route-notify-ok', source: 'route', target: 'notify', sourceHandle: 'ok', data: {case: 'ok'}}]

    const draft = buildWorkflowDraft({id: 'demo.flow', category: '', tags: ['ops', 'ops']}, nodes, edges, 'global', [])

    expect(draft.category).toBe('global')
    expect(draft.tags).toEqual(['ops'])
    expect(draft.nodes[0]).toMatchObject({id: 'route', type: 'condition', name: '按结果分支'})
    expect(draft.nodes[1]).toEqual({id: 'notify', type: 'tool', name: '通知', tool: 'demo.notify', params: {message: '{{ .steps.inspect.stdout }}'}, on_failure: 'stop'})
    expect(draft.nodes[1]).not.toHaveProperty('run')
    expect(draft.nodes[1]).not.toHaveProperty('paramStatus')
    expect(draft.edges).toEqual([{from: 'route', to: 'notify', case: 'ok'}])
  })

  it('serializes unconnected linear nodes as sequential edges by canvas order', () => {
    const nodes = [
      {id: 'deploy', type: 'toolNode', position: {x: 360, y: 80}, data: {name: '部署', tool: 'demo.deploy'}},
      {id: 'inspect', type: 'toolNode', position: {x: 80, y: 120}, data: {name: '巡检', tool: 'demo.inspect'}},
      {id: 'notify', type: 'toolNode', position: {x: 640, y: 60}, data: {name: '通知', tool: 'demo.notify'}}
    ]

    const draft = buildWorkflowDraft({id: 'demo.linear'}, nodes, [], 'global', [])

    expect(draft.edges).toEqual([
      {from: 'inspect', to: 'deploy'},
      {from: 'deploy', to: 'notify'}
    ])
  })

  it('does not guess branch edges for unconnected condition workflows', () => {
    const nodes = [
      {id: 'route', type: 'conditionNode', position: {x: 80, y: 80}, data: {name: '分支', condition: defaultCondition()}},
      {id: 'notify', type: 'toolNode', position: {x: 360, y: 80}, data: {name: '通知', tool: 'demo.notify'}}
    ]

    const draft = buildWorkflowDraft({id: 'demo.branch'}, nodes, [], 'global', [])

    expect(draft.edges).toEqual([])
  })

  it('serializes upload control nodes and validates target directories', () => {
    const nodes = [
      {id: 'upload', type: 'controlNode', position: {x: 80, y: 80}, data: {controlType: 'upload', name: '上传包', upload: {target_dir: 'assets/release'}}},
      {id: 'consume', type: 'toolNode', position: {x: 360, y: 80}, data: {name: '处理', tool: 'demo.consume', params: {path: '{{ .steps.upload.file.path }}'}}}
    ]

    const draft = buildWorkflowDraft({id: 'demo.upload'}, nodes, [], 'global', [])

    expect(defaultUpload()).toEqual({target_dir: ''})
    expect(draft.nodes[0]).toEqual({id: 'upload', type: 'upload', name: '上传包', upload: {target_dir: 'assets/release'}})
    expect(draft.edges).toEqual([{from: 'upload', to: 'consume'}])
    expect(validateControlDraft([
      {id: 'bad', type: 'controlNode', data: {controlType: 'upload', upload: {target_dir: '../bad'}}}
    ], [], [])).toEqual(expect.arrayContaining([
      expect.stringContaining('上传节点 bad 的目标子目录无效')
    ]))
  })

  it('validates condition edge case contracts before save or run', () => {
    const nodes = [
      {id: 'route', type: 'conditionNode', data: {condition: {input: '', cases: [{id: 'default', operator: 'bad'}], default_case: ''}}},
      {id: 'plain', type: 'toolNode', data: {tool: 'demo.plain'}}
    ]
    const edges = [
      {source: 'route', target: 'plain', data: {case: 'missing'}},
      {source: 'plain', target: 'route', data: {case: 'ok'}}
    ]

    expect(validateConditionDraft(nodes, edges)).toEqual(expect.arrayContaining([
      '条件节点 route 缺少输入来源。',
      '条件节点 route 的 case ID 不能使用保留值 default。',
      '条件节点 route 的 case default 操作符非法。',
      '条件节点 route 到 plain 的连线引用不存在的 case：missing',
      '非条件节点 plain 的连线不能配置 case。'
    ]))
  })

  it('validates loop tool references and keeps auto layout out of draft semantics', () => {
    const nodes = [
      {id: 'start', type: 'toolNode', data: {tool: 'demo.start'}},
      {id: 'repeat', type: 'controlNode', data: {controlType: 'loop', loop: {tool: 'missing.tool', max_iterations: 99}}}
    ]
    const edges = [{source: 'start', target: 'repeat'}]

    expect(normalizeLoopConfig({tool: ' demo.loop ', maxIterations: '4'})).toEqual({tool: 'demo.loop', params: {}, max_iterations: 4})
    expect(validateControlDraft(nodes, edges, [{id: 'demo.loop'}])).toEqual(expect.arrayContaining([
      '循环节点 repeat 引用了不存在的工具：missing.tool'
    ]))

    const laidOut = autoLayoutNodes(nodes, edges)
    expect(laidOut[0].position.x).toBeLessThan(laidOut[1].position.x)
    expect(nodes[0]).not.toHaveProperty('position')
  })
})
