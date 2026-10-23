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

export const httpMethods = [
  "GET",
  "POST",
  "PUT",
  "PATCH",
  "DELETE",
  "HEAD",
  "OPTIONS",
];

export const httpBodyTypes = [
  { value: "none", label: "无请求体" },
  { value: "json", label: "JSON" },
  { value: "text", label: "纯文本" },
  { value: "form", label: "表单（已编码）" },
];

export function defaultHttpRequest() {
  return {
    method: "GET",
    url: "",
    headers: [],
    body_type: "none",
    body: "",
    expect_status: [],
    save_response: "",
    fail_on_error: true,
    timeout: "",
  };
}

export function normalizeHttpMethod(value) {
  const method = String(value || "").trim().toUpperCase();
  return httpMethods.includes(method) ? method : "GET";
}

export function normalizeHttpBodyType(value, body = "") {
  const bodyType = String(value || "").trim().toLowerCase();
  if (httpBodyTypes.some((item) => item.value === bodyType)) return bodyType;
  return String(body || "").trim() ? "json" : "none";
}

export const scriptFailOnExitOptions = [
  { value: true, label: "非 0 退出码判定失败" },
  { value: false, label: "忽略退出码，仅记录输出" },
];

export function defaultScriptConfig() {
  return {
    script: "",
    workdir: "",
    env: [],
    fail_on_exit: true,
    timeout: "",
  };
}

export function normalizeScriptConfig(script) {
  const source = script && typeof script === "object" ? script : {};
  const rawEnv = source.env;
  const envRows = Array.isArray(rawEnv)
    ? rawEnv
    : rawEnv && typeof rawEnv === "object"
      ? Object.entries(rawEnv).map(([name, value]) => ({ name, value }))
      : [];
  return {
    script: String(source.script || ""),
    workdir: String(source.workdir || "").trim(),
    env: envRows
      .map((item) => ({
        name: String(item?.name ?? item?.key ?? "").trim(),
        value: String(item?.value ?? ""),
      })),
    fail_on_exit: source.fail_on_exit !== false,
    timeout: String(source.timeout || "").trim(),
  };
}

export function serializeScriptConfig(script) {
  const normalized = normalizeScriptConfig(script);
  const out = { script: normalized.script };
  if (normalized.workdir) out.workdir = normalized.workdir;
  const env = {};
  normalized.env.forEach((item) => {
    if (item.name) env[item.name] = item.value;
  });
  if (Object.keys(env).length > 0) out.env = env;
  if (normalized.fail_on_exit === false) out.fail_on_exit = false;
  if (normalized.timeout) out.timeout = normalized.timeout;
  return out;
}

export const checksumAlgorithms = [
  { value: "sha256", label: "SHA256" },
  { value: "md5", label: "MD5" },
];

export function defaultChecksumConfig() {
  return { path: "", algorithm: "sha256", expected: "" };
}

export function normalizeChecksumConfig(checksum) {
  const source = checksum && typeof checksum === "object" ? checksum : {};
  const algorithm = String(source.algorithm || "").trim().toLowerCase();
  return {
    path: String(source.path || "").trim(),
    algorithm: algorithm === "md5" ? "md5" : "sha256",
    expected: String(source.expected || "").includes("{{")
      ? String(source.expected || "").trim()
      : String(source.expected || "").trim().toLowerCase(),
  };
}

export function serializeChecksumConfig(checksum) {
  const normalized = normalizeChecksumConfig(checksum);
  const out = { path: normalized.path };
  if (normalized.algorithm !== "sha256") out.algorithm = normalized.algorithm;
  if (normalized.expected) out.expected = normalized.expected;
  return out;
}

export const archiveFormats = [
  { value: "", label: "自动识别（按扩展名）" },
  { value: "tar.gz", label: "tar.gz" },
  { value: "tgz", label: "tgz" },
  { value: "tar", label: "tar" },
  { value: "zip", label: "zip" },
];

export function defaultArchiveConfig() {
  return { path: "", target_dir: "", format: "", strip_components: 0, overwrite: false };
}

export function normalizeArchiveConfig(archive) {
  const source = archive && typeof archive === "object" ? archive : {};
  const format = String(source.format || "").trim().toLowerCase();
  const strip = Number.parseInt(String(source.strip_components ?? source.stripComponents ?? 0), 10);
  return {
    path: String(source.path || "").trim(),
    target_dir: String(source.target_dir || source.targetDir || "").trim(),
    format: archiveFormats.some((item) => item.value === format) ? format : "",
    strip_components: Number.isFinite(strip) && strip > 0 ? strip : 0,
    overwrite: source.overwrite === true,
  };
}

export function serializeArchiveConfig(archive) {
  const normalized = normalizeArchiveConfig(archive);
  const out = { path: normalized.path, target_dir: normalized.target_dir };
  if (normalized.format) out.format = normalized.format;
  if (normalized.strip_components > 0) out.strip_components = normalized.strip_components;
  if (normalized.overwrite) out.overwrite = true;
  return out;
}

export const waitModes = [
  { value: "sleep", label: "固定等待" },
  { value: "poll", label: "轮询条件" },
];

// Go 风格正时长文本，例如 30s、2m、1h30m；与后端 time.ParseDuration 的单位一致。
export function isDurationText(value) {
  const text = String(value || "").trim();
  if (!text) return false;
  if (!/^\+?(?:(?:\d+(?:\.\d*)?|\.\d+)(?:ns|us|µs|μs|ms|s|m|h))+$/.test(text)) return false;
  return [...text.matchAll(/(\d+(?:\.\d*)?|\.\d+)(?:ns|us|µs|μs|ms|s|m|h)/g)].some(match => Number(match[1]) > 0);
}

export function normalizeDurationText(value) {
  return String(value || "").trim();
}

export function archiveFormatFromPath(path) {
  const lower = String(path || "").trim().toLowerCase();
  if (lower.endsWith(".zip")) return "zip";
  if (lower.endsWith(".tar.gz") || lower.endsWith(".tgz")) return "tar.gz";
  if (lower.endsWith(".tar")) return "tar";
  return "";
}

export function defaultWaitConfig() {
  return {
    mode: "sleep",
    seconds: 5,
    poll_url: "",
    poll_file: "",
    expect_status: [],
    interval: "",
    max_attempts: 0,
    max_wait: "",
    headers: [],
  };
}

export function normalizeWaitConfig(wait) {
  const source = wait && typeof wait === "object" ? wait : {};
  const rawHeaders = source.headers;
  const headerRows = Array.isArray(rawHeaders)
    ? rawHeaders
    : rawHeaders && typeof rawHeaders === "object"
      ? Object.entries(rawHeaders).map(([name, value]) => ({ name, value }))
      : [];
  const seconds = Number(source.seconds);
  const pollURL = String(source.poll_url || source.pollUrl || "").trim();
  const pollFile = String(source.poll_file || source.pollFile || "").trim();
  // 兼容只写了 poll_* 的配置：只要有轮询目标就按轮询执行。
  const mode = source.mode === "poll" || source.mode === "sleep"
    ? source.mode
    : pollURL || pollFile
      ? "poll"
      : "sleep";
  const maxAttempts = Number.parseInt(String(source.max_attempts ?? source.maxAttempts ?? 0), 10);
  return {
    mode,
    seconds: Number.isFinite(seconds) && seconds > 0 ? seconds : 0,
    poll_url: pollURL,
    poll_file: pollFile,
    expect_status: normalizeHTTPStatusList(source.expect_status || source.expectStatus),
    interval: String(source.interval || "").trim(),
    max_attempts: Number.isFinite(maxAttempts) && maxAttempts > 0 ? maxAttempts : 0,
    max_wait: String(source.max_wait || source.maxWait || "").trim(),
    headers: headerRows
      .map((item) => ({
        name: String(item?.name ?? item?.key ?? "").trim(),
        value: String(item?.value ?? ""),
      }))
      .filter((item) => item.name || item.value),
  };
}

// 序列化只写出与模式匹配的字段：固定等待写 seconds，轮询写 poll_* 与次数/时长限制。
export function serializeWaitConfig(wait) {
  const normalized = normalizeWaitConfig(wait);
  if (normalized.mode !== "poll") {
    return normalized.seconds > 0 ? { seconds: normalized.seconds } : {};
  }
  const out = {};
  if (normalized.poll_url) out.poll_url = normalized.poll_url;
  if (normalized.poll_file) out.poll_file = normalized.poll_file;
  if (normalized.expect_status.length > 0) out.expect_status = normalized.expect_status;
  if (normalized.interval) out.interval = normalized.interval;
  if (normalized.max_attempts > 0) out.max_attempts = normalized.max_attempts;
  if (normalized.max_wait) out.max_wait = normalized.max_wait;
  const headers = {};
  normalized.headers.forEach((item) => {
    if (item.name) headers[item.name] = item.value;
  });
  if (Object.keys(headers).length > 0) out.headers = headers;
  return out;
}

export const notifyChannels = [
  { value: "webhook", label: "通用 Webhook" },
  { value: "wecom", label: "企业微信群机器人" },
  { value: "email", label: "邮件 (SMTP)" },
];

export const notifyTLSModes = [
  { value: "starttls", label: "STARTTLS（常用 587）" },
  { value: "tls", label: "TLS 直连（常用 465）" },
  { value: "none", label: "明文（仅内网）" },
];

export function defaultNotifyConfig() {
  return {
    channel: "webhook",
    title: "",
    message: "",
    url: "",
    webhook: "",
    headers: [],
    timeout: "",
    smtp_host: "",
    smtp_port: 0,
    smtp_user: "",
    smtp_pass: "",
    smtp_tls: "starttls",
    from: "",
    to: "",
    subject: "",
  };
}

export function normalizeNotifyConfig(notify) {
  const source = notify && typeof notify === "object" ? notify : {};
  const rawHeaders = source.headers;
  const headerRows = Array.isArray(rawHeaders)
    ? rawHeaders
    : rawHeaders && typeof rawHeaders === "object"
      ? Object.entries(rawHeaders).map(([name, value]) => ({ name, value }))
      : [];
  const channel = notifyChannels.some((item) => item.value === source.channel)
    ? source.channel
    : "webhook";
  const port = Number.parseInt(String(source.smtp_port ?? source.smtpPort ?? 0), 10);
  const recipients = Array.isArray(source.to) ? source.to.join(", ") : String(source.to || "");
  return {
    channel,
    title: String(source.title || ""),
    message: String(source.message || ""),
    url: String(source.url || "").trim(),
    webhook: String(source.webhook || "").trim(),
    headers: headerRows
      .map((item) => ({
        name: String(item?.name ?? item?.key ?? "").trim(),
        value: String(item?.value ?? ""),
      }))
      .filter((item) => item.name || item.value),
    timeout: String(source.timeout || "").trim(),
    smtp_host: String(source.smtp_host || source.smtpHost || "").trim(),
    smtp_port: Number.isFinite(port) && port > 0 ? port : 0,
    smtp_user: String(source.smtp_user || source.smtpUser || ""),
    smtp_pass: String(source.smtp_pass || source.smtpPass || ""),
    smtp_tls: notifyTLSModes.some((item) => item.value === source.smtp_tls) ? source.smtp_tls : "starttls",
    from: String(source.from || "").trim(),
    to: recipients,
    subject: String(source.subject || ""),
  };
}

// 只序列化当前通道需要的字段，切换通道后不会把旧通道的地址或凭据写进 YAML。
export function serializeNotifyConfig(notify) {
  const normalized = normalizeNotifyConfig(notify);
  const out = { channel: normalized.channel };
  if (normalized.title) out.title = normalized.title;
  if (normalized.message) out.message = normalized.message;
  if (normalized.timeout) out.timeout = normalized.timeout;
  if (normalized.channel === "email") {
    out.smtp_host = normalized.smtp_host;
    if (normalized.smtp_port > 0) out.smtp_port = normalized.smtp_port;
    if (normalized.smtp_user) out.smtp_user = normalized.smtp_user;
    if (normalized.smtp_pass) out.smtp_pass = normalized.smtp_pass;
    if (normalized.smtp_tls !== "starttls") out.smtp_tls = normalized.smtp_tls;
    if (normalized.from) out.from = normalized.from;
    out.to = splitMailRecipients(normalized.to);
    if (normalized.subject) out.subject = normalized.subject;
    return out;
  }
  out.webhook = normalized.webhook || normalized.url;
  const headers = {};
  normalized.headers.forEach((item) => {
    if (item.name) headers[item.name] = item.value;
  });
  if (Object.keys(headers).length > 0) out.headers = headers;
  return out;
}

export function splitMailRecipients(value) {
  return String(value || "")
    .split(/[,;，；\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function notifyDefaultPort(tls) {
  return tls === "tls" ? 465 : 587;
}

// 兼容 "200,201"、[200, 201] 与数字三种写法，范围限定 100..599 且去重。
export function normalizeHTTPStatusList(value) {
  const raw = Array.isArray(value)
    ? value
    : String(value || "").split(/[,，\s]+/);
  const out = [];
  raw.forEach((item) => {
    const code = Number.parseInt(String(item).trim(), 10);
    if (!Number.isFinite(code) || code < 100 || code > 599) return;
    if (!out.includes(code)) out.push(code);
  });
  return out;
}

export function formatHTTPStatusList(list) {
  return (list || []).join(", ");
}

export function normalizeHttpRequestConfig(http) {
  const source = http && typeof http === "object" ? http : {};
  const rawHeaders = source.headers;
  const headerRows = Array.isArray(rawHeaders)
    ? rawHeaders
    : rawHeaders && typeof rawHeaders === "object"
      ? Object.entries(rawHeaders).map(([name, value]) => ({ name, value }))
      : [];
  const body = String(source.body || "");
  return {
    method: normalizeHttpMethod(source.method),
    url: String(source.url || "").trim(),
    headers: headerRows
      .map((item) => ({
        name: String(item?.name ?? item?.key ?? "").trim(),
        value: String(item?.value ?? ""),
      }))
      .filter((item) => item.name || item.value),
    body_type: normalizeHttpBodyType(source.body_type || source.bodyType, body),
    body,
    expect_status: normalizeHTTPStatusList(
      source.expect_status || source.expectStatus,
    ),
    save_response: String(
      source.save_response || source.saveResponse || "",
    ).trim(),
    fail_on_error: source.fail_on_error !== false,
    timeout: String(source.timeout || "").trim(),
  };
}

export function serializeHttpRequest(http) {
  const normalized = normalizeHttpRequestConfig(http);
  const out = {
    method: normalized.method,
    url: normalized.url,
  };
  const headers = {};
  (normalized.headers || []).forEach((item) => {
    if (item.name) headers[item.name] = item.value;
  });
  if (Object.keys(headers).length > 0) out.headers = headers;
  if (normalized.body_type !== "none") {
    out.body_type = normalized.body_type;
    out.body = normalized.body;
  }
  if (normalized.expect_status.length > 0)
    out.expect_status = normalized.expect_status;
  if (normalized.fail_on_error === false) out.fail_on_error = false;
  if (normalized.save_response) out.save_response = normalized.save_response;
  if (normalized.timeout) out.timeout = normalized.timeout;
  return out;
}

// 返回空字符串表示 URL 合法；模板 URL 在运行期才能校验，这里只做静态检查。
export function httpUrlError(value) {
  const url = String(value || "").trim();
  if (!url) return "请输入请求 URL。";
  if (url.includes("{{")) return "";
  if (!/^https?:\/\//i.test(url)) return "URL 只支持 http:// 或 https://。";
  return "";
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

export function defaultWorkflowStages() {
  return [
    { id: "prepare", name: "准备", execution_mode: "serial" },
    { id: "change", name: "变更", execution_mode: "serial" },
    { id: "verify", name: "验证", execution_mode: "serial" },
  ];
}

export function normalizeWorkflowStages(stages) {
  const seen = new Set();
  return (Array.isArray(stages) ? stages : [])
    .map((stage, index) => {
      const id = String(stage?.id || "").trim();
      const name = String(stage?.name || "").trim() || id;
      const mode = stage?.execution_mode === "parallel" ? "parallel" : "serial";
      const { max_concurrency: _concurrency, ...rest } = stage || {};
      const normalized = {...rest, id, name, execution_mode: mode};
      const limit = normalizeStageConcurrency(_concurrency);
      if (limit > 0) normalized.max_concurrency = limit;
      return normalized;
    })
    .filter((stage) => {
      if (!stage.id || seen.has(stage.id)) return false;
      seen.add(stage.id);
      return true;
    })
    .map(({ _index, ...stage }) => stage);
}

export function uniqueStageID(stages, base = "stage") {
  const used = new Set((stages || []).map((stage) => stage.id));
  let candidate = base;
  let suffix = 2;
  while (used.has(candidate)) {
    candidate = `${base}-${suffix}`;
    suffix += 1;
  }
  return candidate;
}

export function nodeStageID(node) {
  return String(node?.data?.stageId || "");
}

export function nodeStageOrder(node) {
  const value = Number(node?.data?.order);
  return Number.isFinite(value) && value > 0 ? value : 0;
}

export function orderedStageNodes(nodes, stageId) {
  return (nodes || [])
    .map((node, index) => ({ node, index }))
    .filter((item) => nodeStageID(item.node) === stageId)
    .sort(
      (left, right) =>
        nodeStageOrder(left.node) - nodeStageOrder(right.node) ||
        left.index - right.index,
    )
    .map((item) => item.node);
}

export function orderedWorkflowNodes(nodes, stages) {
  return (stages || []).flatMap((stage) => orderedStageNodes(nodes, stage.id));
}

export function ensureStageAssignments(nodes, stages) {
  if (!(stages || []).length) return nodes;
  const stageIDs = new Set(stages.map((stage) => stage.id));
  const fallbackStageID = stages[0].id;
  let nextOrder = nextStageOrder(nodes, fallbackStageID);
  return (nodes || []).map((node) => {
    if (stageIDs.has(nodeStageID(node))) return node;
    const order = nextOrder;
    nextOrder += 10;
    return {
      ...node,
      data: { ...node.data, stageId: fallbackStageID, order },
    };
  });
}

// 新节点落在来源节点前面会破坏条件分支的先后；从来源节点所在阶段开始往后找第一个不新增冲突的阶段，
// 入参候选节点的原 order 属于原阶段，换阶段后必须按目标阶段末尾重新编号。
export function relocateNodeStage(nodes, stages, candidate, options = {}) {
  const list = stages || [];
  if (!list.length) return null;
  const items = nodes || [];
  const stageIndex = new Map(list.map((stage, index) => [stage.id, index]));
  const after = options.afterNodeID
    ? items.find((item) => item.id === options.afterNodeID)
    : null;
  const start = Math.max(0, stageIndex.get(nodeStageID(after)) ?? 0);
  const edges = options.edges || [];
  const validate = options.validate || ((all) => validateStageDraft(all, edges, list));
  const baseline = validate(items).length;
  const choices = list.filter((stage, index) => index >= start && stage.id !== options.limitID);
  for (const stage of choices) {
    const order = nextStageOrder(items, stage.id);
    const errors = validate([...items, { ...candidate, data: { ...candidate?.data, stageId: stage.id, order } }]);
    if (errors.length <= baseline) return { stageId: stage.id, order };
  }
  return null;
}

export function normalizeStageNodeOrders(nodes, stages) {
  const stageIDs = new Set((stages || []).map((stage) => stage.id));
  const orders = new Map();
  (stages || []).forEach((stage) => {
    orderedStageNodes(nodes, stage.id).forEach((node, index) => {
      orders.set(node.id, (index + 1) * 10);
    });
  });
  return (nodes || []).map((node) => {
    if (!stageIDs.has(nodeStageID(node))) return node;
    const order = orders.get(node.id);
    if (node.data?.order === order) return node;
    return { ...node, data: { ...node.data, order } };
  });
}

export function nextStageOrder(nodes, stageId) {
  const order = orderedStageNodes(nodes, stageId).reduce(
    (max, node) => Math.max(max, nodeStageOrder(node)),
    0,
  );
  return order + 10;
}

export function moveNodeToStage(nodes, nodeId, stageId) {
  return moveNodesToStage(nodes, [nodeId], stageId);
}

// 批量移动按给定顺序依次追加到目标阶段末尾；目标阶段里被选中的节点提到最前，
// 其余原位节点保持在后面，这样整段移动时阶段内相对顺序不变，单节点前移时也不会插回原位。
export function moveNodesToStage(nodes, nodeIds, stageId) {
  const existing = new Set((nodes || []).map((node) => node.id));
  const ids = new Set((nodeIds || []).filter((id) => existing.has(id)));
  if (ids.size === 0) return nodes;
  const staying = orderedStageNodes(nodes, stageId);
  const selected = staying.filter((node) => ids.has(node.id));
  const rest = staying.filter((node) => !ids.has(node.id));
  const ordered = [...selected, ...rest, ...(nodes || []).filter((node) => ids.has(node.id) && nodeStageID(node) !== stageId)];
  const orderByID = new Map(ordered.map((node, index) => [node.id, (index + 1) * 10]));
  return (nodes || []).map((node) => {
    const order = orderByID.get(node.id);
    if (order == null) return node;
    if (nodeStageID(node) === stageId && nodeStageOrder(node) === order) return node;
    return { ...node, data: { ...node.data, stageId, order } };
  });
}

export function moveNodeWithinStage(nodes, nodeId, direction) {
  const node = (nodes || []).find((item) => item.id === nodeId);
  if (!node) return nodes;
  const stageNodes = orderedStageNodes(nodes, nodeStageID(node));
  const index = stageNodes.findIndex((item) => item.id === nodeId);
  const target = index + direction;
  if (index < 0 || target < 0 || target >= stageNodes.length) return nodes;
  const swapped = stageNodes[target];
  const currentOrder = nodeStageOrder(node) || (index + 1) * 10;
  const targetOrder = nodeStageOrder(swapped) || (target + 1) * 10;
  return nodes.map((item) => {
    if (item.id === nodeId)
      return { ...item, data: { ...item.data, order: targetOrder } };
    if (item.id === swapped.id)
      return { ...item, data: { ...item.data, order: currentOrder } };
    return item;
  });
}

export const stageExecutionModes = [
  { value: "serial", label: "串行" },
  { value: "parallel", label: "并行" },
];

// 画布用 shell_script 区分「内置脚本」和 Shell 形状的分类，后端节点类型统一叫 shell。
export function workflowControlNodeType(controlType) {
  return controlType === "shell_script" ? "shell" : controlType;
}

export function controlNodeTypeFromWorkflow(nodeType) {
  return nodeType === "shell" ? "shell_script" : nodeType;
}

export const maxStageConcurrency = 16;

export function stageExecutionMode(stage) {
  return stage?.execution_mode === "parallel" ? "parallel" : "serial";
}

// 0 表示由后端默认并发（4）；返回值域 0..maxStageConcurrency。
export function normalizeStageConcurrency(value) {
  const parsed = Number.parseInt(String(value ?? ""), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.min(parsed, maxStageConcurrency);
}

export function stageModeLabel(stage) {
  if (stageExecutionMode(stage) !== "parallel") return "串行";
  const limit = normalizeStageConcurrency(stage?.max_concurrency);
  return limit > 0 ? `并行 · 最多 ${limit} 个` : "并行";
}

// 并行阶段切换回串行时，按显式依赖重新编号（稳定拓扑序），返回新的节点数组。
export function renumberStageForSerial(nodes, edges, stageId) {
  const ordered = orderedStageNodes(nodes, stageId);
  const ids = ordered.map((node) => node.id);
  const inStage = new Set(ids);
  const indegree = new Map(ids.map((id) => [id, 0]));
  const children = new Map(ids.map((id) => [id, []]));
  (edges || []).forEach((edge) => {
    if (!inStage.has(edge.source) || !inStage.has(edge.target) || edge.source === edge.target) return;
    children.get(edge.source).push(edge.target);
    indegree.set(edge.target, (indegree.get(edge.target) || 0) + 1);
  });
  // 每次派发当前可执行的节点中顺序最小的那个：既尊重显式依赖，又尽量少移动原有顺序。
  const queue = ids.filter((id) => (indegree.get(id) || 0) === 0);
  const next = [];
  while (queue.length > 0) {
    const position = queue.reduce((best, id, index) => (ids.indexOf(id) < ids.indexOf(queue[best]) ? index : best), 0);
    const current = queue.splice(position, 1)[0];
    next.push(current);
    children.get(current).forEach((child) => {
      const remaining = (indegree.get(child) || 0) - 1;
      indegree.set(child, remaining);
      if (remaining === 0) queue.push(child);
    });
  }
  if (next.length !== ids.length) return nodes;
  const orderByID = new Map(next.map((id, index) => [id, (index + 1) * 10]));
  const updated = (nodes || []).map((node) => {
    const order = orderByID.get(node.id);
    if (!order || node.data?.order === order) return node;
    return { ...node, data: { ...node.data, order } };
  });
  return updated;
}

export function moveWorkflowStage(stages, stageId, direction) {
  const list = [...(stages || [])];
  const index = list.findIndex((stage) => stage.id === stageId);
  const target = index + direction;
  if (index < 0 || target < 0 || target >= list.length) return list;
  const [stage] = list.splice(index, 1);
  list.splice(target, 0, stage);
  return list;
}

export function stageLayout(nodes, stages) {
  const normalizedStages = normalizeWorkflowStages(stages);
  if (normalizedStages.length === 0) return { stages: [], positions: {} };
  const stageGap = 56;
  const stagePadding = 24;
  const headerHeight = 72;
  const nodeGap = 36;
  const footerHeight = 52;
  const positions = {};
  let x = 60;
  const layouts = normalizedStages.map((stage, index) => {
    const stageNodes = orderedStageNodes(nodes, stage.id);
    const sizes = stageNodes.map(autoLayoutNodeSize);
    const contentWidth = Math.max(0, ...sizes.map((size) => size.width));
    const width = Math.max(296, contentWidth + stagePadding * 2);
    const contentHeight =
      sizes.reduce((total, size) => total + size.height, 0) +
      Math.max(0, stageNodes.length - 1) * nodeGap;
    const height = Math.max(
      236,
      headerHeight + stagePadding + contentHeight + footerHeight,
    );
    let y = 72 + headerHeight + stagePadding;
    stageNodes.forEach((node, nodeIndex) => {
      const size = sizes[nodeIndex];
      positions[node.id] = {
        x: x + (width - size.width) / 2,
        y,
      };
      y += size.height + nodeGap;
    });
    const layout = {
      ...stage,
      index,
      x,
      y: 72,
      width,
      height,
      nodeCount: stageNodes.length,
    };
    x += width + stageGap;
    return layout;
  });
  return { stages: layouts, positions };
}

export function autoLayoutWorkflowNodes(nodes, edges, stages) {
  if (!(stages || []).length) return autoLayoutNodes(nodes, edges);
  const { positions } = stageLayout(nodes, stages);
  return (nodes || []).map((node) => ({
    ...node,
    position: positions[node.id] || node.position || { x: 80, y: 80 },
  }));
}

export function buildWorkflowDraft(
  workflow,
  nodes,
  edges,
  category,
  parameters,
  stages = workflow.stages,
) {
  const draftStages = normalizeWorkflowStages(stages);
  const hasStages = draftStages.length > 0;
  const originalByID = new Map(
    (workflow?.nodes || []).map((node) => [node.id, node]),
  );
  const draftEdges =
    (edges || []).length > 0
      ? edges
      : hasStages
        ? []
        : buildSequentialFlowEdges(nodes);
  const draftNodes = (nodes || []).map((node) => {
    const base = { ...node.data.sourceConfig, ...originalByID.get(node.id) };
    delete base.depends_on;
    if (hasStages) {
      base.stage_id = nodeStageID(node);
      base.order = nodeStageOrder(node);
    } else {
      delete base.stage_id;
      delete base.order;
    }
    if (node.type === "conditionNode") {
      delete base.host;
      delete base.tool;
      return {
        ...base,
        id: node.id,
        type: "condition",
        name: node.data.name || node.id,
        condition: node.data.condition || defaultCondition(),
      };
    }
    if (node.type === "controlNode") {
      delete base.tool;
      const draftNode = {
        ...base,
        id: node.id,
        type: workflowControlNodeType(node.data.controlType),
        name: node.data.name || node.id,
      };
      if (["loop", "shell_script", "file_checksum"].includes(node.data.controlType) && node.data.host)
        draftNode.host = node.data.host;
      else delete draftNode.host;
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
      if (node.data.controlType === "http_request")
        draftNode.http = serializeHttpRequest(
          node.data.http || defaultHttpRequest(),
        );
      if (node.data.controlType === "shell_script")
        draftNode.script = serializeScriptConfig(
          node.data.script || defaultScriptConfig(),
        );
      if (node.data.controlType === "file_checksum")
        draftNode.checksum = serializeChecksumConfig(
          node.data.checksum || defaultChecksumConfig(),
        );
      if (node.data.controlType === "archive")
        draftNode.archive = serializeArchiveConfig(
          node.data.archive || defaultArchiveConfig(),
        );
      if (node.data.controlType === "wait")
        draftNode.wait = serializeWaitConfig(
          node.data.wait || defaultWaitConfig(),
        );
      if (node.data.controlType === "notify")
        draftNode.notify = serializeNotifyConfig(
          node.data.notify || defaultNotifyConfig(),
        );
      return draftNode;
    }
    const draftNode = {
      ...base,
      id: node.id,
      type: "tool",
      name: node.data.name || node.id,
      tool: node.data.tool,
      params: node.data.params || {},
      on_failure: node.data.on_failure || "stop",
    };
    if (node.data.host) draftNode.host = node.data.host;
    else delete draftNode.host;
    return draftNode;
  });
  const nextWorkflow = { ...workflow };
  delete nextWorkflow.stages;
  return {
    ...nextWorkflow,
    ...(hasStages ? { stages: draftStages } : {}),
    category: workflowScopeCategory(workflow.category, category),
    tags: normalizeTags(workflow.tags || []),
    parameters: parameters || workflow.parameters || [],
    nodes: draftNodes,
    edges: draftEdges.map((edge) => {
      const sourceNode = nodes.find((node) => node.id === edge.source);
      const out = { ...edge.data?.sourceConfig, from: edge.source, to: edge.target };
      delete out.case;
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
      node.data?.controlType === "extract_config" ||
      node.data?.controlType === "http_request")
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
  if (node.type === "controlNode") return { width: 250, height: 104 };
  return { width: 228, height: 74 };
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

export function validateStageDraft(nodes, edges, stages = []) {
  const list = normalizeWorkflowStages(stages);
  const errors = [];
  if (list.length === 0) {
    if ((nodes || []).some((node) => nodeStageID(node)))
      errors.push("工作流包含阶段归属，但未定义阶段。");
    return errors;
  }
  const indexByID = new Map(list.map((stage, index) => [stage.id, index]));
  const nameByID = new Map(list.map((stage) => [stage.id, stage.name || stage.id]));
  const parallelStages = new Set(list.filter((stage) => stage.execution_mode === "parallel").map((stage) => stage.id));
  const nodeMap = new Map((nodes || []).map((node) => [node.id, node]));
  const orderKeys = new Map();
  (nodes || []).forEach((node) => {
    const nodeName = node.data?.name || node.id;
    const stageID = nodeStageID(node);
    if (!stageID) {
      errors.push(`节点 ${nodeName} 未归属阶段。`);
      return;
    }
    if (!indexByID.has(stageID)) {
      errors.push(`节点 ${nodeName} 引用了不存在的阶段：${stageID}。`);
      return;
    }
    if (parallelStages.has(stageID)) return;
    const order = nodeStageOrder(node);
    if (!order) {
      errors.push(`节点 ${nodeName} 缺少有效的阶段内顺序。`);
      return;
    }
    const key = `${stageID}:${order}`;
    if (orderKeys.has(key)) {
      errors.push(
        `阶段「${nameByID.get(stageID)}」内的步骤「${orderKeys.get(key)}」与「${nodeName}」顺序重复（${order}）。`,
      );
      return;
    }
    orderKeys.set(key, nodeName);
  });
  (edges || []).forEach((edge) => {
    const source = nodeMap.get(edge.source);
    const target = nodeMap.get(edge.target);
    if (!source || !target) return;
    const sourceStage = indexByID.get(nodeStageID(source));
    const targetStage = indexByID.get(nodeStageID(target));
    if (sourceStage == null || targetStage == null) return;
    const sourceName = source.data?.name || source.id;
    const targetName = target.data?.name || target.id;
    if (sourceStage > targetStage) {
      errors.push(
        `依赖「${sourceName}」→「${targetName}」与阶段顺序冲突：不能从「${nameByID.get(nodeStageID(source))}」回指「${nameByID.get(nodeStageID(target))}」。`,
      );
    }
    if (
      sourceStage === targetStage &&
      !parallelStages.has(nodeStageID(source)) &&
      nodeStageOrder(source) >= nodeStageOrder(target)
    ) {
      errors.push(
        `依赖「${sourceName}」→「${targetName}」与阶段「${nameByID.get(nodeStageID(source))}」的节点顺序冲突。`,
      );
    }
  });
  return [...errors, ...parallelStageCycleErrors(nodeMap, edges, list)];
}

// 增量改阶段（拖拽、批量移入）只需要关心新出现的冲突；沿用基线计数可避免旧问题挡住编辑。
export function newStageDraftErrors(nodes, nextNodes, edges, stages = []) {
  const before = new Map();
  validateStageDraft(nodes, edges, stages).forEach((error) => {
    before.set(error, (before.get(error) || 0) + 1);
  });
  return validateStageDraft(nextNodes, edges, stages).filter((error) => {
    const remaining = before.get(error) || 0;
    if (remaining <= 0) return true;
    before.set(error, remaining - 1);
    return false;
  });
}

// 并行阶段没有隐式串行链，节点只按显式依赖派发；阶段内依赖成环会让这些节点永久等待。
function parallelStageCycleErrors(nodeMap, edges, stages) {
  const errors = [];
  stages
    .filter((stage) => stageExecutionMode(stage) === "parallel")
    .forEach((stage) => {
      const ids = new Set();
      nodeMap.forEach((node, id) => {
        if (nodeStageID(node) === stage.id) ids.add(id);
      });
      const indegree = new Map([...ids].map((id) => [id, 0]));
      const children = new Map([...ids].map((id) => [id, []]));
      (edges || []).forEach((edge) => {
        if (!ids.has(edge.source) || !ids.has(edge.target) || edge.source === edge.target) return;
        children.get(edge.source).push(edge.target);
        indegree.set(edge.target, (indegree.get(edge.target) || 0) + 1);
      });
      const queue = [...ids].filter((id) => (indegree.get(id) || 0) === 0);
      let visited = 0;
      while (queue.length > 0) {
        const id = queue.shift();
        visited += 1;
        children.get(id).forEach((child) => {
          const next = (indegree.get(child) || 0) - 1;
          indegree.set(child, next);
          if (next === 0) queue.push(child);
        });
      }
      if (visited !== ids.size) {
        errors.push(
          `并行阶段「${stage.name || stage.id}」内的显式依赖存在环，节点将无法开始执行。`,
        );
      }
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
      if (node.data.controlType === "shell_script") {
        const script = normalizeScriptConfig(node.data.script || {});
        if (!script.script.trim())
          errors.push(`Shell 节点 ${node.id} 的脚本内容不能为空。`);
        if (script.timeout && !isDurationText(script.timeout))
          errors.push(`Shell 节点 ${node.id} 的超时格式无效，例如 30s、2m。`);
        script.env.forEach(item => {
          if ((item.name || item.value) && !/^[A-Za-z_][A-Za-z0-9_]*$/.test(item.name))
            errors.push(`Shell 节点 ${node.id} 的环境变量名「${item.name}」无效。`);
        });
      }
      if (node.data.controlType === "file_checksum") {
        const checksum = normalizeChecksumConfig(node.data.checksum || {});
        if (!checksum.path)
          errors.push(`文件校验节点 ${node.id} 的文件路径不能为空。`);
        const want = checksum.algorithm === "md5" ? 32 : 64;
        if (checksum.expected.includes("{{")) return;
        if (checksum.expected && !/^[0-9a-f]+$/.test(checksum.expected))
          errors.push(`文件校验节点 ${node.id} 的期望摘要只能包含十六进制字符。`);
        else if (checksum.expected && checksum.expected.length !== want)
          errors.push(
            `文件校验节点 ${node.id} 的期望摘要长度应为 ${want} 位（${checksum.algorithm.toUpperCase()}）。`,
          );
      }
      if (node.data.controlType === "archive") {
        const archive = normalizeArchiveConfig(node.data.archive || {});
        if (!archive.path)
          errors.push(`解压节点 ${node.id} 的压缩包路径不能为空。`);
        if (!archive.target_dir)
          errors.push(`解压节点 ${node.id} 的目标目录不能为空。`);
        if (!archive.format && archive.path && !archive.path.includes("{{") && !archiveFormatFromPath(archive.path))
          errors.push(
            `解压节点 ${node.id} 的路径看不出压缩格式，请显式选择 tar、tar.gz 或 zip。`,
          );
      }
      if (node.data.controlType === "wait") {
        const wait = normalizeWaitConfig(node.data.wait || {});
        if (wait.mode === "poll") {
          if (!wait.poll_url && !wait.poll_file)
            errors.push(`等待节点 ${node.id} 请填写轮询 URL 或轮询文件。`);
          if (wait.poll_url && wait.poll_file)
            errors.push(`等待节点 ${node.id} 的轮询 URL 与轮询文件只能填写一个。`);
          if (wait.poll_url && !wait.poll_url.includes("{{") && !/^https?:\/\//i.test(wait.poll_url))
            errors.push(`等待节点 ${node.id} 的轮询 URL 只支持 http:// 或 https://。`);
          if (wait.interval && !isDurationText(wait.interval))
            errors.push(`等待节点 ${node.id} 的轮询间隔格式无效，例如 5s、1m。`);
          if (wait.max_wait && !isDurationText(wait.max_wait))
            errors.push(`等待节点 ${node.id} 的最大等待时长格式无效，例如 5m。`);
        } else if (!(wait.seconds > 0) || wait.seconds > 3600) {
          errors.push(`等待节点 ${node.id} 的固定等待秒数必须在 0 到 3600 之间。`);
        }
      }
      if (node.data.controlType === "notify") {
        const notify = normalizeNotifyConfig(node.data.notify || {});
        if (notify.timeout && !isDurationText(notify.timeout))
          errors.push(`通知节点 ${node.id} 的发送超时格式无效，例如 10s、30s。`);
        if (notify.channel === "email") {
          if (!notify.smtp_host) errors.push(`通知节点 ${node.id} 请填写 SMTP 服务器。`);
          if (splitMailRecipients(notify.to).length === 0)
            errors.push(`通知节点 ${node.id} 至少需要一个收件人。`);
          if (!notify.from && !notify.smtp_user)
            errors.push(`通知节点 ${node.id} 请填写发件人或 SMTP 用户名。`);
        } else {
          const target = notify.webhook || notify.url;
          if (!target) errors.push(`通知节点 ${node.id} 请填写 Webhook 地址。`);
          else if (!target.includes("{{") && !/^https?:\/\//i.test(target))
            errors.push(`通知节点 ${node.id} 的 Webhook 地址只支持 http:// 或 https://。`);
        }
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
      if (node.data.controlType === "http_request") {
        const http = normalizeHttpRequestConfig(node.data.http || {});
        const urlError = httpUrlError(http.url);
        if (urlError) errors.push(`HTTP 节点 ${node.id}：${urlError}`);
        if (
          http.body_type !== "none" &&
          !String(http.body || "").trim()
        )
          errors.push(
            `HTTP 节点 ${node.id} 选择了 ${http.body_type} 请求体，但内容为空。`,
          );
        if (http.body_type !== "none" && http.method === "GET")
          errors.push(
            `HTTP 节点 ${node.id} 的 GET 请求不能配置请求体，请改用 POST/PUT/PATCH。`,
          );
        (http.headers || []).forEach((item, index) => {
          if (!item.name)
            errors.push(
              `HTTP 节点 ${node.id} 的第 ${index + 1} 个 Header 缺少名称。`,
            );
          if (/[\r\n]/.test(item.name) || /[\r\n]/.test(item.value))
            errors.push(
              `HTTP 节点 ${node.id} 的第 ${index + 1} 个 Header 不能包含换行。`,
            );
        });
        if (http.save_response) {
          const saved = normalizeRelativeConfigPath(
            http.save_response,
            "响应保存路径",
          );
          if (saved.error)
            errors.push(
              `HTTP 节点 ${node.id} 的响应保存路径无效：${saved.error}`,
            );
        }
        if (http.timeout && !/^\d+(\.\d+)?(ms|s|m|h)$/.test(http.timeout))
          errors.push(
            `HTTP 节点 ${node.id} 的超时时间格式无效，例如 30s、2m。`,
          );
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
  stages = [],
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
  upstreamNodeIDs(selectedNodeID, edges, nodes, stages).forEach((nodeID) => {
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
    if (node.type === "controlNode" && node.data?.controlType === "http_request") {
      sources.push({
        label: `${nodeID} / HTTP 状态码`,
        value: `{{ .steps.${nodeID}.http.status }}`,
      });
      sources.push({
        label: `${nodeID} / HTTP 耗时（毫秒）`,
        value: `{{ .steps.${nodeID}.http.duration_ms }}`,
      });
      sources.push({
        label: `${nodeID} / 响应保存路径`,
        value: `{{ .steps.${nodeID}.http.saved_path }}`,
      });
    }
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
  stages = [],
) {
  const sources = [];
  const toolMap = new Map((tools || []).map((tool) => [tool.id, tool]));
  upstreamNodeIDs(selectedNodeID, edges, nodes, stages).forEach((nodeID) => {
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

function upstreamNodeIDs(selectedNodeID, edges, nodes = [], stages = []) {
  if (!selectedNodeID) return [];
  if (stages.length > 0) {
    const selected = nodes.find((node) => node.id === selectedNodeID);
    const selectedStage = stages.findIndex((stage) => stage.id === nodeStageID(selected));
    // 串行阶段按 order 形成隐式链；并行阶段没有隐式链，上游只能来自显式依赖和更早的阶段。
    const upstream = new Set();
    const pending = [selectedNodeID];
    const visited = new Set(pending);
    while (pending.length > 0) {
      const target = pending.pop();
      edges.filter(edge => edge.target === target).forEach(edge => {
        upstream.add(edge.source);
        if (!visited.has(edge.source)) {
          visited.add(edge.source);
          pending.push(edge.source);
        }
      });
    }
    if (selectedStage >= 0 && stages[selectedStage].execution_mode !== "parallel") {
      const stageNodes = orderedStageNodes(nodes, stages[selectedStage].id);
      const index = stageNodes.findIndex((node) => node.id === selectedNodeID);
      stageNodes.slice(0, Math.max(index, 0)).forEach((node) => upstream.add(node.id));
    }
    stages.slice(0, Math.max(selectedStage, 0)).forEach((stage) => {
      orderedStageNodes(nodes, stage.id).forEach((node) => upstream.add(node.id));
    });
    upstream.delete(selectedNodeID);
    return Array.from(upstream);
  }
  const direct = edges
    .filter((edge) => edge.target === selectedNodeID)
    .map((edge) => edge.source);
  return Array.from(new Set(direct)).sort((a, b) =>
    a.localeCompare(b, "zh-CN"),
  );
}
