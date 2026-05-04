# 规范插件模板

## 功能

这个插件提供普通工具 plugin.template.inspect、高风险工具 plugin.template.apply 和 workflow plugin.template.maintenance-flow。它是可复制的规范模板，用来展示插件目录、manifest 字段、脚本参数解析、confirm 配置、depends_on 依赖和交付说明；它不是业务逻辑。

复制模板后，请把插件 ID、分类、工具 ID、脚本逻辑、风险说明、回滚说明和联系人改成你的真实插件含义。

## 目录说明

- plugin.yaml：声明插件元数据、分类、工具、workflow、参数和 confirm。
- scripts/run.sh：工具脚本，演示 usage、参数解析、未知参数拒绝、必填校验、dry-run、错误返回和安全输出。
- workflows/maintenance-flow.yaml：插件内 workflow 示例，引用本插件工具并使用 depends_on 表达依赖。
- examples/params.yaml：本地运行参数示例。

## 输入

- target：目标标识，必填。
- action：执行动作，示例支持 inspect 或 apply。
- dry_run：是否仅预览动作，默认 true。

## 输出

stdout 会输出执行进度和结果；stderr 只输出参数错误或执行错误。不要输出密码、令牌、密钥、完整连接串等敏感信息。

## 风险与确认

plugin.template.inspect 是普通只读示例，confirm.required 为 false。

plugin.template.apply 是高风险示例，confirm.required 为 true。真实插件如果会删除、覆盖、重启、变更生产配置或影响业务，请保留确认策略，并把 confirm.message 写清目标、动作、影响范围和回滚要求。

workflow plugin.template.maintenance-flow 包含高风险节点，因此 workflow 自身也配置 confirm.required: true。

## 本地验证

将插件目录安装到本地验证环境的 plugins/plugin.template 后运行：

```bash
./bin/opsctl.exe validate
./bin/opsctl.exe list
./bin/opsctl.exe run tool plugin.template.inspect --set target=demo --set action=inspect --set dry_run=true --no-prompt
printf '确认\n' | ./bin/opsctl.exe run tool plugin.template.apply --set target=demo --set action=apply --set dry_run=true --no-prompt
printf '确认\n确认\n' | ./bin/opsctl.exe run workflow plugin.template.maintenance-flow --set target=demo --set action=apply --set dry_run=true --no-prompt
```

## 打包交付

只压缩这个插件目录。ZIP 根目录可以直接是 plugin.yaml，也可以是 plugin.template/plugin.yaml。不要把上层开发包目录或无关文件一起交付。

如果交付的是已存在插件的新版本，请先提升 plugin.yaml 的 version；只有版本高于已安装版本时才应替换。

## 安全与运维

- 不要把密码、令牌、密钥或生产连接串打进 ZIP。
- 不要依赖插件目录外的相对路径；command、workdir、workflow path 都应留在插件目录内部。
- 根据实际耗时设置 timeout，避免长时间占用运行队列。
- 不要假设交付或接入时会自动执行工具；上线前仍需手动 run tool / run workflow 验证。
- 高风险动作必须先 dry-run，再按变更窗口和回滚方案执行。

## 回滚

如果插件工具会修改系统状态，请在这里写清楚回滚步骤、影响范围和联系人。
