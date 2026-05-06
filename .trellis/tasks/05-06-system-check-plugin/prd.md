# 系统资源检查工具插件

## Goal

创建一个跨 Linux 发行版的系统资源检查工具插件，支持检查系统版本、CPU、内存、磁盘等资源信息，兼容 CentOS、Ubuntu、Debian、RHEL 等主流发行版。

## Requirements

### 检查项

1. **系统信息**
   - 发行版名称和版本（通过 `/etc/os-release`）
   - 内核版本（`uname -r`）
   - 主机名（`hostname`）
   - 系统架构（`uname -m`）

2. **CPU 信息**
   - CPU 型号
   - CPU 核心数（物理核心 + 逻辑核心）
   - CPU 使用率

3. **内存信息**
   - 总内存
   - 已用内存
   - 可用内存
   - 内存使用率

4. **磁盘信息**
   - 各分区挂载点
   - 总容量
   - 已用容量
   - 可用容量
   - 使用率

5. **负载信息**
   - 1 分钟、5 分钟、15 分钟平均负载

### 输出格式

- 结构化 JSON 输出（便于后续处理）
- 人类可读的文本输出（便于直接查看）
- 支持通过参数选择输出格式

## Technical Approach

### 插件结构

```
plugins/plugin.system-check/
├── plugin.yaml           # 插件清单
├── scripts/
│   └── check.sh         # 系统检查脚本
└── README.md            # 使用说明
```

### 脚本实现

**跨发行版兼容性**
- 使用标准 POSIX 命令
- 优先使用 `/proc` 文件系统
- 避免依赖特定发行版的工具

**关键命令**
- 系统信息：`/etc/os-release`、`uname`
- CPU：`/proc/cpuinfo`、`top` 或 `mpstat`
- 内存：`/proc/meminfo` 或 `free`
- 磁盘：`df`
- 负载：`/proc/loadavg` 或 `uptime`

## Acceptance Criteria

* [ ] 插件可以在 CentOS、Ubuntu、Debian 上运行
* [ ] 输出包含所有必需的系统信息
* [ ] 支持 JSON 和文本两种输出格式
* [ ] 脚本有错误处理和友好提示
* [ ] 通过 `opsctl validate` 验证
* [ ] 通过 `opsctl run tool` 执行成功
