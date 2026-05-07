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
├── config/                # 配置文件目录（可选）
│   └── example.conf       # 示例配置文件
└── examples/              # 示例文件目录（可选）
    ├── params.yaml        # 参数示例
    └── host-absolute.mapping.yaml  # 宿主绝对路径映射示例（仅管理员参考）
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

**1. 在插件目录内放置配置文件**

例如：

```text
plugins/plugin.mycompany.mytool/
├── plugin.yaml
├── config/
│   └── example.conf
└── scripts/
    └── run.sh
```

**2. 在 plugin.yaml 中声明插件内配置基准目录和相对项**

推荐新版写法：用 `config_dir` 指定插件内基准目录，`config_files` 只写相对文件或相对目录项。

```yaml
tools:
  - id: plugin.mycompany.mytool.action
    name: 执行操作
    command: scripts/run.sh
    args:
      - --config
      - config/example.conf
    config_dir: config
    config_files:
      - example.conf
```

兼容旧写法：旧插件继续可以写完整插件内相对路径。

```yaml
config_files:
  - config/example.conf
```

配置文件声明规则：

- 插件 manifest 里的 `config_files` 仍只表示插件包内配置，不授予任意宿主文件访问能力。
- 未声明 `config_dir` 时，新短文件名默认以插件 `config/` 目录作为基准；旧 `config/...` 路径保持兼容。
- 声明了 `config_dir` 后，`config_files` / `path` 只写相对文件或相对目录项，例如 `example.conf`、`profiles/dev.env`、`profiles`。
- `config_files` 可以声明目录项；Web/API 列表会展开该目录下一层普通文件，不递归扫描子目录。
- `config_dir` 和 `config_files` 都不能使用 `..` 逃逸；`config_files` 不能写绝对路径。
- 插件包本身不能直接声明 `/etc/...`、`C:\\...` 等任意宿主绝对路径。
- `config_files` 只声明页面可以编辑哪些配置文件；框架不会自动传参、复制或生成配置文件，工具脚本应按自己的逻辑读取这些文件。

**3. 如需编辑宿主机器绝对路径配置，由管理员在宿主侧显式映射**

宿主绝对路径属于更高风险能力，不应写进插件包默认 `plugin.yaml`。插件作者可以在 README 中说明建议映射方式，实际启用必须由管理员在框架配置和 host-side mapping 中声明。

框架全局配置先声明允许访问的宿主目录白名单，只支持目录，不支持单文件白名单：

```yaml
# configs/ops.yaml
host_config_files:
  allowed_dirs:
    - /etc/myapp
    - C:\\ProgramData\\Vendor\\MyApp
```

然后管理员在宿主侧 mapping 中按工具声明宿主配置文件：

```yaml
# configs/plugins/plugin.mycompany.mytool.mapping.yaml
tools:
  plugin.mycompany.mytool.action:
    config_files:
      - id: app-main
        label: 应用主配置
        scope: host_absolute
        config_dir: /etc/myapp
        path: app.conf
        access: read_write
        create: false
      - id: app-extra
        label: 可选扩展配置
        scope: host_absolute
        config_dir: /etc/myapp/conf.d
        path: extra.conf
        access: read
        create: false
```

宿主映射规则：

- `scope: host_absolute` 表示这是宿主机器绝对路径映射。
- `config_dir` 必须是当前平台可识别的宿主绝对目录，并落在 `host_config_files.allowed_dirs` 白名单目录内。
- `path` / `config_files` 条目只写相对文件或相对目录项，最终路径只能由 `config_dir + path` 得出。
- 白名单只支持目录前缀；清理后的最终路径和符号链接解析后的真实路径也必须仍在白名单内，避免软链逃逸授权目录。
- `access: read` 只允许查看，拒绝保存；`access: read_write` 在 OS 权限允许时才允许保存。
- `create: false` 表示文件不存在时不自动创建；`create: true` 表示文件不存在且父目录可写时允许创建。
- 默认不删除宿主绝对路径配置文件，避免误删机器文件。

**4. 在脚本中使用配置文件**

```bash
#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="config/example.conf"

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

if [[ -f "$CONFIG_FILE" ]]; then
  echo "使用配置文件: $CONFIG_FILE"
  # 读取配置...
fi
```

**5. 用户如何编辑配置文件**

用户通过 Web UI 或 API 直接编辑已声明的配置文件：
- 新写法声明 `config_dir: config` + `example.conf` 时，页面编辑 `plugins/{plugin-id}/config/example.conf`
- 旧写法声明 `config/example.conf` 时，页面同样编辑 `plugins/{plugin-id}/config/example.conf`
- 声明目录项时，页面会展开该目录下一层普通文件，不递归扫描子目录
- 插件内路径必须留在插件目录内，不能使用绝对路径或 `..` 逃逸

同一插件的多个工具可以共享同一个配置文件声明。

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
- [ ] 插件内配置文件路径已在 `config_dir` / `config_files` 中声明，且只使用插件内相对项
- [ ] 如需宿主绝对路径配置文件，已在 README 或 `examples/host-absolute.mapping.yaml` 给出管理员 mapping 示例，且未在插件默认清单中直接启用宿主路径
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

A: 将共享配置文件放在插件目录内，例如 `config/common.yaml`，多个工具可以声明相同的 `config_dir: config` 与 `config_files: [common.yaml]`，并在脚本参数或脚本默认值中读取同一个文件。旧写法 `config_files: [config/common.yaml]` 仍兼容。

### Q: `config_files` 会不会自动传给工具？

A: 不会。`config_files` 只用于 Web 配置页面展示和编辑声明文件。工具是否通过 `--config config/example.conf`、固定路径、环境变量或其他方式读取配置，由插件脚本自己决定。

### Q: 插件能否直接声明宿主绝对路径配置文件？

A: 不能。插件包默认清单只能声明插件目录内配置文件。宿主绝对路径必须由管理员通过 `configs/ops.yaml` 的 `host_config_files.allowed_dirs` 白名单和 `configs/plugins/<plugin-id>.mapping.yaml` 中的 `scope: host_absolute` 显式启用，并受 `access`、`create` 和 OS 权限共同限制。

### Q: 如何调试配置文件读取？

A: 直接检查插件目录内声明的文件内容，例如 `plugins/{plugin-id}/config/example.conf`；工具运行日志仍在 `runs/logs/{run_id}/`。

## 参考资源

- [系统资源检查插件](../plugin.system-check/) - 完整的插件示例
- [框架文档](../../README.md) - 框架使用说明
