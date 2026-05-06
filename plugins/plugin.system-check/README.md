# 系统资源检查工具插件

## 功能说明

跨 Linux 发行版的系统资源检查工具，支持检查系统版本、CPU、内存、磁盘等信息。

兼容 CentOS、Ubuntu、Debian、RHEL 等主流发行版。

## 检查项

- **系统信息**：发行版名称、版本、内核、主机名、架构
- **CPU 信息**：型号、物理核心、逻辑核心、使用率
- **内存信息**：总内存、已用、可用、使用率
- **磁盘信息**：各分区容量、使用率、挂载点
- **系统负载**：1/5/15 分钟平均负载

## 使用方法

### CLI 执行

```bash
# 文本格式输出（默认）
./bin/opsctl.exe run tool plugin.system-check.info

# JSON 格式输出
./bin/opsctl.exe run tool plugin.system-check.info --set format=json
```

### Web UI 执行

1. 打开 Web UI：`./bin/opsctl.exe serve`
2. 访问 http://127.0.0.1:8080
3. 在"系统管理"分类下找到"系统资源检查"
4. 选择输出格式（text 或 json）
5. 点击"执行"

## 输出示例

### 文本格式

```
=========================================
系统资源检查报告
=========================================

【系统信息】
  操作系统: Ubuntu 22.04.3 LTS
  内核版本: 5.15.0-91-generic
  主机名: server01
  系统架构: x86_64

【CPU 信息】
  型号: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz
  物理核心: 2
  逻辑核心: 4
  使用率: 15.3%

【内存信息】
  总内存: 15.63 GB
  已用内存: 8.42 GB
  可用内存: 7.21 GB
  使用率: 53.85%

【磁盘信息】
  设备                 总容量     已用       可用       使用率     挂载点
  /dev/sda1            50G        25G        23G        53%        /
  /dev/sdb1            100G       45G        51G        47%        /data

【系统负载】
  1 分钟: 0.52
  5 分钟: 0.68
  15 分钟: 0.71

=========================================
检查完成
=========================================
```

### JSON 格式

```json
{
  "system": {
    "os_name": "Ubuntu",
    "os_version": "22.04.3 LTS",
    "kernel": "5.15.0-91-generic",
    "hostname": "server01",
    "architecture": "x86_64"
  },
  "cpu": {
    "model": "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz",
    "physical_cores": 2,
    "logical_cores": 4,
    "usage_percent": "15.3"
  },
  "memory": {
    "total_kb": 16384000,
    "used_kb": 8823296,
    "available_kb": 7560704,
    "usage_percent": "53.85"
  },
  "disk": [
    {
      "device": "/dev/sda1",
      "size": "50G",
      "used": "25G",
      "available": "23G",
      "usage": "53%",
      "mount": "/"
    }
  ],
  "load": {
    "load_1min": "0.52",
    "load_5min": "0.68",
    "load_15min": "0.71"
  }
}
```

## 兼容性

- ✅ CentOS 7/8
- ✅ Ubuntu 18.04/20.04/22.04
- ✅ Debian 10/11
- ✅ RHEL 7/8/9
- ✅ 其他使用标准 `/proc` 文件系统的 Linux 发行版

## 技术说明

- 使用标准 POSIX shell 命令，无需额外依赖
- 优先读取 `/proc` 文件系统，确保跨发行版兼容
- 脚本包含错误处理和参数验证
- 支持文本和 JSON 两种输出格式

## 许可证

MIT
