@echo off
setlocal EnableExtensions

set "SERVICE_NAME="
set "ACTION=status"
set "MANAGER=auto"

:parse
if "%~1"=="" goto after_parse
rem Label jumps inside a for block break parsing on older cmd.exe, so stay in plain if branches.
if /i "%~1"=="--service" if "%~2"=="" goto missing_value
if /i "%~1"=="--action" if "%~2"=="" goto missing_value
if /i "%~1"=="--manager" if "%~2"=="" goto missing_value
if /i "%~1"=="--service" (
  set "SERVICE_NAME=%~2"
  shift
  shift
  goto parse
)
if /i "%~1"=="--action" (
  set "ACTION=%~2"
  shift
  shift
  goto parse
)
if /i "%~1"=="--manager" (
  set "MANAGER=%~2"
  shift
  shift
  goto parse
)
:missing_value
chcp 65001 >nul
echo 错误: 参数 %1 缺少取值 1>&2
goto fail_usage

:parse_flags
if /i "%~1"=="-h" goto usage
if /i "%~1"=="--help" goto usage
chcp 65001 >nul
echo 错误: 未知参数 %~1 1>&2
goto fail_usage
:after_parse

if /i "%ACTION%"=="restart" goto action_ok
if /i "%ACTION%"=="status" goto action_ok
chcp 65001 >nul
echo 错误: --action 仅支持 restart 或 status，当前值: %ACTION% 1>&2
goto fail_usage
:action_ok

if /i "%MANAGER%"=="auto" goto manager_ok
if /i "%MANAGER%"=="sc" goto manager_ok
chcp 65001 >nul
echo 错误: Windows 目标仅支持 --manager auto 或 sc，当前值: %MANAGER% 1>&2
goto fail_usage
:manager_ok

rem Validate the name before printing anything: a missing name makes sc query list all services.
set "SERVICE_NAME=%SERVICE_NAME: =%"
if "%SERVICE_NAME%"=="" (
  chcp 65001 >nul
  echo 错误: --service 必填 1>&2
  goto fail_usage
)
echo %SERVICE_NAME% | findstr /R /C:"[*.?]" >nul
if not errorlevel 1 (
  chcp 65001 >nul
  echo 错误: 服务名不合法: %SERVICE_NAME% 1>&2
  goto fail_usage
)

where sc >nul 2>nul
if errorlevel 1 (
  chcp 65001 >nul
  echo 错误: 未找到 sc 命令，确认当前目标为 Windows 管理机 1>&2
  exit /b 1
)

rem Avoid matching localized sc output: report unknown services through sc query exit codes.
sc query "%SERVICE_NAME%" >nul 2>nul
if errorlevel 1060 (
  chcp 65001 >nul
  echo 错误: 服务不存在: %SERVICE_NAME% 1>&2
  exit /b 1
)

chcp 65001 >nul
echo 服务: %SERVICE_NAME%
echo 动作: %ACTION%
echo 管理方式: sc

if /i "%ACTION%"=="restart" goto restart
goto status

:restart
sc stop "%SERVICE_NAME%" >nul 2>nul
sc start "%SERVICE_NAME%" >nul
if errorlevel 1 (
  chcp 65001 >nul
  echo 错误: 重启服务失败: %SERVICE_NAME% 1>&2
  exit /b 1
)
chcp 65001 >nul
echo 服务已重启: %SERVICE_NAME%
echo {"service":"%SERVICE_NAME%","action":"restart","status":"ok","manager":"sc"}
exit /b 0

:status
sc query "%SERVICE_NAME%" | findstr /L /C:"RUNNING" >nul
if not errorlevel 1 goto status_running
sc query "%SERVICE_NAME%" | findstr /L /C:"STOPPED" >nul
if not errorlevel 1 (
  chcp 65001 >nul
  echo 错误: 服务未运行: %SERVICE_NAME% 1>&2
  exit /b 1
)
chcp 65001 >nul
echo 错误: 服务状态异常: %SERVICE_NAME% 1>&2
exit /b 1

:status_running
chcp 65001 >nul
echo 服务运行正常: %SERVICE_NAME%
echo {"service":"%SERVICE_NAME%","action":"status","status":"active","manager":"sc"}
exit /b 0

:usage
chcp 65001 >nul
echo 用法: service.cmd --service SERVICE_NAME --action restart OR status --manager auto OR sc
echo 选项:
echo   --action restart / status   重启服务或检查服务状态，默认 status
echo   --manager auto / sc         Windows 固定使用 sc
exit /b 0

:fail_usage
exit /b 1
