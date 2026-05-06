# 重新设计配置管理：直接编辑配置文件

## Goal

重新设计配置管理机制，让用户直接编辑工具实际使用的配置文件内容（INI、ENV、YAML 等格式），而不是编辑 YAML 业务配置。框架根据工具声明的配置文件，自动映射和传递配置文件给工具。

## What I already know

* **当前配置管理机制**：
  - 用户编辑 `configs/plugins/{plugin-id}.yaml`（YAML 格式的业务配置）
  - 框架使用 Go template 渲染配置模板
  - 渲染后的配置文件传递给工具
  - 配置值来源：参数默认值 → 全局配置 → 插件配置 → 工具配置 → 宿主配置 → 运行时参数

* **当前问题**：
  - 用户看到的是 YAML 业务配置，不是工具实际使用的配置文件
  - 用户需要理解 YAML 结构和 Go template 语法
  - 配置文件格式（INI、ENV 等）对用户不可见
  - 用户无法直观地看到工具会收到什么配置

* **用户期望**：
  - 直接编辑工具使用的配置文件内容（INI、ENV、YAML 等）
  - 看到的就是工具会收到的配置
  - 不需要理解 YAML 结构和模板语法
  - 框架自动处理配置文件的映射和传递

## Requirements

### 1. 配置文件声明

工具在 `plugin.yaml` 中声明需要的配置文件：

```yaml
tools:
  - id: plugin.backup.full
    name: 全量备份
    config_files:
      - name: backup.conf
        format: ini  # ini/env/yaml/json/toml/text
        description: 备份配置文件
        required: true
        pass_via: arg  # arg/env/copy
        arg: --config  # 当 pass_via=arg 时使用
        # env: BACKUP_CONFIG_FILE  # 当 pass_via=env 时使用
        default_content: |
          [backup]
          target = /data
          retention = 7
```

**传递方式**：
- `arg`：通过命令行参数传递路径（`--config /path/to/file`）
- `env`：通过环境变量传递路径（`BACKUP_CONFIG_FILE=/path/to/file`）
- `copy`：复制到工具工作目录，工具直接读取（`./backup.conf`）

**格式类型**：
- `ini`：INI 格式
- `env`：ENV 格式（`KEY=value`）
- `yaml`：YAML 格式
- `json`：JSON 格式
- `toml`：TOML 格式
- `text`：纯文本

### 2. 配置文件存储

配置文件内容存储在：
- **位置**：`configs/plugins/{plugin-id}/files/{file-name}`
- **格式**：工具声明的格式（INI、ENV、YAML 等）
- **默认值**：如果文件不存在，使用 `default_content`

### 3. 前端编辑

用户在工具配置页面：
- 看到工具声明的配置文件列表
- 点击配置文件打开编辑器
- 根据格式类型提供语法高亮
- 直接编辑配置文件内容
- 保存后立即生效

### 4. 配置文件传递

工具执行时：
- 框架读取配置文件内容（从存储位置或使用默认内容）
- 根据 `pass_via` 方式传递：
  - `arg`：添加到命令行参数
  - `env`：设置环境变量
  - `copy`：复制到工作目录

### 5. 废弃旧机制

完全废弃现有的配置模板机制：
- 移除 `config_templates` 支持
- 移除 Go template 渲染逻辑
- 移除 YAML 业务配置编辑
- 更新插件开发文档

## Acceptance Criteria

* [ ] 工具可以在 `plugin.yaml` 中声明 `config_files`
* [ ] 配置文件内容存储在 `configs/plugins/{plugin-id}/files/` 目录
* [ ] 后端 API 支持读取/保存配置文件内容
* [ ] 前端显示工具的配置文件列表
* [ ] 可以编辑配置文件内容（根据格式提供语法高亮）
* [ ] 工具执行时自动传递配置文件（支持 arg/env/copy 三种方式）
* [ ] 不存在的配置文件使用默认内容
* [ ] 移除 `config_templates` 相关代码
* [ ] 更新插件开发模板和文档
* [ ] 前端构建通过
* [ ] 后端测试通过

## Definition of Done

* `npm run build --prefix web` 通过
* `GOTOOLCHAIN=local go test ./...` 通过
* `GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl` 通过
* 示例工具可以正常使用配置文件
* 插件开发文档已更新

## Out of Scope

* 配置文件的语法验证（由工具自己验证）
* 配置文件的版本管理
* 配置文件的权限控制
* 配置文件的加密存储
* 向后兼容旧的 `config_templates` 机制（完全废弃）
* 提供迁移工具（手动迁移）

## Technical Approach

### 配置文件声明结构

```go
type ConfigFile struct {
    Name           string `yaml:"name"`
    Format         string `yaml:"format"`  // ini/env/yaml/json/toml/text
    Description    string `yaml:"description"`
    Required       bool   `yaml:"required"`
    PassVia        string `yaml:"pass_via"`  // arg/env/copy
    Arg            string `yaml:"arg,omitempty"`
    Env            string `yaml:"env,omitempty"`
    DefaultContent string `yaml:"default_content,omitempty"`
}
```

### API 端点

- `GET /api/plugins/{plugin-id}/files` - 列出插件的配置文件
- `GET /api/plugins/{plugin-id}/files/{file-name}` - 读取配置文件内容
- `PUT /api/plugins/{plugin-id}/files/{file-name}` - 保存配置文件内容
- `DELETE /api/plugins/{plugin-id}/files/{file-name}` - 删除配置文件

### 运行时传递逻辑

```go
func (r *Runner) passConfigFiles(tool *registry.Tool, runDir string) ([]string, []string, error) {
    args := []string{}
    envs := []string{}
    
    for _, cf := range tool.Config.ConfigFiles {
        content := loadConfigFileContent(tool.Config.PluginConfig.ID, cf.Name, cf.DefaultContent)
        
        switch cf.PassVia {
        case "arg":
            path := writeConfigFile(runDir, cf.Name, content)
            args = append(args, cf.Arg, path)
        case "env":
            path := writeConfigFile(runDir, cf.Name, content)
            envs = append(envs, cf.Env+"="+path)
        case "copy":
            copyToWorkdir(tool.Dir, cf.Name, content)
        }
    }
    
    return args, envs, nil
}
```

## Technical Notes

* 主要文件：
  - 后端：`internal/config/types.go`（新增 `ConfigFile` 结构）
  - 后端：`internal/server/server.go`（配置文件 API）
  - 后端：`internal/runner/runner.go`（配置文件传递逻辑）
  - 前端：`web/src/main.jsx`（配置文件编辑器）
* 需要移除的代码：
  - `internal/runner/runner.go` 中的 `renderConfigTemplates()` 函数
  - `internal/config/types.go` 中的 `ConfigTemplate` 结构
  - 前端的配置模板相关组件
* 这是一个破坏性变更，现有使用 `config_templates` 的插件需要手动迁移

