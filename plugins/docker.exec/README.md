# Docker 容器执行插件

在 Docker 容器中执行命令，支持指定容器名称和待执行命令。

## 工具列表

### docker.exec.run — 容器内执行命令

在指定的 Docker 容器中执行命令（非交互式）。

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| container | string | 是 | Docker 容器名称或 ID |
| command | string | 是 | 在容器内执行的命令 |

**示例：**
```bash
opsctl run tool docker.exec.run \
  --set container=ansible \
  --set command="./ops/everisk/install_all.sh"
```

### docker.exec.bash — 容器内执行 Bash 脚本

在指定的 Docker 容器中通过 `bash -c` 执行复合脚本命令。

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| container | string | 是 | Docker 容器名称或 ID |
| script | string | 是 | 通过 bash -c 执行的脚本内容 |

**示例：**
```bash
opsctl run tool docker.exec.bash \
  --set container=ansible \
  --set script="cd /app && ./deploy.sh"
```

## 前置依赖

- 目标服务器需安装 Docker
- 目标容器需处于运行状态

## 风险提示

- 执行命令前会弹出确认提示，请确认容器和命令无误后再执行
- 脚本内部会对容器运行状态进行预检查，避免对未运行容器执行操作
