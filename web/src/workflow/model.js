import {conditionOperators} from "./catalog.js";

export {conditionOperators};

export function defaultCondition() {
  return {
    input: "",
    cases: [
      { id: "case1", name: "分支 1", operator: "contains", values: [] },
      { id: "case2", name: "分支 2", operator: "contains", values: [] },
    ],
    default_case: "default",
  };
}

export function defaultLoop() {
  return { tool: "", params: {}, max_iterations: 3 };
}

export function defaultUpload() {
  return { target_dir: "" };
}

export function normalizeUploadConfig(upload) {
  return {
    target_dir: String(upload?.target_dir || upload?.targetDir || "").trim(),
  };
}

export function defaultExtractConfig() {
  return {
    source_type: "file",
    file_name: "",
    target_path: "",
    label: "",
    replace: false,
    source_dir: "",
    files: [],
  };
}

export function normalizeExtractConfig(extract) {
  const explicitSourceType = String(
    extract?.source_type || extract?.sourceType || "",
  ).trim();
  const sourceType = normalizeExtractSourceType(
    explicitSourceType,
  );
  if (explicitSourceType) {
    return sourceType === "directory"
      ? normalizeExtractDirectoryConfig(extract)
      : normalizeExtractFileConfig(extract);
  }
  const hasDirectoryShape =
    String(extract?.source_dir || extract?.sourceDir || "").trim() !== "" ||
    Array.isArray(extract?.files);
  if (!hasDirectoryShape) return normalizeExtractFileConfig(extract);
  return normalizeExtractDirectoryConfig(extract);
}

export function normalizeExtractConfigFile(file) {
  return {
    source_type: "file",
    file_name: normalizeRelativeSourceName(
      file?.file_name ||
        file?.fileName ||
        file?.source_path ||
        file?.sourcePath ||
        "",
    ),
    target_path: String(file?.target_path || file?.targetPath || "").trim(),
    label: String(file?.label || "").trim(),
    replace: file?.replace === true,
    source_dir: "",
    files: [],
  };
}

export function serializeExtractConfig(extract) {
  const normalized = normalizeExtractConfig(extract);
  if (normalized.source_type === "directory") {
    return {
      source_type: "directory",
      source_dir: normalized.source_dir,
      files: (normalized.files || []).map((item) => ({
        source_path: item.source_path,
        target_path: item.source_path,
        label: item.label,
        replace: item.replace,
      })),
    };
  }
  return {
    file_name: normalized.file_name,
    target_path: normalized.target_path,
    label: normalized.label,
    replace: normalized.replace,
  };
}

export function normalizeUploadTargetDir(value) {
  const raw = String(value || "").trim();
  if (!raw) return { value: "", error: "" };
  if (raw.includes("://"))
    return { value: "", error: "上传目标目录必须是 uploads 下的相对子目录。" };
  if (/^[A-Za-z]:/.test(raw) || raw.startsWith("/") || raw.startsWith("\\"))
    return { value: "", error: "上传目标目录不能是绝对路径。" };
  const normalized = raw.replace(/\\/g, "/");
  const parts = normalized.split("/");
  if (parts.some((part) => !part.trim() || part === "." || part === ".."))
    return { value: "", error: "上传目标目录不能包含空路径、. 或 ..。" };
  return { value: parts.join("/"), error: "" };
}

export function normalizeRelativeConfigPath(value, label = "配置文件路径") {
  const raw = String(value || "").trim();
  if (!raw) return { value: "", error: `${label}不能为空。` };
  if (raw.includes("://"))
    return { value: "", error: `${label}必须是相对路径。` };
  if (/^[A-Za-z]:/.test(raw) || raw.startsWith("/") || raw.startsWith("\\"))
    return { value: "", error: `${label}不能是绝对路径。` };
  const normalized = raw.replace(/\\/g, "/");
  const parts = normalized.split("/");
  if (parts.some((part) => !part.trim() || part === "." || part === ".."))
    return { value: "", error: `${label}不能包含空路径、. 或 ..。` };
  return { value: parts.join("/"), error: "" };
}

export function normalizeOptionalRelativeConfigPath(
  value,
  label = "配置文件路径",
) {
  const raw = String(value || "").trim();
  if (!raw) return { value: "", error: "" };
  return normalizeRelativeConfigPath(raw, label);
}

function normalizeRelativeSourceName(value) {
  const raw = String(value || "").trim();
  if (!raw) return "";
  return raw.replace(/\\/g, "/");
}

function normalizeExtractSourceType(value) {
  const raw = String(value || "").trim().toLowerCase();
  if (raw === "directory") return "directory";
  return "file";
}

function normalizeExtractFileConfig(extract) {
  return {
    source_type: "file",
    file_name: normalizeRelativeSourceName(
      extract?.file_name ||
        extract?.fileName ||
        extract?.source_path ||
        extract?.sourcePath ||
        "",
    ),
    target_path: String(extract?.target_path || extract?.targetPath || "").trim(),
    label: String(extract?.label || "").trim(),
    replace: extract?.replace === true,
    source_dir: "",
    files: [],
  };
}

function normalizeExtractDirectoryConfig(extract) {
  const sourceDir = normalizeOptionalRelativeConfigPath(
    extract?.source_dir || extract?.sourceDir || extract?.source_path || extract?.sourcePath || "",
    "源目录来源",
  ).value;
  const files = normalizeExtractDirectoryFiles(extract?.files);
  return {
    source_type: "directory",
    file_name: "",
    target_path: "",
    label: String(extract?.label || "").trim(),
    replace: extract?.replace === true,
    source_dir: sourceDir,
    files,
  };
}

function normalizeExtractDirectoryFiles(files) {
  return (Array.isArray(files) ? files : []).map(normalizeExtractDirectoryFile);
}

function normalizeExtractDirectoryFile(file) {
  const sourcePath = normalizeRelativeSourceName(
    file?.source_path ||
      file?.sourcePath ||
      file?.file_name ||
      file?.fileName ||
      "",
  );
  const targetPath = String(file?.target_path || file?.targetPath || "").trim();
  return {
    source_path: sourcePath,
    target_path: targetPath || sourcePath,
    label: String(file?.label || "").trim(),
    replace: file?.replace === true,
  };
}

export function normalizeLoopConfig(loop) {
  const params =
    loop?.params &&
    typeof loop.params === "object" &&
    !Array.isArray(loop.params)
      ? loop.params
      : {};
  return {
    tool: String(loop?.tool || "").trim(),
    params,
    max_iterations: clampLoopIterations(
      loop?.max_iterations || loop?.maxIterations || 1,
    ),
  };
}

export function clampLoopIterations(value) {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed)) return 1;
  return Math.min(20, Math.max(1, parsed));
}

export function workflowScopeCategory(value, fallbackCategory = "") {
  if (value === "global") return "global";
  return value || fallbackCategory || "global";
}

export function normalizeTags(tags) {
  const seen = new Set();
  const out = [];
  (Array.isArray(tags) ? tags : String(tags || "").split(/[\n,]/)).forEach(
    (item) => {
      const tag = String(item || "").trim();
      if (!tag || seen.has(tag)) return;
      seen.add(tag);
      out.push(tag);
    },
  );
  return out;
}

export function conditionBranchRows(condition) {
  const cases = condition?.cases || [];
  const rows = cases.map((item, index) => {
    const id = String(item.id || "").trim();
    return {
      key: `${id || "case"}-${index}`,
      handleID: id,
      label: item.name || id || `未命名分支 ${index + 1}`,
      meta: id ? `case: ${id}` : "请先填写 case ID",
      kind: "case",
      disabled: !id,
    };
  });
  if (condition?.default_case === "default") {
    rows.push({
      key: "default",
      handleID: "default",
      label: "默认分支",
      meta: "default",
      kind: "default",
      disabled: false,
    });
  } else {
    rows.push({
      key: "default-disabled",
      handleID: "",
      label: "默认分支",
      meta: "未启用",
      kind: "default",
      disabled: true,
    });
  }
  return rows;
}

export function conditionCaseLabel(condition, caseID) {
  if (!caseID) return "";
  if (caseID === "default") return "default";
  const item = (condition?.cases || []).find((item) => item.id === caseID);
  return item ? item.name || item.id : caseID;
}

export function buildWorkflowDraft(
  workflow,
  nodes,
  edges,
  category,
  parameters,
) {
  const draftEdges =
    (edges || []).length > 0 ? edges : buildSequentialFlowEdges(nodes);
  return {
    ...workflow,
    category: workflowScopeCategory(workflow.category, category),
    tags: normalizeTags(workflow.tags || []),
    parameters: parameters || workflow.parameters || [],
    nodes: nodes.map((node) => {
      if (node.type === "conditionNode") {
        return {
          id: node.id,
          type: "condition",
          name: node.data.name || node.id,
          condition: node.data.condition || defaultCondition(),
        };
      }
      if (node.type === "controlNode") {
        const draftNode = {
          id: node.id,
          type: node.data.controlType,
          name: node.data.name || node.id,
        };
        if (node.data.controlType === "loop")
          draftNode.loop = normalizeLoopConfig(node.data.loop || defaultLoop());
        if (node.data.controlType === "upload")
          draftNode.upload = normalizeUploadConfig(
            node.data.upload || defaultUpload(),
          );
        if (node.data.controlType === "extract_config")
          draftNode.extract_config = serializeExtractConfig(
            node.data.extract_config || defaultExtractConfig(),
          );
        return draftNode;
      }
      return {
        id: node.id,
        type: "tool",
        name: node.data.name || node.id,
        tool: node.data.tool,
        params: node.data.params || {},
        on_failure: node.data.on_failure || "stop",
      };
    }),
    edges: draftEdges.map((edge) => {
      const sourceNode = nodes.find((node) => node.id === edge.source);
      const out = { from: edge.source, to: edge.target };
      const edgeCase =
        sourceNode?.type === "conditionNode"
          ? edge.data?.case || edge.sourceHandle || ""
          : "";
      if (edgeCase) out.case = edgeCase;
      return out;
    }),
  };
}

export function canBuildSequentialFlowEdges(nodes, edges = []) {
  return (
    (nodes || []).length > 1 &&
    (edges || []).length === 0 &&
    (nodes || []).every(isLinearFlowNode)
  );
}

export function buildSequentialFlowEdges(nodes, edges = []) {
  if (!canBuildSequentialFlowEdges(nodes, edges)) return [];
  const ordered = orderedFlowNodes(nodes);
  return ordered.slice(1).map((node, index) => {
    const source = ordered[index];
    return {
      id: `${source.id}-${node.id}`,
      source: source.id,
      target: node.id,
    };
  });
}

function isLinearFlowNode(node) {
  if (node?.type === "toolNode") return true;
  return (
    node?.type === "controlNode" &&
    (node.data?.controlType === "loop" ||
      node.data?.controlType === "upload" ||
      node.data?.controlType === "extract_config")
  );
}

function orderedFlowNodes(nodes) {
  return [...(nodes || [])]
    .map((node, index) => ({ node, index }))
    .sort((left, right) => compareNodePosition(left, right))
    .map((item) => item.node);
}

function compareNodePosition(left, right) {
  const leftX = numberOrNull(left.node?.position?.x);
  const rightX = numberOrNull(right.node?.position?.x);
  const leftY = numberOrNull(left.node?.position?.y);
  const rightY = numberOrNull(right.node?.position?.y);
  if (leftX !== null && rightX !== null && leftX !== rightX)
    return leftX - rightX;
  if (leftY !== null && rightY !== null && leftY !== rightY)
    return leftY - rightY;
  if (leftX !== null && rightX === null) return -1;
  if (leftX === null && rightX !== null) return 1;
  return left.index - right.index;
}

function numberOrNull(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

export function autoLayoutNodes(nodes, edges) {
  if (!nodes.length) return nodes;
  const nodeIDs = new Set(nodes.map((node) => node.id));
  const nodeOrder = new Map(nodes.map((node, index) => [node.id, index]));
  const children = new Map(nodes.map((node) => [node.id, []]));
  const incomingCounts = new Map(nodes.map((node) => [node.id, 0]));
  const depths = new Map(nodes.map((node) => [node.id, 0]));

  (edges || []).forEach((edge) => {
    if (
      !nodeIDs.has(edge.source) ||
      !nodeIDs.has(edge.target) ||
      edge.source === edge.target
    )
      return;
    children.get(edge.source).push(edge.target);
    incomingCounts.set(edge.target, (incomingCounts.get(edge.target) || 0) + 1);
  });

  const queue = nodes
    .filter((node) => (incomingCounts.get(node.id) || 0) === 0)
    .map((node) => node.id);
  const visited = new Set();
  while (queue.length > 0) {
    const nodeID = queue.shift();
    if (visited.has(nodeID)) continue;
    visited.add(nodeID);
    const nextDepth = (depths.get(nodeID) || 0) + 1;
    (children.get(nodeID) || [])
      .sort(
        (left, right) =>
          (nodeOrder.get(left) || 0) - (nodeOrder.get(right) || 0),
      )
      .forEach((childID) => {
        depths.set(childID, Math.max(depths.get(childID) || 0, nextDepth));
        const nextIncoming = (incomingCounts.get(childID) || 0) - 1;
        incomingCounts.set(childID, nextIncoming);
        if (nextIncoming === 0) queue.push(childID);
      });
  }
  if (visited.size < nodes.length) {
    const fallbackDepth = Math.max(0, ...Array.from(depths.values())) + 1;
    nodes.forEach((node) => {
      if (visited.has(node.id)) return;
      depths.set(node.id, fallbackDepth + (nodeOrder.get(node.id) || 0));
    });
  }

  const layers = new Map();
  nodes.forEach((node) => {
    const depth = depths.get(node.id) || 0;
    if (!layers.has(depth)) layers.set(depth, []);
    layers.get(depth).push(node);
  });
  const orderedLayers = Array.from(layers.entries())
    .sort(([left], [right]) => left - right)
    .map(([, layerNodes]) =>
      layerNodes.sort(
        (left, right) =>
          (nodeOrder.get(left.id) || 0) - (nodeOrder.get(right.id) || 0),
      ),
    );
  const layerMetrics = orderedLayers.map((layerNodes) => {
    const sizes = layerNodes.map(autoLayoutNodeSize);
    const height =
      sizes.reduce((total, size) => total + size.height, 0) +
      Math.max(0, sizes.length - 1) * 42;
    const width = Math.max(...sizes.map((size) => size.width));
    return { sizes, height, width };
  });
  const maxLayerHeight = Math.max(
    ...layerMetrics.map((metric) => metric.height),
    0,
  );
  const positions = new Map();
  let x = 80;
  orderedLayers.forEach((layerNodes, layerIndex) => {
    const metrics = layerMetrics[layerIndex];
    let y = 80 + Math.max(0, (maxLayerHeight - metrics.height) / 2);
    layerNodes.forEach((node, nodeIndex) => {
      const size = metrics.sizes[nodeIndex];
      positions.set(node.id, { x, y });
      y += size.height + 42;
    });
    x += metrics.width + 140;
  });
  return nodes.map((node) => ({
    ...node,
    position: positions.get(node.id) || node.position || { x: 80, y: 80 },
  }));
}

export function autoLayoutNodeSize(node) {
  if (node.type === "conditionNode") {
    const branchCount = conditionBranchRows(
      node.data.condition || defaultCondition(),
    ).length;
    return { width: 440, height: Math.max(156, 72 + branchCount * 42) };
  }
  if (node.type === "controlNode") return { width: 250, height: 82 };
  return { width: 210, height: 74 };
}

export function validateConditionDraft(nodes, edges) {
  const errors = [];
  const nodeMap = new Map(nodes.map((node) => [node.id, node]));
  nodes
    .filter((node) => node.type === "conditionNode")
    .forEach((node) => {
      const condition = node.data.condition || {};
      if (!String(condition.input || "").trim())
        errors.push(`条件节点 ${node.id} 缺少输入来源。`);
      if (!condition.cases || condition.cases.length === 0)
        errors.push(`条件节点 ${node.id} 至少需要一个 case。`);
      const seen = new Set();
      (condition.cases || []).forEach((item) => {
        if (!String(item.id || "").trim())
          errors.push(`条件节点 ${node.id} 存在空 case ID。`);
        if (item.id === "default")
          errors.push(
            `条件节点 ${node.id} 的 case ID 不能使用保留值 default。`,
          );
        if (seen.has(item.id))
          errors.push(`条件节点 ${node.id} 的 case ID 重复：${item.id}`);
        seen.add(item.id);
        if (
          !conditionOperators.some(
            (operator) => operator.value === item.operator,
          )
        )
          errors.push(
            `条件节点 ${node.id} 的 case ${item.id || "-"} 操作符非法。`,
          );
      });
      edges
        .filter((edge) => edge.source === node.id)
        .forEach((edge) => {
          const edgeCase = edge.data?.case || edge.sourceHandle || "";
          if (!edgeCase)
            errors.push(
              `条件节点 ${node.id} 到 ${edge.target} 的连线缺少 case。`,
            );
          if (edgeCase === "default" && condition.default_case !== "default")
            errors.push(
              `条件节点 ${node.id} 未启用 default 分支，但到 ${edge.target} 的连线选择了 default。`,
            );
          if (
            edgeCase &&
            edgeCase !== "default" &&
            !(condition.cases || []).some((item) => item.id === edgeCase)
          )
            errors.push(
              `条件节点 ${node.id} 到 ${edge.target} 的连线引用不存在的 case：${edgeCase}`,
            );
        });
    });
  edges.forEach((edge) => {
    const source = nodeMap.get(edge.source);
    if (source?.type !== "conditionNode" && edge.data?.case)
      errors.push(`非条件节点 ${edge.source} 的连线不能配置 case。`);
  });
  return errors;
}

export function validateControlDraft(nodes, edges, tools = []) {
  const errors = [];
  const toolMap = new Map((tools || []).map((tool) => [tool.id, tool]));
  nodes
    .filter((node) => node.type === "controlNode")
    .forEach((node) => {
      if (node.data.controlType === "loop") {
        const loop = normalizeLoopConfig(node.data.loop || {});
        if (!loop.tool) errors.push(`循环节点 ${node.id} 请选择循环工具。`);
        if (loop.tool && !toolMap.has(loop.tool))
          errors.push(`循环节点 ${node.id} 引用了不存在的工具：${loop.tool}`);
        if (
          !Number.isInteger(loop.max_iterations) ||
          loop.max_iterations < 1 ||
          loop.max_iterations > 20
        )
          errors.push(
            `循环节点 ${node.id} 的最大循环次数必须在 1 到 20 之间。`,
          );
      }
      if (node.data.controlType === "upload") {
        const targetDir = normalizeUploadTargetDir(
          node.data.upload?.target_dir || "",
        );
        if (targetDir.error)
          errors.push(
            `上传节点 ${node.id} 的目标子目录无效：${targetDir.error}`,
          );
      }
      if (node.data.controlType === "extract_config") {
        const extract = normalizeExtractConfig(node.data.extract_config || {});
        if (extract.source_type === "directory") {
          if (!String(extract.source_dir || "").trim())
            errors.push(`提取配置节点 ${node.id} 的源目录来源不能为空。`);
          if (extract.source_dir && !extract.source_dir.includes("{{")) {
            const sourceDir = normalizeRelativeConfigPath(
              extract.source_dir || "",
              "源目录来源",
            );
            if (sourceDir.error)
              errors.push(
                `提取配置节点 ${node.id} 的源目录来源无效：${sourceDir.error}`,
              );
          }
          if (!Array.isArray(extract.files) || extract.files.length === 0)
            errors.push(`提取配置节点 ${node.id} 的目录文件列表不能为空。`);
          (extract.files || []).forEach((item, index) => {
            if (!String(item.source_path || "").trim()) {
              errors.push(
                `提取配置节点 ${node.id} 的目录文件 ${index + 1} 源相对文件路径不能为空。`,
              );
              return;
            }
            if (String(item.source_path).includes("{{")) {
              errors.push(
                `提取配置节点 ${node.id} 的目录文件 ${index + 1} 源相对文件路径不能包含模板。`,
              );
            }
            const source = normalizeRelativeConfigPath(
              item.source_path || "",
              "源相对文件路径",
            );
            if (source.error)
              errors.push(
                `提取配置节点 ${node.id} 的目录文件 ${index + 1} 源相对文件路径无效：${source.error}`,
              );
            const target = normalizeRelativeConfigPath(
              item.target_path || item.source_path || "",
              "目标配置路径",
            );
            if (target.error)
              errors.push(
                `提取配置节点 ${node.id} 的目录文件 ${index + 1} 目标配置路径无效：${target.error}`,
              );
          });
        } else {
          if (!extract.file_name)
            errors.push(`提取配置节点 ${node.id} 的源文件来源不能为空。`);
          if (extract.file_name && !extract.file_name.includes("{{")) {
            const source = normalizeRelativeConfigPath(
              extract.file_name || "",
              "源文件名",
            );
            if (source.error)
              errors.push(
                `提取配置节点 ${node.id} 的源文件名无效：${source.error}`,
              );
          }
          const target = normalizeRelativeConfigPath(
            extract.target_path || "",
            "目标配置路径",
          );
          if (target.error)
            errors.push(
              `提取配置节点 ${node.id} 的目标配置路径无效：${target.error}`,
            );
        }
      }
      if (
        node.data.controlType === "parallel" &&
        !edges.some((edge) => edge.source === node.id)
      )
        errors.push(`并行分支节点 ${node.id} 至少需要一条出边。`);
      if (
        node.data.controlType === "join" &&
        !edges.some((edge) => edge.target === node.id)
      )
        errors.push(`合流节点 ${node.id} 至少需要一条入边。`);
    });
  return errors;
}

export function defaultParams(parameters) {
  const out = {};
  (parameters || []).forEach((param) => {
    out[param.name] =
      param.default === undefined || param.default === null
        ? ""
        : param.default;
  });
  return out;
}

export function parseJSONList(value) {
  try {
    const parsed = JSON.parse(value || "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function findOutOfScopeToolNodes(nodes, tools, scopedCategory) {
  if (!scopedCategory) return [];
  const toolMap = new Map((tools || []).map((tool) => [tool.id, tool]));
  return nodes
    .filter(
      (node) =>
        node.type === "toolNode" ||
        (node.type === "controlNode" && node.data.controlType === "loop"),
    )
    .map((node) => {
      const toolID =
        node.type === "controlNode" ? node.data.loop?.tool : node.data.tool;
      return { node, toolID, tool: toolMap.get(toolID) };
    })
    .filter((item) => item.tool && item.tool.category !== scopedCategory)
    .map((item) => ({
      nodeID: item.node.id,
      toolID: item.toolID,
      scopeName: scopedCategory,
    }));
}

export function findMissingRequiredNodeParams(nodes, tools) {
  const toolMap = new Map((tools || []).map((tool) => [tool.id, tool]));
  const missing = [];
  nodes.forEach((node) => {
    if (node.type === "toolNode") {
      const tool = toolMap.get(node.data.tool);
      (tool?.parameters || []).forEach((param) => {
        if (!param.required) return;
        const value = node.data.params?.[param.name];
        if (
          value === undefined ||
          value === null ||
          String(value).trim() === ""
        ) {
          missing.push({
            nodeID: node.id,
            toolName: tool.name || tool.id,
            paramName: param.name,
          });
        }
      });
      return;
    }
    if (node.type === "controlNode" && node.data.controlType === "loop") {
      const loop = normalizeLoopConfig(node.data.loop || defaultLoop());
      const tool = toolMap.get(loop.tool);
      (tool?.parameters || []).forEach((param) => {
        if (!param.required) return;
        const value = loop.params?.[param.name];
        if (
          value === undefined ||
          value === null ||
          String(value).trim() === ""
        ) {
          missing.push({
            nodeID: node.id,
            toolName: tool.name || tool.id,
            paramName: param.name,
          });
        }
      });
    }
  });
  return missing;
}

export function findMissingRequiredWorkflowParams(parameters, params) {
  const values =
    params && typeof params === "object" && !Array.isArray(params)
      ? params
      : {};
  return (parameters || [])
    .filter((param) => param?.required && param.name)
    .filter((param) => {
      const value = values[param.name];
      if (param.type === "bool")
        return value === undefined || value === null || value === "";
      return (
        value === undefined || value === null || String(value).trim() === ""
      );
    })
    .map((param) => ({ name: param.name }));
}

export function buildMappingSources(
  workflowParameters,
  selectedNodeID,
  nodes,
  edges,
  tools = [],
) {
  const sources = [];
  const toolMap = new Map((tools || []).map((tool) => [tool.id, tool]));
  (workflowParameters || []).forEach((param) => {
    if (param?.name)
      sources.push({
        label: `工作流参数 / ${param.name}`,
        value: `{{ .${param.name} }}`,
      });
  });
  upstreamNodeIDs(selectedNodeID, edges).forEach((nodeID) => {
    const node = nodes.find((item) => item.id === nodeID);
    if (!node) return;
    sources.push({
      label: `${nodeID} / 标准输出 stdout`,
      value: `{{ .steps.${nodeID}.stdout }}`,
    });
    sources.push({
      label: `${nodeID} / 错误输出 stderr`,
      value: `{{ .steps.${nodeID}.stderr }}`,
    });
    if (node.type === "controlNode" && node.data?.controlType === "upload") {
      sources.push({
        label: `${nodeID} / 首个上传文件名`,
        value: `{{ .steps.${nodeID}.file.filename }}`,
      });
      sources.push({
        label: `${nodeID} / 首个上传文件相对路径`,
        value: `{{ .steps.${nodeID}.file.relative_path }}`,
      });
      sources.push({
        label: `${nodeID} / 上传目录`,
        value: `{{ .steps.${nodeID}.file.dir }}`,
      });
      sources.push({
        label: `${nodeID} / 上传目录（相对路径）`,
        value: `{{ .steps.${nodeID}.file.relative_dir }}`,
      });
      sources.push({
        label: `${nodeID} / 全部文件路径`,
        value: `{{ .steps.${nodeID}.files.paths }}`,
      });
      sources.push({
        label: `${nodeID} / 全部文件名`,
        value: `{{ .steps.${nodeID}.files.filenames }}`,
      });
    }
    const tool = node.type === "toolNode" ? toolMap.get(node.data?.tool) : null;
    (tool?.outputs || []).forEach((output) => {
      if (!output?.name) return;
      sources.push({
        label: `${nodeID} / 输出参数 ${output.name}`,
        value: `{{ .steps.${nodeID}.outputs.${output.name} }}`,
      });
    });
    Object.keys(node.data.params || {}).forEach((name) => {
      sources.push({
        label: `${nodeID} / 输入参数 ${name}`,
        value: `{{ .steps.${nodeID}.params.${name} }}`,
      });
    });
  });
  return sources;
}

export function buildUpstreamToolOutputSources(
  selectedNodeID,
  nodes,
  edges,
  tools = [],
  runNodes = {},
) {
  const sources = [];
  const toolMap = new Map((tools || []).map((tool) => [tool.id, tool]));
  upstreamNodeIDs(selectedNodeID, edges).forEach((nodeID) => {
    const node = nodes.find((item) => item.id === nodeID);
    const tool =
      node?.type === "toolNode" ? toolMap.get(node.data?.tool) : null;
    (tool?.outputs || []).forEach((output) => {
      if (!output?.name) return;
      const preview = previewOutputValue(
        runNodes?.[nodeID]?.stdout,
        output.json_path || output.name,
      );
      sources.push({
        label: `${nodeID} / 输出参数 ${output.name}${output.description ? `（${output.description}）` : ""}`,
        value: `{{ .steps.${nodeID}.outputs.${output.name} }}`,
        kind: output.type || "string",
        preview,
      });
    });
  });
  return sources;
}

export function previewOutputValue(stdout, jsonPath) {
  const payload = lastStdoutJSONObject(stdout);
  if (!payload) return "";
  const value = valueAtDotPath(payload, jsonPath);
  if (value === undefined || value === null) return "";
  if (Array.isArray(value))
    return value.map((item) => previewValueText(item)).join(",");
  return previewValueText(value);
}

function lastStdoutJSONObject(stdout) {
  const lines = String(stdout || "").split(/\r?\n/);
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const line = lines[index].trim();
    if (!line) continue;
    try {
      const parsed = JSON.parse(line);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed))
        return parsed;
    } catch {}
  }
  return null;
}

function valueAtDotPath(payload, path) {
  let current = payload;
  for (const part of String(path || "").split(".")) {
    const key = part.trim();
    if (
      !key ||
      !current ||
      typeof current !== "object" ||
      Array.isArray(current) ||
      !(key in current)
    )
      return undefined;
    current = current[key];
  }
  return current;
}

function previewValueText(value) {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean")
    return String(value);
  return JSON.stringify(value);
}

function upstreamNodeIDs(selectedNodeID, edges) {
  if (!selectedNodeID) return [];
  const direct = edges
    .filter((edge) => edge.target === selectedNodeID)
    .map((edge) => edge.source);
  return Array.from(new Set(direct)).sort((a, b) =>
    a.localeCompare(b, "zh-CN"),
  );
}
