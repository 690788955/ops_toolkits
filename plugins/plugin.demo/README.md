# 插件接入演示

这个目录演示外部用户如何通过 `plugin.yaml` 把自己的工具接入 opsctl 菜单、Web UI 和工作流。

## 包结构

```text
plugins/plugin.demo/
  plugin.yaml
  scripts/
    greet.sh
    confirmed.sh
```

## 工具

### plugin.demo.greet

普通插件工具，演示参数、命令参数和环境变量传递。

```bash
./opsctl run tool plugin.demo.greet --set name=Tester --set message=Hello --no-prompt
```

### plugin.demo.confirmed

确认流程演示工具，`plugin.yaml` 中配置了 `confirm.required: true`。

```bash
./opsctl run tool plugin.demo.confirmed --set target=demo --no-prompt
```

执行时需要输入 `yes`、`确认`、`是` 或 `继续`。

## 校验

```bash
./opsctl validate
./opsctl list
```

菜单中会出现 `插件演示` 分类。
