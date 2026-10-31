# k3s 维护工具插件

k3s 集群日常维护工具集：巡检、诊断、日志、服务控制、kubectl 与 etcd 快照，并提供 Helm 仓库、查询、安装升级、卸载、回滚操作。

所有工具均为 shell 脚本，默认在「本机」执行，也可以在运行时选择 SSH 操作机下发到目标 k3s 节点执行。脚本优先使用 `k3s kubectl`，找不到时回退到 `kubectl`；kubeconfig 自动探测顺序为：工具参数 > 环境变量 `KUBECONFIG` > `/etc/rancher/k3s/k3s.yaml`。

## 工具列表

### k3s 运维

| 工具 | 说明 | 变更风险 |
|------|------|----------|
| `k3s.status` | 只读巡检节点、Pod、服务与资源，输出集群健康摘要 | 无 |
| `k3s.diagnose` | 收集节点、Pod、事件、服务日志、资源与 etcd 快照，生成诊断报告文件 | 无 |
| `k3s.logs` | journalctl 查看 k3s / k3s-agent 日志，支持行数与时间范围 | 无 |
| `k3s.service` | 启动 / 停止 / 重启 k3s 服务（需二次确认） | 高，会短暂中断集群 API |
| `k3s.kubectl` | 从预设动作中选择执行 kubectl 操作，命名空间/资源名/副本数作为入参（需二次确认） | 取决于所选动作 |
| `k3s.etcd_snapshot` | etcd 快照保存 / 列出 / 按保留数量清理（需二次确认） | 中，占用磁盘或删除旧快照 |

### Helm

| 工具 | 说明 | 变更风险 |
|------|------|----------|
| `k3s.helm_repo` | 添加、更新、查看 chart 仓库 | 无 |
| `k3s.helm_query` | 查看发布列表、详情、历史版本 | 无 |
| `k3s.helm_deploy` | `helm upgrade --install` 安装或升级，支持 values、--set、等待、原子回滚（需二次确认） | 高 |
| `k3s.helm_uninstall` | 卸载发布，可选保留历史（需二次确认） | 高 |
| `k3s.helm_rollback` | 回滚到指定版本（需二次确认） | 高 |

### kubectl 预设动作

`k3s.kubectl` 不再自由填写整条命令，通过 `action` 下拉选择动作，并按需填写 `namespace`、`resource`、`replicas`、`lines`。

| action | 实际命令 | 需要入参 |
|--------|----------|----------|
| `get_nodes` | `kubectl get nodes -o wide` | - |
| `get_pods` | `kubectl get pods -o wide -n <ns>` | namespace |
| `get_pods_all` | `kubectl get pods -A -o wide` | - |
| `get_deployments` | `kubectl get deployments -o wide -n <ns>` | namespace |
| `get_services` | `kubectl get services -o wide -n <ns>` | namespace |
| `get_events` | `kubectl get events -n <ns> --sort-by=.lastTimestamp`（取最近 50 条） | namespace |
| `describe_node` | `kubectl describe node <node>` | resource=节点名 |
| `describe_pod` | `kubectl describe pod <pod> -n <ns>` | namespace + resource |
| `describe_deployment` | `kubectl describe deployment <name> -n <ns>` | namespace + resource |
| `logs_pod` | `kubectl logs <pod> -n <ns> --tail=<lines>` | namespace + resource + lines |
| `rollout_restart` | `kubectl rollout restart deployment/<name> -n <ns>` | namespace + resource |
| `rollout_status` | `kubectl rollout status deployment/<name> -n <ns>` | namespace + resource |
| `scale_deployment` | `kubectl scale deployment/<name> --replicas=<n> -n <ns>` | namespace + resource + replicas |
| `delete_pod` | `kubectl delete pod <pod> -n <ns>` | namespace + resource |
| `cordon_node` | `kubectl cordon <node>` | resource=节点名 |
| `uncordon_node` | `kubectl uncordon <node>` | resource=节点名 |
| `drain_node` | `kubectl drain <node> --ignore-daemonsets --delete-emptydir-data` | resource=节点名 |
| `top_nodes` | `kubectl top nodes` | - |
| `top_pods` | `kubectl top pods -n <ns>` | namespace |

## 使用示例

```bash
# 集群巡检（本机执行）
./bin/opsctl.exe run tool k3s.status --no-prompt

# 生成诊断报告
./bin/opsctl.exe run tool k3s.diagnose --set report_dir=/var/log/opsctl/k3s --no-prompt

# 查看 k3s-agent 最近 500 行日志
./bin/opsctl.exe run tool k3s.logs --set service=k3s-agent --set lines=500 --set since=30m --no-prompt

# 重启 k3s 服务（交互确认）
./bin/opsctl.exe run tool k3s.service --set action=restart --set service=auto

# 执行 kubectl 预设动作：查看 prod 命名空间的 Pod
./bin/opsctl.exe run tool k3s.kubectl --set action=get_pods --set namespace=prod --no-prompt

# 重启 prod 命名空间的 myapp Deployment
./bin/opsctl.exe run tool k3s.kubectl --set action=rollout_restart --set namespace=prod --set resource=myapp --no-prompt

# 升级前保存 etcd 快照
./bin/opsctl.exe run tool k3s.etcd_snapshot --set action=save --set name=before-upgrade --no-prompt

# 添加 Helm 仓库并安装发布
./bin/opsctl.exe run tool k3s.helm_repo --set action=add --set name=bitnami --set url=https://charts.bitnami.com/bitnami --no-prompt
./bin/opsctl.exe run tool k3s.helm_deploy --set release=myapp --set chart=bitnami/nginx --set namespace=prod --set dry_run=yes --no-prompt
```

## 工作流

插件贡献 `k3s.inspect`（k3s 日常巡检）：串行执行「集群状态巡检 → 生成诊断报告」，全部只读。可在工作流页面把节点目标改为 SSH 操作机后对远端节点巡检，报告路径可通过 `{{ .steps.diagnose.outputs.report_path }}` 被后续节点引用。

## 参数说明

- `kubeconfig`：可选，指定 kubeconfig 路径；留空时自动探测。
- `service`：`auto` / `k3s` / `k3s-agent`，脚本自动识别当前节点安装的服务。
- `report_dir`：诊断报告目录，默认 `/var/log/opsctl/k3s`；不可写时回退到 `$TMPDIR/opsctl-k3s`。
- `dry_run=yes`：Helm 安装升级只渲染不提交，建议首次发布前先 dry-run。

## 前置依赖

- 目标节点已安装 k3s（含 `k3s` 命令或可用的 `kubectl`）。
- 执行服务控制、日志、etcd 快照通常需要 root 权限；非 root 用户请配置免密 sudo 或使用 root 登录的操作机。
- Helm 操作需要 Helm 3（`helm` 在 PATH 中，或安装到 `/usr/local/bin/helm`、`/usr/bin/helm`、`~/.local/bin/helm`）。
- etcd 快照仅适用于使用内置 etcd 存储的 k3s server 节点。

## 风险提示

- `k3s.service` 重启/停止会短暂中断该节点的 Kubernetes API 和网络控制面，请选择变更窗口。
- `k3s.kubectl` 支持任意 kubectl 参数（含引号与管道），执行前请核对命令涉及的命名空间与对象。
- `k3s.etcd_snapshot` 的 `save` 会写入磁盘，`prune` 会删除超出保留数量的旧快照，执行前确认保留策略。
- Helm 卸载会删除发布关联的集群资源；建议先执行 `k3s.helm_query` 确认当前版本，并保留 `keep_history=yes` 以便追溯。
- 生产环境建议先在工作流中串联 `k3s.status` 与 `k3s.diagnose` 留存基线，再做变更。

## 维护联系人

- 插件负责人：ops-team
- 问题反馈：提交到本仓库对应插件目录