# Start X-ASR sidecar on the Go server host (Phase1 cloud ASR)
# Usage:
#   .\scripts\start-xasr-sidecar.ps1
#   .\scripts\start-xasr-sidecar.ps1 -SetupOnly
#   .\scripts\start-xasr-sidecar.ps1 -Port 8766 -BindHost 127.0.0.1
#
# Logs (sidecar_log.py): set MOCHI_XASR_LOG_DIR

param(
    [switch]$SetupOnly,
    [int]$Port = 8766,
    [string]$BindHost = "127.0.0.1"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..")
$XAsrRoot = Join-Path $RepoRoot "tools\x-asr"
$SetupScript = Join-Path $XAsrRoot "setup-and-start.ps1"

if (-not (Test-Path $SetupScript)) {
    throw "X-ASR setup script not found: $SetupScript"
}

if (-not $env:MOCHI_XASR_LOG_DIR) {
    $defaultLogDir = Join-Path $RepoRoot "logs\xasr"
    New-Item -ItemType Directory -Force -Path $defaultLogDir | Out-Null
    $env:MOCHI_XASR_LOG_DIR = $defaultLogDir
}

Write-Host "Mochi server X-ASR sidecar" -ForegroundColor Cyan
Write-Host "  repo:     $RepoRoot"
Write-Host "  bind:     ws://${BindHost}:${Port}"
Write-Host "  log dir:  $($env:MOCHI_XASR_LOG_DIR)"
Write-Host ""

Push-Location $XAsrRoot
try {
    if ($SetupOnly) {
        & $SetupScript -Port $Port -BindHost $BindHost -SetupOnly
    } else {
        & $SetupScript -Port $Port -BindHost $BindHost
    }
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
