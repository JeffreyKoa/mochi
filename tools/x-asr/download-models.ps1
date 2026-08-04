# 从 Hugging Face 下载 X-ASR 脚本与模型（可单独运行；推荐用 setup-and-start.ps1）
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $Root "setup-and-start.ps1") -SetupOnly
