param(
  [Parameter(Mandatory = $true)][string]$Service,
  [string]$Action = "status",
  [string]$Manager = "auto"
)

$ErrorActionPreference = "Stop"

function Write-Json([string]$Text) {
  [Console]::Out.WriteLine($Text)
}

function ConvertTo-JsonString([string]$Value) {
  if ($null -eq $Value) {
    return '""'
  }
  $builder = New-Object System.Text.StringBuilder
  [void]$builder.Append('"')
  foreach ($char in $Value.ToCharArray()) {
    switch ([int]$char) {
      34 { [void]$builder.Append('\"') }
      92 { [void]$builder.Append('\\') }
      10 { [void]$builder.Append('\n') }
      13 { [void]$builder.Append('\r') }
      default { [void]$builder.Append($char) }
    }
  }
  [void]$builder.Append('"')
  return $builder.ToString()
}

function Stop-WithError([string]$Message) {
  $json = '{"service":' + (ConvertTo-JsonString $Service) + ',"action":' + (ConvertTo-JsonString $Action) + ',"status":"error","error":' + (ConvertTo-JsonString $Message) + '}'
  Write-Json $json
  [Console]::Error.WriteLine("错误: $Message")
  [Console]::Out.Flush()
  exit 1
}

function Initialize-ServiceApi {
  # C# 实现放在独立文件，Windows PowerShell 5.1 按 ANSI 解析无 BOM 脚本，此处控制逻辑使用纯 ASCII。
  $source = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'service_api.cs')
  Add-Type -TypeDefinition $source -ErrorAction Stop
}

function Wait-ServiceState([string]$Name, [string[]]$States, [int]$TimeoutSeconds) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ($true) {
    $current = [OpenOps.WinService.ServiceApi]::State($Name)
    if ($States -contains $current) {
      return $current
    }
    if ((Get-Date) -ge $deadline) {
      return $current
    }
    Start-Sleep -Milliseconds 500
  }
}

if ($Action -ne "restart" -and $Action -ne "status") {
  Stop-WithError "--action 仅支持 restart 或 status，当前值: $Action"
}
if ($Manager -ne "auto" -and $Manager -ne "sc") {
  Stop-WithError "Windows 目标仅支持 --manager auto 或 sc，当前值: $Manager"
}

$Service = $Service -replace ' ', ''
if ([string]::IsNullOrEmpty($Service)) {
  Stop-WithError "--service 必填"
}
if ($Service.IndexOfAny([char[]]@('*', '?')) -ge 0) {
  Stop-WithError "服务名不合法: $Service"
}

$nameJson = ConvertTo-JsonString $Service

# sc 的输出文本随系统语言变化，非英文系统上无法可靠匹配 RUNNING，因此直接查询服务控制状态。
try {
  Initialize-ServiceApi
  if ($Action -eq "restart") {
    $current = [OpenOps.WinService.ServiceApi]::State($Service)
    if ($current -ne "stopped") {
      [OpenOps.WinService.ServiceApi]::Stop($Service)
      $current = Wait-ServiceState $Service @("stopped") 30
      if ($current -ne "stopped") {
        Stop-WithError "停止服务超时，当前状态: $current"
      }
    }
    [OpenOps.WinService.ServiceApi]::Start($Service)
    $current = Wait-ServiceState $Service @("running") 60
    if ($current -ne "running") {
      Stop-WithError "服务启动失败，当前状态: $current"
    }
    Write-Json "服务已重启: $Service"
    Write-Json ('{"service":' + $nameJson + ',"action":"restart","status":"ok","manager":"sc"}')
  } else {
    $current = [OpenOps.WinService.ServiceApi]::State($Service)
    if ($current -ne "running") {
      Stop-WithError "服务未运行，当前状态: $current"
    }
    Write-Json "服务运行正常: $Service"
    Write-Json ('{"service":' + $nameJson + ',"action":"status","status":"active","manager":"sc"}')
  }
} catch {
  $detail = $_.Exception
  if ($detail.InnerException) {
    $detail = $detail.InnerException
  }
  Stop-WithError $detail.Message
}
[Console]::Out.Flush()
exit 0
