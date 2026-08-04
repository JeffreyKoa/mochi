# 仅启动 sidecar（需已完成 setup-and-start 或 download-models）
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $Root "setup-and-start.ps1") -SkipDownload
