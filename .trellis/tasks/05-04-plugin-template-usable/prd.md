# brainstorm: 插件模板可直接试用

## Goal

确认并定义“下载的插件模板应可直接试用”的产品要求，确保开发者拿到模板后能最短路径完成本地验证与二次开发，而不是只能当文档参考。

## What I already know

* 你提出的核心诉求是：插件模板应该“正常能试用”，从而降低他人开发插件的门槛。
* 当前前端已提供插件管理弹窗和“下载插件模板”入口：`web/src/main.jsx:382` 指向 `/api/dev/toolkit.zip`。
* 后端模板下载接口存在：`internal/server/server.go:879` 的 `toolDevKitHandler()` 调用 `buildToolDevKitZip()`。
* 模板 ZIP 当前包含完整示例文件：`README.md`、`SPEC.md`、`plugins/plugin.template/plugin.yaml`、`scripts/run.sh`、`workflows/maintenance-flow.yaml`、`examples/params.yaml` 等（`internal/server/server.go:986`）。
* 现有测试已强校验模板内容与“非 legacy 路径”：`internal/server/server_test.go:280`（含关键文案、命令、旧路径禁用项检查）。
* 现有模板文案中已包含可执行命令（例如 `./bin/opsctl.exe run tool plugin.template.inspect`），目标上是“可复制的规范模板”。

## Assumptions (temporary)

* 你说“不能直接用”很可能指“下载后不能零修改跑通”或“跑通过程不够直观/稳定”，而不是“完全缺少模板内容”。
* 当前问题更可能是“可试用标准不清晰”或“模板到试跑链路有断点（环境/命令/路径/脚本权限/上传后验证）”。

## Open Questions

* 暂无（已确定采用 A：零修改可试跑标准）。

## Requirements (evolving)

* 插件模板必须满足“零修改可试跑”：下载后的模板在不修改任何文件的前提下，可直接完成一次 tool 与 workflow 试跑。
* 模板 README 必须给出从下载到验证再到运行的最短步骤，并可按步骤成功。
* 模板脚本、manifest、workflow、示例参数必须互相一致，避免文档与文件不对齐。
* 模板测试需要覆盖“可试用”验收，而不只是文件存在检查。
* 下载包内必须包含可直接安装的标准插件目录结构，且命令全部基于 plugin-first 结构。
* 必须新增自动化回归测试，防止后续模板改动破坏“零修改可试跑”链路。

## Acceptance Criteria (evolving)

* [ ] 已文档化并落地“零修改可试跑”标准（无需改模板任意文件）。
* [ ] 按标准完成端到端路径成功：下载模板 ZIP → 安装/上传 → `./bin/opsctl.exe validate` → `run tool plugin.template.inspect` 成功 → `run workflow plugin.template.maintenance-flow` 成功。
* [ ] 自动化测试覆盖“模板可运行”断言（至少验证模板示例工具和工作流可被注册并执行到成功路径），不止检查 zip 文件列表。
* [ ] 自动化测试作为回归门禁，模板内容变更时若破坏可试跑链路会直接失败。
* [ ] 模板说明中的命令在当前 plugin-first 结构下可执行，不包含 legacy 路径。
* [ ] 对外开发者按 README 步骤操作时，不需要补充隐含前置知识（如额外路径修正、命令替换）。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Out of Scope (explicit)

* 远程插件市场能力。
* 插件签名体系。
* 与模板可试用无关的 UI 大改。

## Technical Notes

* 模板下载后端：`internal/server/server.go` (`toolDevKitHandler`, `buildToolDevKitZip`)。
* 模板下载前端入口：`web/src/main.jsx` (`/api/dev/toolkit.zip`)。
* 模板质量测试：`internal/server/server_test.go:280`。
* 相关历史任务：`.trellis/tasks/04-27-04-27-plugin-template-download-upload/prd.md`，该任务已定义“产品级模板+上传”范围。
