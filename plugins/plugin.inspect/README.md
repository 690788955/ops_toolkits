# 巡检插件演示

这个目录演示一个独立的巡检 demo 插件，用于验证多插件、多分类、多标签，以及工作流编辑器中“全局/分类工具范围”的交互。

## 包结构

```text
plugins/plugin.inspect/
  plugin.yaml
  scripts/
    check.sh
```

## 工具

### plugin.inspect.check

普通插件工具，模拟输出目标、CPU、磁盘、服务和状态等巡检结果。该工具只输出日志，不连接真实外部系统，也不修改任何系统状态。

参数：

- `target`：巡检目标，必填，默认 `demo`
- `service`：模拟巡检服务名称，默认 `nginx`
- `status`：模拟服务状态，默认 `OK`

```bash
./bin/opsctl.exe run tool plugin.inspect.check --set target=demo --set service=nginx --set status=OK --no-prompt
```

## 校验

```bash
./bin/opsctl.exe validate
./bin/opsctl.exe list
```

菜单中会出现 `巡检演示` 分类和 `plugin.inspect.check` 工具。
