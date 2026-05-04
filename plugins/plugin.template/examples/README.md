# 示例

## 参数文件

params.yaml 是模板工具参数示例，可用于参数输入或命令行 --set 对照。

## 本地命令

```bash
./bin/opsctl.exe validate
./bin/opsctl.exe run tool plugin.template.inspect --set target=demo --set action=inspect --set dry_run=true --no-prompt
printf '确认\n' | ./bin/opsctl.exe run tool plugin.template.apply --set target=demo --set action=apply --set dry_run=true --no-prompt
printf '确认\n确认\n' | ./bin/opsctl.exe run workflow plugin.template.maintenance-flow --set target=demo --set action=apply --set dry_run=true --no-prompt
```

## 交付前检查

交付 ZIP 前，请核对：

- 分类：插件模板
- 普通工具：plugin.template.inspect
- 高风险工具：plugin.template.apply
- 工作流：plugin.template.maintenance-flow
- confirm.required 示例已按真实风险调整
