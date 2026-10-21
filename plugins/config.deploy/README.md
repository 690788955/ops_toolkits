# 配置文件部署（config.deploy）

把插件内受管配置文件的内容写入目标路径。配置文件在插件目录内维护（可在 Web 控制台直接编辑），工具入参为目标文件路径；执行时按目标路径写入，并完成备份、权限保留与写入后校验。

## 目录结构

```text
plugins/config.deploy/
├── plugin.yaml              # 插件清单
├── README.md                # 本文件
├── config/
│   └── app.conf             # 受管配置文件（部署内容来源，可在 Web 端编辑）
├── scripts/
│   └── push-config.sh       # 部署脚本（push / check 复用）
└── examples/
    └── params.yaml          # 参数示例
```

## 工具

### 1. `config.deploy.push` 下发配置文件

| 参数 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `target_path` | string | 是 | - | 目标文件绝对路径；传入已存在目录或以 `/` 结尾时，写入该目录下的同名文件 |
| `source` | string | 否 | `app.conf` | 插件 `config/` 目录内的源文件名 |
| `create_dirs` | yes/no | 否 | `yes` | 目标父目录不存在时自动创建 |
| `backup` | yes/no | 否 | `yes` | 目标已存在时备份为 `<目标文件>.bak.<时间戳>` |
| `if_changed` | yes/no | 否 | `yes` | 内容与目标一致时跳过写入（幂等） |
| `mode` | string | 否 | 空 | 写入后权限位，如 `0644`；留空沿用目标原权限 |
| `owner` | string | 否 | 空 | 写入后属主 `user:group`；通常需要 root 权限 |

输出：`target_path`、`sha256`、`status`（`created` / `updated` / `skipped`）、`backup_path`、`bytes`。

### 2. `config.deploy.check` 检查配置差异

只读工具，不写任何文件。参数为 `target_path` 与 `source`，输出 `status`（`identical` / `different` / `missing`）和 `changed`，并打印 unified diff（最多 200 行）。

## 执行流程

`push` 依次执行：

1. 校验源文件：`source` 必须是 `config/` 目录内的相对路径，解析后的真实路径（含符号链接）仍需落在该目录内。
2. 解析目标：要求绝对路径；支持 `D:/etc/app.conf`、`D:\etc\app.conf`、`/etc/app.conf` 等写法；目标是目录时自动拼接源文件同名文件；目标是符号链接时写入其指向的真实文件。
3. 比对 SHA256：一致且 `if_changed=yes` 时跳过写入，返回 `status=skipped`。
4. 备份：目标存在且 `backup=yes` 时生成 `<目标文件>.bak.<YYYYmmddHHMMSS>`。
5. 原子写入：先写同目录临时文件，再设置权限/属主，最后 `mv` 覆盖目标文件，避免写入中断产生半截配置。
6. 写入后校验：比对目标文件与源配置的 SHA256，不一致直接失败并返回非 0。

## 使用示例

```bash
# 查看差异，不落盘
./bin/opsctl.exe run tool config.deploy.check \
  --set target_path=/etc/myapp/app.conf --no-prompt

# 下发配置（写入 /etc/myapp/app.conf）
./bin/opsctl.exe run tool config.deploy.push \
  --set target_path=/etc/myapp/app.conf --no-prompt

# 写入目录：等价于 /etc/myapp/app.conf
./bin/opsctl.exe run tool config.deploy.push \
  --set target_path=/etc/myapp/ --no-prompt

# 指定权限与属主
./bin/opsctl.exe run tool config.deploy.push \
  --set target_path=/etc/myapp/app.conf --set mode=0640 --set owner=app:app --no-prompt

# 使用参数文件
./bin/opsctl.exe run tool config.deploy.push \
  --params plugins/config.deploy/examples/params.yaml --no-prompt
```

Web 控制台：进入「配置部署」分类，编辑 `config/app.conf` 后运行工具并填写目标路径。

工作流：`target_path` 可接上传节点的文件路径输出，实现“上传配置 → 下发到指定路径”。

## 安全与限制

- 目标路径必须是绝对路径，避免相对路径落到运行目录造成误覆盖。
- `source` 不允许绝对路径，也不能用 `..` 逃逸 `config/` 目录（含符号链接解析后校验）。
- 目标文件与源配置文件为同一文件时直接拒绝执行。
- 覆盖写入默认开启备份，`backup=no` 时请自行确认目标文件来源。
- Linux 下写入 `/etc` 等受保护目录需要运行账号具备对应权限；需要保留 `root:root` 属主时显式传 `owner=root:root`。
- 属主设置失败会中止执行（此时目标文件尚未被替换），不会留下权限错误的配置。
