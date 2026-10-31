import { describe, expect, it } from "vitest";
import {
  buildMappingSources,
  buildUpstreamToolOutputSources,
  autoLayoutNodes,
  autoLayoutWorkflowNodes,
  buildWorkflowDraft,
  controlNodeTypeFromWorkflow,
  defaultApprovalConfig,
  defaultCondition,
  defaultExtractConfig,
  defaultNotifyConfig,
  defaultUpload,
  defaultWorkflowStages,
  moveNodeToStage,
  moveNodeWithinStage,
  moveNodesToStage,
  newStageDraftErrors,
  orderedStageNodes,
  moveWorkflowStageTo,
  normalizeApprovalConfig,
  normalizeExtractConfig,
  normalizeHttpRequestConfig,
  normalizeLoopConfig,
  normalizeScriptConfig,
  normalizeUploadTargetDir,
  normalizeStageConcurrency,
  normalizeStageNodeOrders,
  normalizeWorkflowStages,
  relocateNodeStage,
  renumberStageForSerial,
  serializeArchiveConfig,
  serializeApprovalConfig,
  serializeNotifyConfig,
  serializeScriptConfig,
  serializeWaitConfig,
  isDurationText,
  stageModeLabel,
  stageLayout,
  validateConditionDraft,
  validateControlDraft,
  validateStageDraft,
  workflowControlNodeType,
} from "./model.js";

describe("workflow model", () => {
  it("serializes condition case edges and strips display-only node data", () => {
    const condition = {
      ...defaultCondition(),
      input: "{{ .steps.inspect.stdout }}",
      cases: [{ id: "ok", name: "正常", operator: "contains", values: ["OK"] }],
    };
    const nodes = [
      {
        id: "route",
        type: "conditionNode",
        data: { name: "按结果分支", condition, run: { status: "succeeded" } },
      },
      {
        id: "notify",
        type: "toolNode",
        data: {
          name: "通知",
          tool: "demo.notify",
          params: { message: "{{ .steps.inspect.stdout }}" },
          paramStatus: { total: 1 },
        },
      },
    ];
    const edges = [
      {
        id: "route-notify-ok",
        source: "route",
        target: "notify",
        sourceHandle: "ok",
        data: { case: "ok" },
      },
    ];

    const draft = buildWorkflowDraft(
      { id: "demo.flow", category: "", tags: ["ops", "ops"] },
      nodes,
      edges,
      "global",
      [],
    );

    expect(draft.category).toBe("global");
    expect(draft.tags).toEqual(["ops"]);
    expect(draft.nodes[0]).toMatchObject({
      id: "route",
      type: "condition",
      name: "按结果分支",
    });
    expect(draft.nodes[1]).toEqual({
      id: "notify",
      type: "tool",
      name: "通知",
      tool: "demo.notify",
      params: { message: "{{ .steps.inspect.stdout }}" },
      on_failure: "stop",
    });
    expect(draft.nodes[1]).not.toHaveProperty("run");
    expect(draft.nodes[1]).not.toHaveProperty("paramStatus");
    expect(draft.edges).toEqual([{ from: "route", to: "notify", case: "ok" }]);
  });

  it("serializes unconnected linear nodes as sequential edges by canvas order", () => {
    const nodes = [
      {
        id: "deploy",
        type: "toolNode",
        position: { x: 360, y: 80 },
        data: { name: "部署", tool: "demo.deploy" },
      },
      {
        id: "inspect",
        type: "toolNode",
        position: { x: 80, y: 120 },
        data: { name: "巡检", tool: "demo.inspect" },
      },
      {
        id: "notify",
        type: "toolNode",
        position: { x: 640, y: 60 },
        data: { name: "通知", tool: "demo.notify" },
      },
    ];

    const draft = buildWorkflowDraft(
      { id: "demo.linear" },
      nodes,
      [],
      "global",
      [],
    );

    expect(draft.edges).toEqual([
      { from: "inspect", to: "deploy" },
      { from: "deploy", to: "notify" },
    ]);
  });

  it("does not guess branch edges for unconnected condition workflows", () => {
    const nodes = [
      {
        id: "route",
        type: "conditionNode",
        position: { x: 80, y: 80 },
        data: { name: "分支", condition: defaultCondition() },
      },
      {
        id: "notify",
        type: "toolNode",
        position: { x: 360, y: 80 },
        data: { name: "通知", tool: "demo.notify" },
      },
    ];

    const draft = buildWorkflowDraft(
      { id: "demo.branch" },
      nodes,
      [],
      "global",
      [],
    );

    expect(draft.edges).toEqual([]);
  });

  it("serializes upload control nodes and validates target directories", () => {
    const nodes = [
      {
        id: "upload",
        type: "controlNode",
        position: { x: 80, y: 80 },
        data: {
          controlType: "upload",
          name: "上传包",
          upload: { target_dir: "assets/release" },
        },
      },
      {
        id: "consume",
        type: "toolNode",
        position: { x: 360, y: 80 },
        data: {
          name: "处理",
          tool: "demo.consume",
          params: { path: "{{ .steps.upload.file.path }}" },
        },
      },
    ];

    const draft = buildWorkflowDraft(
      { id: "demo.upload" },
      nodes,
      [],
      "global",
      [],
    );

    expect(defaultUpload()).toEqual({ target_dir: "" });
    expect(draft.nodes[0]).toEqual({
      id: "upload",
      type: "upload",
      name: "上传包",
      upload: { target_dir: "assets/release" },
    });
    expect(draft.edges).toEqual([{ from: "upload", to: "consume" }]);
    // 绝对路径由服务端按上传白名单校验，前端只拦相对路径里的逃逸写法。
    expect(normalizeUploadTargetDir("/opt/app/release")).toEqual({ value: "/opt/app/release", error: "" });
    expect(normalizeUploadTargetDir("D:\\deploy\\packages")).toEqual({ value: "D:/deploy/packages", error: "" });
    expect(
      validateControlDraft(
        [
          {
            id: "bad",
            type: "controlNode",
            data: { controlType: "upload", upload: { target_dir: "../bad" } },
          },
        ],
        [],
        [],
      ),
    ).toEqual(
      expect.arrayContaining([
        expect.stringContaining("上传节点 bad 的目标子目录无效"),
      ]),
    );
  });

  it("serializes extract config nodes and validates source and target paths", () => {
    const nodes = [
      {
        id: "upload",
        type: "controlNode",
        position: { x: 80, y: 80 },
        data: {
          controlType: "upload",
          name: "上传包",
          upload: { target_dir: "" },
        },
      },
      {
        id: "extract_config",
        type: "controlNode",
        position: { x: 360, y: 80 },
        data: {
          controlType: "extract_config",
          name: "提取配置",
          extract_config: {
            file_name: "{{ .steps.upload.file.filename }}",
            target_path: "conf/app.yaml",
            label: "应用配置",
            replace: true,
          },
        },
      },
    ];

    const draft = buildWorkflowDraft(
      { id: "demo.extract" },
      nodes,
      [],
      "global",
      [],
    );

    expect(defaultExtractConfig()).toEqual({
      source_type: "file",
      file_name: "",
      target_path: "",
      label: "",
      replace: false,
      source_dir: "",
      files: [],
    });
    expect(
      normalizeExtractConfig({
        source_path: "old/app.yaml",
        target_path: "app.yaml",
      }),
    ).toMatchObject({
      file_name: "old/app.yaml",
      target_path: "app.yaml",
      label: "",
      replace: false,
    });
    expect(draft.nodes[1]).toEqual({
      id: "extract_config",
      type: "extract_config",
      name: "提取配置",
      extract_config: {
        file_name: "{{ .steps.upload.file.filename }}",
        target_path: "conf/app.yaml",
        label: "应用配置",
        replace: true,
      },
    });
    expect(draft.edges).toEqual([{ from: "upload", to: "extract_config" }]);
    expect(
      validateControlDraft(
        [
          {
            id: "bad",
            type: "controlNode",
            data: {
              controlType: "extract_config",
              extract_config: {
                file_name: "../secret",
                target_path: "/abs.yaml",
              },
            },
          },
        ],
        [],
        [],
      ),
    ).toEqual(
      expect.arrayContaining([
        expect.stringContaining("提取配置节点 bad 的源文件名无效"),
        expect.stringContaining("提取配置节点 bad 的目标配置路径无效"),
      ]),
    );
  });

  it("serializes directory extract configs with source files following relative paths", () => {
    const nodes = [
      {
        id: "upload",
        type: "controlNode",
        position: { x: 80, y: 80 },
        data: { controlType: "upload", name: "上传包", upload: { target_dir: "" } },
      },
      {
        id: "extract",
        type: "controlNode",
        position: { x: 360, y: 80 },
        data: {
          controlType: "extract_config",
          name: "提取配置",
          extract_config: {
            source_type: "directory",
            source_dir: "{{ .steps.upload.file.relative_dir }}",
            files: [
              { source_path: "conf/app.yaml", label: "应用配置", replace: true },
              { source_path: "conf/db.yaml", label: "数据库配置", replace: false },
            ],
          },
        },
      },
    ];

    const draft = buildWorkflowDraft(
      { id: "demo.extract.directory" },
      nodes,
      [],
      "global",
      [],
    );

    expect(defaultExtractConfig()).toEqual({
      source_type: "file",
      file_name: "",
      target_path: "",
      label: "",
      replace: false,
      source_dir: "",
      files: [],
    });
    expect(draft.nodes[1]).toEqual({
      id: "extract",
      type: "extract_config",
      name: "提取配置",
      extract_config: {
        source_type: "directory",
        source_dir: "{{ .steps.upload.file.relative_dir }}",
        files: [
          { source_path: "conf/app.yaml", target_path: "conf/app.yaml", label: "应用配置", replace: true },
          { source_path: "conf/db.yaml", target_path: "conf/db.yaml", label: "数据库配置", replace: false },
        ],
      },
    });
    expect(
      validateControlDraft(
        [
          {
            id: "bad",
            type: "controlNode",
            data: {
              controlType: "extract_config",
              extract_config: {
                source_type: "directory",
                source_dir: "",
                files: [{ source_path: "../bad.yaml" }],
              },
            },
          },
        ],
        [],
        [],
      ),
    ).toEqual(
      expect.arrayContaining([
        expect.stringContaining("源目录来源不能为空"),
        expect.stringContaining("目录文件 1 源相对文件路径无效"),
      ]),
    );
  });

  it("validates condition edge case contracts before save or run", () => {
    const nodes = [
      {
        id: "route",
        type: "conditionNode",
        data: {
          condition: {
            input: "",
            cases: [{ id: "default", operator: "bad" }],
            default_case: "",
          },
        },
      },
      { id: "plain", type: "toolNode", data: { tool: "demo.plain" } },
    ];
    const edges = [
      { source: "route", target: "plain", data: { case: "missing" } },
      { source: "plain", target: "route", data: { case: "ok" } },
    ];

    expect(validateConditionDraft(nodes, edges)).toEqual(
      expect.arrayContaining([
        "条件节点 route 缺少输入来源。",
        "条件节点 route 的 case ID 不能使用保留值 default。",
        "条件节点 route 的 case default 操作符非法。",
        "条件节点 route 到 plain 的连线引用不存在的 case：missing",
        "非条件节点 plain 的连线不能配置 case。",
      ]),
    );
  });

  it("validates loop tool references and keeps auto layout out of draft semantics", () => {
    const nodes = [
      { id: "start", type: "toolNode", data: { tool: "demo.start" } },
      {
        id: "repeat",
        type: "controlNode",
        data: {
          controlType: "loop",
          loop: { tool: "missing.tool", max_iterations: 99 },
        },
      },
    ];
    const edges = [{ source: "start", target: "repeat" }];

    expect(
      normalizeLoopConfig({ tool: " demo.loop ", maxIterations: "4" }),
    ).toEqual({ tool: "demo.loop", params: {}, max_iterations: 4 });
    expect(validateControlDraft(nodes, edges, [{ id: "demo.loop" }])).toEqual(
      expect.arrayContaining([
        "循环节点 repeat 引用了不存在的工具：missing.tool",
      ]),
    );

    const laidOut = autoLayoutNodes(nodes, edges);
    expect(laidOut[0].position.x).toBeLessThan(laidOut[1].position.x);
    expect(nodes[0]).not.toHaveProperty("position");
  });

  it("exposes upstream tool outputs as mapping sources without serializing display metadata", () => {
    const nodes = [
      {
        id: "produce",
        type: "toolNode",
        data: {
          name: "生成",
          tool: "demo.produce",
          params: { path: "{{ .package_path }}" },
          toolMeta: { sourceLabel: "Demo" },
          outputs: [{ name: "ignored" }],
        },
      },
      {
        id: "consume",
        type: "toolNode",
        data: {
          name: "消费",
          tool: "demo.consume",
          params: { file: "{{ .steps.produce.outputs.output_file }}" },
        },
      },
    ];
    const edges = [{ source: "produce", target: "consume" }];
    const tools = [
      {
        id: "demo.produce",
        outputs: [{ name: "output_file", description: "输出文件" }],
      },
    ];

    expect(buildMappingSources([], "consume", nodes, edges, tools)).toEqual(
      expect.arrayContaining([
        {
          label: "produce / 输出参数 output_file",
          value: "{{ .steps.produce.outputs.output_file }}",
        },
        {
          label: "produce / 输入参数 path",
          value: "{{ .steps.produce.params.path }}",
        },
      ]),
    );
    expect(
      buildUpstreamToolOutputSources("consume", nodes, edges, tools, {
        produce: { stdout: 'human log\n{"output_file":"release.tar.gz"}\n' },
      }),
    ).toEqual([
      {
        label: "produce / 输出参数 output_file（输出文件）",
        value: "{{ .steps.produce.outputs.output_file }}",
        kind: "string",
        preview: "release.tar.gz",
      },
    ]);

    const draft = buildWorkflowDraft(
      { id: "demo.outputs" },
      nodes,
      edges,
      "global",
      [],
    );
    expect(draft.nodes[0]).toEqual({
      id: "produce",
      type: "tool",
      name: "生成",
      tool: "demo.produce",
      params: { path: "{{ .package_path }}" },
      on_failure: "stop",
    });
    expect(draft.nodes[0]).not.toHaveProperty("outputs");
    expect(draft.nodes[0]).not.toHaveProperty("toolMeta");
  });
});

describe("阶段工作流模型", () => {
  const stages = defaultWorkflowStages();
  const nodes = [
    {
      id: "inspect",
      type: "toolNode",
      position: { x: 0, y: 0 },
      data: { name: "检查", tool: "demo.inspect", stageId: "prepare", order: 10 },
    },
    {
      id: "deploy",
      type: "toolNode",
      position: { x: 0, y: 0 },
      data: { name: "发布", tool: "demo.deploy", stageId: "change", order: 10 },
    },
    {
      id: "verify",
      type: "toolNode",
      position: { x: 0, y: 0 },
      data: { name: "验证", tool: "demo.verify", stageId: "verify", order: 10 },
    },
  ];

  it("序列化阶段归属和未知字段，不为串行阶段生成顺序边", () => {
    const draft = buildWorkflowDraft(
      {
        id: "demo.flow",
        nodes: [
          {
            id: "inspect",
            tool: "demo.inspect",
            timeout: "2m",
            confirm: true,
            depends_on: ["legacy"],
            custom_flag: "keep",
          },
        ],
      },
      nodes,
      [],
      "global",
      [],
      stages,
    );

    expect(draft.stages).toEqual(stages);
    expect(draft.edges).toEqual([]);
    expect(
      draft.nodes.map((node) => ({
        id: node.id,
        stage_id: node.stage_id,
        order: node.order,
      })),
    ).toEqual([
      { id: "inspect", stage_id: "prepare", order: 10 },
      { id: "deploy", stage_id: "change", order: 10 },
      { id: "verify", stage_id: "verify", order: 10 },
    ]);
    expect(draft.nodes[0]).toMatchObject({
      timeout: "2m",
      confirm: true,
      custom_flag: "keep",
    });
    expect(draft.nodes[0]).not.toHaveProperty("depends_on");
  });

  it("保存时不恢复已删除节点或已内联的旧循环目标", () => {
    const workflow = {
      id: "demo.flow",
      nodes: [
        { id: "inspect", tool: "demo.inspect", timeout: "2m" },
        { id: "deleted", tool: "demo.deleted" },
        { id: "loop_target", tool: "demo.target" },
      ],
    };
    const draft = buildWorkflowDraft(workflow, nodes, [], "global", [], stages);
    expect(draft.nodes.map((node) => node.id)).toEqual(["inspect", "deploy", "verify"]);
    expect(draft.nodes[0].timeout).toBe("2m");
  });

  it("没有显式连线时仍提供阶段内和前置阶段的输出映射", () => {
    const tools = [{ id: "demo.inspect", outputs: [{ name: "backup" }] }];
    const sources = buildMappingSources([], "verify", nodes, [], tools, stages);
    expect(sources.map((source) => source.value)).toEqual(expect.arrayContaining([
      "{{ .steps.inspect.outputs.backup }}",
      "{{ .steps.deploy.stdout }}",
    ]));
    expect(sources.some((source) => source.value.includes(".steps.verify."))).toBe(false);
    expect(buildMappingSources([], "inspect", nodes, [], tools, stages)).toEqual([]);

    const withinStage = nodes.map((node) => node.id === "deploy"
      ? { ...node, data: { ...node.data, stageId: "prepare", order: 20 } }
      : node);
    expect(buildMappingSources([], "deploy", withinStage, [], tools, stages))
      .toEqual(expect.arrayContaining([expect.objectContaining({ value: "{{ .steps.inspect.stdout }}" })]));
  });

  it("删除并保存后撤销，仍保留恢复节点及依赖的扩展字段", () => {
    const restored = nodes.map((node) => node.id === "inspect"
      ? { ...node, data: { ...node.data, sourceConfig: { id: "inspect", timeout: "2m", optional: true, extension: { flag: true } } } }
      : node);
    const edge = { source: "inspect", target: "deploy", data: { sourceConfig: { from: "inspect", to: "deploy", note: "业务依赖" } } };
    const draft = buildWorkflowDraft({id: "demo.flow", nodes: []}, restored, [edge], "global", [], stages);
    expect(draft.nodes[0]).toMatchObject({ timeout: "2m", optional: true, extension: { flag: true } });
    expect(draft.nodes[0]).not.toHaveProperty("sourceConfig");
    expect(draft.edges).toEqual([{ from: "inspect", to: "deploy", note: "业务依赖" }]);
  });

  it("拒绝与阶段内串行顺序冲突的显式依赖", () => {
    const sameStage = nodes.map((node) => node.id === "deploy"
      ? { ...node, data: { ...node.data, stageId: "prepare", order: 20 } }
      : node);
    expect(validateStageDraft(sameStage, [{ source: "deploy", target: "inspect" }], stages))
      .toEqual(expect.arrayContaining([expect.stringContaining("节点顺序冲突")]));
    expect(validateStageDraft(sameStage, [{ source: "inspect", target: "deploy" }], stages)).toEqual([]);
  });

  it("在阶段内重新编号并支持跨阶段移动和调整阶段顺序", () => {
    const withExtra = [
      ...nodes,
      {
        id: "precheck",
        type: "toolNode",
        position: { x: 0, y: 0 },
        data: { name: "预检", tool: "demo.precheck", stageId: "prepare", order: 5 },
      },
    ];
    const normalized = normalizeStageNodeOrders(withExtra, stages);
    expect(
      Object.fromEntries(
        normalized
          .filter((node) => node.data.stageId === "prepare")
          .map((node) => [node.id, node.data.order]),
      ),
    ).toEqual({
      precheck: 10,
      inspect: 20,
    });

    const moved = moveNodeToStage(withExtra, "verify", "change");
    expect(moved.find((node) => node.id === "verify").data).toMatchObject({
      stageId: "change",
      order: 20,
    });

    const reordered = moveNodeWithinStage(withExtra, "inspect", -1);
    expect(reordered.find((node) => node.id === "inspect").data.order).toBe(5);
    expect(reordered.find((node) => node.id === "precheck").data.order).toBe(10);

    const partial = moveNodesToStage(withExtra, ["deploy"], "change");
    expect(partial.find((node) => node.id === "deploy").data.order).toBe(10);

    const batched = moveNodesToStage(partial, ["verify", "inspect"], "change");
    expect(
      orderedStageNodes(batched, "change").map((node) => [node.id, node.data.order]),
    ).toEqual([
      ["deploy", 10],
      ["inspect", 20],
      ["verify", 30],
    ]);
    expect(validateStageDraft(batched, [], stages)).toEqual([]);

    expect(moveWorkflowStageTo(stages, "change", 0).map((stage) => stage.id)).toEqual([
      "change",
      "prepare",
      "verify",
    ]);
    expect(moveWorkflowStageTo(stages, "prepare", 2).map((stage) => stage.id)).toEqual([
      "change",
      "verify",
      "prepare",
    ]);
  });

  it("校验阶段归属、重复顺序和跨阶段依赖", () => {
    expect(validateStageDraft(nodes, [], stages)).toEqual([]);

    const duplicate = nodes.map((node) =>
      node.id === "precheck"
        ? node
        : node.id === "verify"
          ? { ...node, data: { ...node.data, stageId: "change", order: 10 } }
          : node,
    );
    expect(validateStageDraft(duplicate, [], stages)).toEqual(
      expect.arrayContaining([
        expect.stringContaining("顺序重复"),
      ]),
    );

    const backwards = [
      { source: "deploy", target: "inspect" },
    ];
    expect(validateStageDraft(nodes, backwards, stages)).toEqual(
      expect.arrayContaining([
        expect.stringContaining("阶段顺序冲突"),
      ]),
    );

    const crossStageCondition = [
      ...nodes,
      {
        id: "route",
        type: "conditionNode",
        position: { x: 0, y: 0 },
        data: { name: "分支", condition: defaultCondition(), stageId: "prepare", order: 30 },
      },
    ];
    expect(
      validateStageDraft(crossStageCondition, [{ source: "route", target: "deploy" }], stages),
    ).toEqual([]);
  });

  it("条件分支可以跨阶段，但被分支连回的更早阶段仍视为顺序冲突", () => {
    const condition = [
      ...nodes,
      {
        id: "route",
        type: "conditionNode",
        position: { x: 0, y: 0 },
        data: { name: "分支", condition: defaultCondition(), stageId: "change", order: 5 },
      },
    ];
    expect(validateStageDraft(condition, [{ source: "route", target: "verify" }], stages)).toEqual([]);
    expect(
      validateStageDraft(condition, [{ source: "route", target: "inspect" }], stages),
    ).toEqual(expect.arrayContaining([expect.stringContaining("阶段顺序冲突")]));
  });

  it("增量校验只报告新出现的冲突，并满足批量移动自身的顺序约束", () => {
    const backwards = [{ source: "deploy", target: "inspect" }];
    expect(newStageDraftErrors(nodes, nodes, backwards, stages)).toEqual([]);
    expect(
      newStageDraftErrors(nodes, moveNodesToStage(nodes, ["deploy"], "prepare"), backwards, stages),
    ).toEqual(
      expect.arrayContaining([expect.stringContaining("节点顺序冲突")]),
    );
    expect(
      newStageDraftErrors(nodes, moveNodesToStage(nodes, ["inspect"], "verify"), backwards, stages),
    ).toEqual([]);
  });

  it("按阶段顺序生成确定性的列布局", () => {
    const layout = stageLayout(nodes, stages);
    expect(layout.stages.map((stage) => stage.id)).toEqual([
      "prepare",
      "change",
      "verify",
    ]);
    expect(layout.stages[0].x).toBeLessThan(layout.stages[1].x);
    expect(layout.stages[1].x).toBeLessThan(layout.stages[2].x);
    // 阶段头要能完整显示 7 个字的名称、执行模式和操作按钮。
    expect(layout.stages.every((stage) => stage.width >= 324)).toBe(true);
    expect(layout.positions.inspect).toBeTruthy();
    expect(layout.positions.verify).toBeTruthy();

    const laidOut = autoLayoutWorkflowNodes(nodes, [], stages);
    expect(laidOut.find((node) => node.id === "deploy").position).toEqual(
      layout.positions.deploy,
    );
  });

  it("归一化阶段名称并补默认串行模式", () => {
    expect(
      normalizeWorkflowStages([
        { id: " prepare ", name: "" },
        { id: "change", name: "变更", execution_mode: "serial" },
        { id: "prepare", name: "重复" },
      ]),
    ).toEqual([
      { id: "prepare", name: "prepare", execution_mode: "serial" },
      { id: "change", name: "变更", execution_mode: "serial" },
    ]);
  });
});

describe("HTTP 节点配置", () => {
  it("把画布节点序列化为 http 配置，并丢弃空 Header", () => {
    const nodes = [
      {
        id: "call_api",
        type: "controlNode",
        data: {
          controlType: "http_request",
          name: "调用发布接口",
          http: {
            method: "post",
            url: "https://api.example.com/deploy",
            headers: [
              { name: "Authorization", value: "Bearer {{ .token }}" },
              { name: "", value: "" },
            ],
            body_type: "json",
            body: '{"env":"{{ .env }}"}',
            expect_status: [200, 201],
            save_response: "response.json",
            fail_on_error: false,
            timeout: "15s",
          },
        },
      },
    ];
    const draft = buildWorkflowDraft({ id: "demo.http" }, nodes, [], "global", []);
    expect(draft.nodes[0]).toEqual({
      id: "call_api",
      type: "http_request",
      name: "调用发布接口",
      http: {
        method: "POST",
        url: "https://api.example.com/deploy",
        headers: { Authorization: "Bearer {{ .token }}" },
        body_type: "json",
        body: '{"env":"{{ .env }}"}',
        expect_status: [200, 201],
        save_response: "response.json",
        fail_on_error: false,
        timeout: "15s",
      },
    });
  });

  it("兼容后端 yaml 里的 headers 对象写法与字符串状态码", () => {
    const http = normalizeHttpRequestConfig({
      method: "get",
      url: " https://example.com/health ",
      headers: { "X-Token": "abc" },
      expect_status: "200, 201, 200, 999",
      fail_on_error: false,
    });
    expect(http.method).toBe("GET");
    expect(http.url).toBe("https://example.com/health");
    expect(http.headers).toEqual([{ name: "X-Token", value: "abc" }]);
    expect(http.expect_status).toEqual([200, 201]);
    expect(http.body_type).toBe("none");
    expect(http.fail_on_error).toBe(false);
  });

  it("校验必填 URL、协议、GET 请求体与超时格式", () => {
    const base = {
      id: "call",
      type: "controlNode",
      data: {
        controlType: "http_request",
        http: { method: "GET", url: "", body_type: "none" },
      },
    };
    expect(
      validateControlDraft([base], []).some((item) =>
        item.includes("请输入请求 URL"),
      ),
    ).toBe(true);

    const fileScheme = {
      ...base,
      data: {
        controlType: "http_request",
        http: { method: "GET", url: "file:///etc/passwd" },
      },
    };
    expect(
      validateControlDraft([fileScheme], []).some((item) =>
        item.includes("http:// 或 https://"),
      ),
    ).toBe(true);

    const getWithBody = {
      ...base,
      data: {
        controlType: "http_request",
        http: {
          method: "GET",
          url: "https://example.com",
          body_type: "json",
          body: '{"a":1}',
        },
      },
    };
    expect(
      validateControlDraft([getWithBody], []).some((item) =>
        item.includes("GET 请求不能配置请求体"),
      ),
    ).toBe(true);

    const badTimeout = {
      ...base,
      data: {
        controlType: "http_request",
        http: { url: "https://example.com", timeout: "30 秒" },
      },
    };
    expect(
      validateControlDraft([badTimeout], []).some((item) =>
        item.includes("超时时间格式无效"),
      ),
    ).toBe(true);

    const escapingPath = {
      ...base,
      data: {
        controlType: "http_request",
        http: { url: "https://example.com", save_response: "../x.json" },
      },
    };
    expect(
      validateControlDraft([escapingPath], []).some((item) =>
        item.includes("响应保存路径无效"),
      ),
    ).toBe(true);

    const ok = {
      ...base,
      data: {
        controlType: "http_request",
        http: {
          method: "POST",
          url: "{{ .base_url }}/health",
          body_type: "json",
          body: '{"a":1}',
          save_response: "response/health.json",
          timeout: "10s",
        },
      },
    };
    expect(validateControlDraft([ok], [])).toEqual([]);
  });
});

describe("阶段并行模式", () => {
  const stages = [
    { id: "prepare", name: "准备", execution_mode: "serial" },
    { id: "change", name: "变更", execution_mode: "parallel", max_concurrency: 3 },
    { id: "verify", name: "验证" },
  ];
  const node = (id, stageId, order, extra = {}) => ({
    id,
    type: "toolNode",
    data: { name: id, stageId, order, ...extra },
  });

  it("归一化保留串行与并行阶段的并发上限", () => {
    const normalized = normalizeWorkflowStages([
      { id: "a", execution_mode: "parallel", max_concurrency: 6 },
      { id: "b", execution_mode: "serial", max_concurrency: 6 },
      { id: "c", execution_mode: "parallel", max_concurrency: "99" },
    ]);
    expect(normalized).toEqual([
      { id: "a", name: "a", execution_mode: "parallel", max_concurrency: 6 },
      { id: "b", name: "b", execution_mode: "serial", max_concurrency: 6 },
      { id: "c", name: "c", execution_mode: "parallel", max_concurrency: 16 },
    ]);
    expect(normalizeStageConcurrency("")).toBe(0);
    expect(normalizeStageConcurrency("0")).toBe(0);
    expect(stageModeLabel(normalized[0])).toBe("并行 · 最多 6 个");
    expect(stageModeLabel(normalized[1])).toBe("串行");
  });

  it("并行阶段不再校验阶段内顺序，但仍拦截跨阶段倒序依赖", () => {
    const nodes = [node("b1", "change", 0), node("b2", "change", 0), node("v1", "verify", 10)];
    expect(
      validateStageDraft(nodes, [{ source: "b2", target: "b1" }], stages),
    ).toEqual([]);
    expect(
      validateStageDraft(nodes, [{ source: "v1", target: "b1" }], stages).length,
    ).toBe(1);
  });

  it("并行阶段内的显式依赖成环时报错，避免永久等待", () => {
    const nodes = [node("b1", "change", 10), node("b2", "change", 20), node("b3", "change", 30)];
    const errors = validateStageDraft(
      nodes,
      [
        { source: "b1", target: "b2" },
        { source: "b2", target: "b3" },
        { source: "b3", target: "b1" },
      ],
      stages,
    );
    expect(errors.join("\n")).toContain("显式依赖存在环");
  });

  it("切回串行时按显式依赖重排节点顺序，且不删除依赖", () => {
    const nodes = [node("b1", "change", 10), node("b2", "change", 20), node("b3", "change", 30)];
    const edges = [
      { source: "b3", target: "b1" },
      { source: "b1", target: "b2" },
    ];
    const renumbered = renumberStageForSerial(nodes, edges, "change");
    const orderOf = Object.fromEntries(renumbered.map((item) => [item.id, item.data.order]));
    expect(orderOf).toEqual({ b3: 10, b1: 20, b2: 30 });
    expect(validateStageDraft(renumbered, edges, stages)).toEqual([]);
    expect(edges).toHaveLength(2);
  });

  it("串并行往返与序列化重载均保留并发上限", () => {
    const original = [{ id: "change", name: "变更", execution_mode: "parallel", max_concurrency: 3 }];
    const toSerial = original.map((stage) => ({ ...stage, execution_mode: "serial" }));
    const normalized = normalizeWorkflowStages(toSerial);
    expect(normalized).toEqual([{ id: "change", name: "变更", execution_mode: "serial", max_concurrency: 3 }]);
    const saved = buildWorkflowDraft({id: "save-serial"}, [], [], "global", [], normalized);
    const backToParallel = saved.stages.map((stage) => ({ ...stage, execution_mode: "parallel" }));
    expect(normalizeWorkflowStages(backToParallel)[0]).toEqual({
      id: "change",
      name: "变更",
      execution_mode: "parallel",
      max_concurrency: 3,
    });
  });

  it("序列化阶段模式，并行阶段不生成隐式顺序边", () => {
    const nodes = [node("b1", "change", 10), node("b2", "change", 20)];
    const draft = buildWorkflowDraft({ id: "demo.parallel" }, nodes, [], "global", [], stages);
    expect(draft.stages[1]).toEqual({
      id: "change",
      name: "变更",
      execution_mode: "parallel",
      max_concurrency: 3,
    });
    expect(draft.edges).toEqual([]);
  });

  it("并行阶段的映射来源只包含显式上游和更早阶段", () => {
    const nodes = [
      node("p1", "prepare", 10),
      node("b1", "change", 10),
      node("b2", "change", 20),
    ];
    const sources = buildMappingSources([], "b2", nodes, [], [], stages);
    expect(sources.map((item) => item.value)).toEqual(
      expect.arrayContaining(["{{ .steps.p1.stdout }}"]),
    );
    expect(sources.some((item) => item.value.includes(".steps.b1."))).toBe(false);

    const chained = buildMappingSources([], "b2", nodes, [{ source: "b1", target: "b2" }], [], stages);
    expect(chained.map((item) => item.value)).toEqual(
      expect.arrayContaining(["{{ .steps.b1.stdout }}"]),
    );
  });

  it("并行阶段映射包含传递祖先，并排除无依赖的兄弟节点", () => {
    const nodes = [node("a", "change", 10), node("b", "change", 20), node("c", "change", 30), node("sibling", "change", 40)];
    const sources = buildMappingSources([], "c", nodes, [{source: "a", target: "b"}, {source: "b", target: "c"}], [], stages);
    expect(sources.map(source => source.value)).toContain("{{ .steps.a.stdout }}");
    expect(sources.map(source => source.value)).toContain("{{ .steps.b.stdout }}");
    expect(sources.some(source => source.value.includes(".steps.sibling."))).toBe(false);
  });
});

describe("内置执行节点配置", () => {
  it("Shell与校验节点保存和再次运行时保留执行机器，清空后不恢复旧机器", () => {
    const original = [{id: "run", type: "shell", host: "old-host"}, {id: "sum", type: "file_checksum", host: "old-host"}];
    const nodes = [
      {id: "run", type: "controlNode", data: {controlType: "shell_script", host: "host-a", script: {script: "true"}}},
      {id: "sum", type: "controlNode", data: {controlType: "file_checksum", host: "host-b", checksum: {path: "/tmp/a"}}}
    ];
    const saved = buildWorkflowDraft({id: "hosts", nodes: original}, nodes, [], "global", []);
    expect(saved.nodes.map(node => node.host)).toEqual(["host-a", "host-b"]);
    const cleared = buildWorkflowDraft(saved, nodes.map(node => ({...node, data: {...node.data, host: ""}})), [], "global", []);
    expect(cleared.nodes.every(node => !("host" in node))).toBe(true);
  });

  it("环境变量草稿保留空行供编辑，序列化只输出已命名变量", () => {
    const config = normalizeScriptConfig({script: "true", env: [{name: "", value: ""}, {name: "lowercase", value: "ok"}]});
    expect(config.env).toHaveLength(2);
    expect(serializeScriptConfig(config).env).toEqual({lowercase: "ok"});
  });

  it("时长校验与Go保持正值且必须有单位", () => {
    for (const text of ["30", "0s", "-1s", "1s garbage"]) expect(isDurationText(text)).toBe(false);
    for (const text of ["30s", "1h30m", ".5s", "1.s", "+2m", "1μs"]) expect(isDurationText(text)).toBe(true);
  });

  it("文件校验模板不改写大小写，动态压缩包路径允许运行时识别格式", () => {
    const nodes = [
      {id: "sum", type: "controlNode", data: {controlType: "file_checksum", checksum: {path: "/tmp/a", expected: "{{ .ExpectedSHA }}"}}},
      {id: "unpack", type: "controlNode", data: {controlType: "archive", archive: {path: "{{ .PackagePath }}", target_dir: "/tmp/out"}}}
    ];
    expect(validateControlDraft(nodes, [])).toEqual([]);
    const saved = buildWorkflowDraft({id: "templates"}, nodes, [], "global", []);
    expect(saved.nodes[0].checksum.expected).toBe("{{ .ExpectedSHA }}");
  });

  it("Shell变量名称非法时报告错误", () => {
    const nodes = [{id: "shell", type: "controlNode", data: {controlType: "shell_script", script: {script: "true", env: [{name: "BAD-NAME", value: "x"}]}}}];
    expect(validateControlDraft(nodes, []).join("\n")).toContain("环境变量名");
  });
  it("Shell 脚本序列化保留脚本、工作目录、环境变量与退出码策略", () => {
    expect(
      serializeScriptConfig({
        script: "echo hi",
        workdir: "/opt/app",
        env: [{ name: "ENV", value: "prod" }, { name: "", value: "x" }],
        fail_on_exit: false,
        timeout: "2m",
      }),
    ).toEqual({
      script: "echo hi",
      workdir: "/opt/app",
      env: { ENV: "prod" },
      fail_on_exit: false,
      timeout: "2m",
    });
  });

  it("解压节点只输出有值的可选字段", () => {
    expect(
      serializeArchiveConfig({ path: "/tmp/a.tgz", target_dir: "/opt/a", format: "zip", strip_components: "2", overwrite: true }),
    ).toEqual({ path: "/tmp/a.tgz", target_dir: "/opt/a", format: "zip", strip_components: 2, overwrite: true });
  });

  it("等待节点按模式只写出匹配的字段", () => {
    expect(serializeWaitConfig({ mode: "sleep", seconds: 30, poll_url: "https://x" })).toEqual({ seconds: 30 });
    expect(
      serializeWaitConfig({
        poll_file: "/tmp/done",
        interval: "2s",
        max_attempts: 10,
        headers: [{ name: "X-Token", value: "{{ .token }}" }],
      }),
    ).toEqual({
      poll_file: "/tmp/done",
      interval: "2s",
      max_attempts: 10,
      headers: { "X-Token": "{{ .token }}" },
    });
  });

  it("新节点序列化为对应类型，并能通过控制节点校验", () => {
    const nodes = [
      { id: "run", type: "controlNode", data: { controlType: "shell_script", name: "run", script: { script: "true" } } },
      { id: "sum", type: "controlNode", data: { controlType: "file_checksum", name: "sum", checksum: { path: "/tmp/a", algorithm: "md5" } } },
      { id: "unpack", type: "controlNode", data: { controlType: "archive", name: "unpack", archive: { path: "/tmp/a.zip", target_dir: "/tmp/out" } } },
      { id: "sleep", type: "controlNode", data: { controlType: "wait", name: "sleep", wait: { mode: "sleep", seconds: 2 } } },
    ];
    const draft = buildWorkflowDraft({ id: "demo.builtin" }, nodes, [], "global", []);
    expect(draft.nodes.map((item) => item.type)).toEqual(["shell", "file_checksum", "archive", "wait"]);
    expect(draft.nodes[0].script).toEqual({ script: "true" });
    expect(draft.nodes[1].checksum).toEqual({ path: "/tmp/a", algorithm: "md5" });
    expect(draft.nodes[2].archive).toEqual({ path: "/tmp/a.zip", target_dir: "/tmp/out" });
    expect(draft.nodes[3].wait).toEqual({ seconds: 2 });
    expect(validateControlDraft(nodes, [])).toEqual([]);
  });

  it("后端 shell 节点类型加载回画布为 shell_script", () => {
    expect(workflowControlNodeType("shell_script")).toBe("shell");
    expect(workflowControlNodeType("wait")).toBe("wait");
    expect(controlNodeTypeFromWorkflow("shell")).toBe("shell_script");
    expect(controlNodeTypeFromWorkflow("archive")).toBe("archive");
  });

  it("新节点缺失必填项时给出中文校验信息", () => {
    const nodes = [
      { id: "run", type: "controlNode", data: { controlType: "shell_script", script: { script: " " } } },
      { id: "sum", type: "controlNode", data: { controlType: "file_checksum", checksum: { path: "/tmp/a", algorithm: "sha256", expected: "abc" } } },
      { id: "unpack", type: "controlNode", data: { controlType: "archive", archive: { path: "/tmp/a.bin" } } },
      { id: "poll", type: "controlNode", data: { controlType: "wait", wait: { mode: "poll" } } },
    ];
    const errors = validateControlDraft(nodes, []).join("\n");
    expect(errors).toContain("run 的脚本内容不能为空");
    expect(errors).toContain("期望摘要长度应为 64 位");
    expect(errors).toContain("unpack 的目标目录不能为空");
    expect(errors).toContain("poll 请填写轮询 URL 或轮询文件");
  });
});

describe('通知节点配置', () => {
  it('邮件通道拆分收件人并丢弃其他通道字段', () => {
    const serialized = serializeNotifyConfig({channel: 'email', smtp_host: 'smtp.example.com', from: 'ops@example.com', to: 'a@example.com; b@example.com，c@example.com', url: 'https://ignored.example.com', message: '完成'})
    expect(serialized.to).toEqual(['a@example.com', 'b@example.com', 'c@example.com'])
    expect(serialized.smtp_port).toBeUndefined()
    expect(serialized.webhook).toBeUndefined()
  })

  it('webhook 头转换为对象，企业微信与 webhook 共用 webhook 字段', () => {
    const serialized = serializeNotifyConfig({channel: 'wecom', webhook: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc', headers: [{name: 'X-Env', value: 'prod'}, {name: '', value: 'skip'}]})
    expect(serialized).toEqual({channel: 'wecom', webhook: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc', headers: {'X-Env': 'prod'}})
  })

  it('校验缺少地址的通知节点', () => {
    const node = {id: 'alert', type: 'controlNode', data: {controlType: 'notify', notify: defaultNotifyConfig()}}
    const errors = validateControlDraft([node], [], [])
    expect(errors.some(error => error.includes('Webhook 地址'))).toBe(true)
  })
})

describe('人工审批节点配置', () => {
  it('默认配置两个字段都为空，空配置不写出任何字段', () => {
    expect(defaultApprovalConfig()).toEqual({message: '', timeout: ''})
    expect(serializeApprovalConfig(defaultApprovalConfig())).toEqual({})
    expect(serializeApprovalConfig(null)).toEqual({})
  })

  it('归一化去掉首尾空白，序列化只写有值的字段', () => {
    expect(normalizeApprovalConfig({message: '  确认配置已改好后继续  ', timeout: ' 30m '})).toEqual({message: '确认配置已改好后继续', timeout: '30m'})
    expect(serializeApprovalConfig({message: '确认配置已改好后继续', timeout: ''})).toEqual({message: '确认配置已改好后继续'})
    expect(serializeApprovalConfig({timeout: '2h'})).toEqual({timeout: '2h'})
  })

  it('审批节点序列化为 approval 类型，且不会被控制节点校验拦下', () => {
    const nodes = [
      {id: 'gate', type: 'controlNode', data: {controlType: 'approval', name: '人工确认', approval: {message: '确认配置已改好后继续', timeout: '30m'}}},
    ]
    const draft = buildWorkflowDraft({id: 'demo.approval'}, nodes, [], 'global', [])
    expect(draft.nodes[0].type).toBe('approval')
    expect(draft.nodes[0].approval).toEqual({message: '确认配置已改好后继续', timeout: '30m'})
    expect(validateControlDraft(nodes, [])).toEqual([])
    // 类型字符串原名直通，不需要额外的映射。
    expect(workflowControlNodeType('approval')).toBe('approval')
    expect(controlNodeTypeFromWorkflow('approval')).toBe('approval')
  })

  it('超时不是合法时长时给出明确中文错误，留空表示一直等待', () => {
    const bad = {id: 'gate', type: 'controlNode', data: {controlType: 'approval', approval: {timeout: '半小时'}}}
    expect(validateControlDraft([bad], []).join('\n')).toContain('人工审批节点 gate 的等待超时格式无效')
    const blank = {id: 'gate', type: 'controlNode', data: {controlType: 'approval', approval: {timeout: '  '}}}
    expect(validateControlDraft([blank], [])).toEqual([])
    const ok = {id: 'gate', type: 'controlNode', data: {controlType: 'approval', approval: {timeout: '1h30m'}}}
    expect(validateControlDraft([ok], [])).toEqual([])
  })
})
