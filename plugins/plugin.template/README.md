# 插件开发模板

这是一个插件开发模板，帮助你快速创建符合框架规范的工具插件。

## 快速开始

1. 复制此目录并重命名为你的插件 ID（例如：`plugin.mycompany.backup`）
2. 修改 `plugin.yaml` 中的插件信息
3. 编写工具脚本（`scripts/` 目录）
4. 测试插件功能
5. 打包交付

## 目录结构

```
plugin.template/
├── plugin.yaml              # 插件清单（必需）
├── README.md               # 插件说明文档
├── scripts/                # 工具脚本目录
│   └── example.sh         # 示例工具脚本
├── configs/               # 配置文件目录（可选）
│   ├── default.yaml       # 插件默认配置
│   └── templates/         # 配置模板目录
│       └── example.conf.tmpl  # 示例配置模板
└── examples/              # 示例文件目录（可选）
    └── params.yaml        # 参数示例
```

## 插件清单（plugin.yaml）

### 基本信息

```yaml
id: plugin.mycompany.mytool
name: 我的工具集
version: 1.0.0
description: 工具集描述
author: your-name
```

### 贡献分类

```yaml
contributes:
  categories:
    - id: mycat
      name: 我的分类
      description: 分类描述
```

### 定义工具

```yaml
  tools:
    - id: plugin.mycompany.mytool.action
      name: 执行操作
      category: mycat
      description: 工具描述
      tags: [tag1, tag2]
      command: scripts/example.sh
      args:
        - --param
        - "{{ .param }}"
      workdir: .
      timeout: 30s
      parameters:
        - name: param
          type: string
          description: 参数描述
          required: true
      confirm:
        required: false
```

## 配置文件功能

### 什么时候使用配置文件？

当你的工具需要：
- 复杂的配置结构（多个配置项、嵌套结构）
- 特定的配置文件格式（INI、ENV、JSON 等）
- 多个工具共享同一套配置
- 用户可以直接编辑配置文件内容

### 如何使用配置文件？

**1. 在 plugin.yaml 中声明配置文件**

```yaml
tools:
  - id: plugin.mycompany.mytool.action
    name: 执行操作
    config_files:
      - name: example.conf
        format: ini  # ini/env/yaml/json/toml/text
        description: 示例配置文件
        required: false
        pass_via: arg  # arg/env/copy
        arg: --config
        default_content: |
          [service]
          name = MyService
          endpoint = https://api.example.com
```

**配置文件字段说明**：
- `name`：配置文件名称（必填）
- `format`：文件格式（必填）- `ini`/`env`/`yaml`/`json`/`toml`/`text`
- `description`：配置文件描述
- `required`：是否必填
- `pass_via`：传递方式（必填）- `arg`/`env`/`copy`
- `arg`：命令行参数名（当 `pass_via=arg` 时必填）
- `env`：环境变量名（当 `pass_via=env` 时必填）
- `default_content`：默认内容（用户未创建配置文件时使用）

**2. 配置文件传递方式**

**方式 1：通过命令行参数传递路径**
```yaml
config_files:
  - name: backup.conf
    format: ini
    pass_via: arg
    arg: --config  # 生成: --config /path/to/backup.conf
```

**方式 2：通过环境变量传递路径**
```yaml
config_files:
  - name: backup.conf
    format: ini
    pass_via: env
    env: BACKUP_CONFIG_FILE  # 设置: BACKUP_CONFIG_FILE=/path/to/backup.conf
```

**方式 3：复制到工具工作目录**
```yaml
config_files:
  - name: backup.conf
    format: ini
    pass_via: copy  # 框架复制到工作目录，工具直接读取 ./backup.conf
```

**3. 在脚本中使用配置文件**

```bash
#!/usr/bin/env bash
set -euo pipefail

# 通过参数接收配置文件路径
CONFIG_FILE=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --config)
      CONFIG_FILE="$2"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
done

# 使用配置文件
if [[ -f "$CONFIG_FILE" ]]; then
  echo "使用配置文件: $CONFIG_FILE"
  # 读取配置...
fi
```

**4. 用户如何编辑配置文件**

用户通过 Web UI 或 API 直接编辑配置文件内容：
- 配置文件存储在：`configs/plugins/{plugin-id}/files/{file-name}`
- 用户看到的就是工具实际使用的配置文件内容
- 支持语法高亮（根据 `format` 字段）

### 配置文件存储位置

配置文件按插件存储：
```
configs/
└── plugins/
    └── {plugin-id}/
        └── files/
            ├── example.conf
            └── database.conf
```

同一插件的多个工具可以共享配置文件。

## 参数定义

### 参数类型

- `string`：字符串
- `number`：数字
- `boolean`：布尔值

### 参数属性

```yaml
parameters:
  - name: param_name
    type: string
    description: 参数描述
    required: true
    default: default_value
```

### 参数传递方式

**1. 环境变量（默认）**
```yaml
# 参数自动转为环境变量传递给脚本
# 例如：param_name -> PARAM_NAME
```

**2. 命令行参数**
```yaml
args:
  - --param
  - "{{ .param }}"
```

**3. 参数文件**
```yaml
# 框架自动生成 params.yaml 并设置 OPS_PARAM_FILE 环境变量
```

## 确认提示

对于高风险操作，可以要求用户确认：

```yaml
confirm:
  required: true
  message: 确认执行此操作？此操作不可撤销。
```

## 脚本编写规范

### 基本结构

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. 参数解析
PARAM=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --param)
      PARAM="$2"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
done

# 2. 参数验证
if [[ -z "$PARAM" ]]; then
  echo "错误: --param 参数必填" >&2
  exit 1
fi

# 3. 执行逻辑
echo "执行操作: $PARAM"

# 4. 错误处理
if ! some_command; then
  echo "错误: 操作失败" >&2
  exit 1
fi

# 5. 成功退出
echo "操作完成"
exit 0
```

### 错误处理

- 使用 `set -euo pipefail` 确保错误时脚本退出
- 错误信息输出到 stderr（`>&2`）
- 失败时返回非 0 退出码

### 输出规范

- 正常信息输出到 stdout
- 错误信息输出到 stderr
- 支持 JSON 格式输出（便于程序处理）

## 测试插件

### 1. 验证配置

```bash
./bin/opsctl.exe validate
```

### 2. 执行工具

```bash
# 交互式执行
./bin/opsctl.exe run tool plugin.mycompany.mytool.action

# 非交互式执行
./bin/opsctl.exe run tool plugin.mycompany.mytool.action --set param=value --no-prompt
```

### 3. Web UI 测试

```bash
./bin/opsctl.exe serve
# 访问 http://127.0.0.1:8080
```

## 打包交付

### 1. 检查清单

- [ ] `plugin.yaml` 信息完整
- [ ] 工具脚本可执行
- [ ] 参数定义正确
- [ ] 配置模板语法正确
- [ ] README.md 文档完善
- [ ] 示例文件齐全

### 2. 打包插件

只压缩插件目录本身：

```bash
cd plugins
zip -r plugin.mycompany.mytool.zip plugin.mycompany.mytool/
```

### 3. 安装插件

用户通过 Web UI 或 CLI 上传插件 ZIP 包即可安装。

## 最佳实践

1. **插件 ID 命名**：使用反向域名格式（`plugin.company.product`）
2. **工具 ID 命名**：插件 ID + 工具名（`plugin.company.product.action`）
3. **脚本可移植**：避免依赖特定环境，使用标准命令
4. **错误友好**：提供清晰的错误信息和解决建议
5. **文档完善**：README.md 包含使用说明、参数说明、示例
6. **配置分离**：敏感信息通过配置文件传递，不硬编码
7. **幂等性**：工具多次执行结果一致，避免副作用

## 常见问题

### Q: 如何访问全局环境配置？

A: 框架会设置 `OPS_GLOBAL_ENV_FILE` 环境变量，指向 `configs/global-env.conf`：

```bash
# Shell 脚本可以直接 source
source "$OPS_GLOBAL_ENV_FILE"

# 或手动解析
while IFS='=' read -r key value; do
  export "$key=$value"
done < "$OPS_GLOBAL_ENV_FILE"
```

### Q: 如何在工具间共享配置？

A: 使用全局配置文件库（`configs/templates/`），多个工具可以绑定同一个配置文件。

### Q: 配置模板支持哪些语法？

A: 使用 Go template 语法：
- 变量：`{{ .var }}`
- 条件：`{{ if .condition }}...{{ end }}`
- 循环：`{{ range .items }}...{{ end }}`
- 函数：`{{ .var | default "value" }}`

### Q: 如何调试配置模板？

A: 运行工具后，生成的配置文件在 `runs/logs/{run_id}/generated/` 目录。

## 参考资源

- [系统资源检查插件](../plugin.system-check/) - 完整的插件示例
- [框架文档](../../README.md) - 框架使用说明
- [Go Template 语法](https://pkg.go.dev/text/template) - 配置模板语法参考
