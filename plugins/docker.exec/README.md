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

### docker.exec.host_command — 宿主机执行命令

在宿主机上通过 `bash -lc` 执行一条命令，支持管道、重定向和 `cd && ...` 等复合命令。

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| command | string | 是 | 在宿主机执行的命令 |

**示例：**
```bash
opsctl run tool docker.exec.host_command \
  --set command="docker ps"
```

### docker.exec.verify_md5_merge — 分片包 MD5 校验并合并

读取上传目录中的 `md5.txt`，先执行 `md5sum -c md5.txt` 校验全部分片，校验通过后按清单顺序合并为完整包。

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 上传文件节点传入的文件路径或目录路径；传入文件时自动使用其所在目录 |
| output | string | 否 | 合并后的输出文件名；留空时自动去掉首个分片名末尾的 `.00/.01` 等数字后缀 |

**示例：**
```bash
opsctl run tool docker.exec.verify_md5_merge \
  --set path="/data/uploads/release/md5.txt" \
  --set output=""
```

工作流中可将上传节点路径传给该工具，例如 `{{ .steps.upload.file.path }}`。如果上传的是多文件目录，也可传 `{{ .steps.upload.file.dir }}`。

## 前置依赖

- 目标服务器需安装 Docker
- 运行校验合并的宿主机需安装 `md5sum`（coreutils）
- 目标容器需处于运行状态

## 风险提示

- 执行命令前会弹出确认提示，请确认容器和命令无误后再执行
- 脚本内部会对容器运行状态进行预检查，避免对未运行容器执行操作
- 宿主机命令会直接在当前服务所在宿主机执行，请确认命令内容和影响范围
- 分片合并会在上传目录内写入输出文件；如果同名文件已存在，会在校验通过后覆盖
