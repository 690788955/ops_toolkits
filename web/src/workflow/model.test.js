import { describe, expect, it } from "vitest";
import {
  buildMappingSources,
  buildUpstreamToolOutputSources,
  autoLayoutNodes,
  buildWorkflowDraft,
  defaultCondition,
  defaultExtractConfig,
  defaultUpload,
  normalizeExtractConfig,
  normalizeLoopConfig,
  validateConditionDraft,
  validateControlDraft,
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
