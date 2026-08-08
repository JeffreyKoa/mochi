# Start X-TTS sidecar on the Go server host (Matcha/sherpa-onnx HTTP :8767)
# Usage:
#   .\server\scripts\start-xtts-sidecar.ps1
#   .\server\scripts\start-xtts-sidecar.ps1 -SetupOnly
#   .\server\scripts\start-xtts-sidecar.ps1 -Port 8767 -BindHost 127.0.0.1

param(
    [switch]$SetupOnly,
    [int]$Port = 8767,
    [string]$BindHost = "127.0.0.1",
    [int]$NumThreads = 2
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$XTtsRoot = Join-Path $RepoRoot "tools\x-tts"
$SetupScript = Join-Path $XTtsRoot "setup-and-start.ps1"

if (-not (Test-Path $SetupScript)) {
    throw "X-TTS setup script not found: $SetupScript"
}

Write-Host "Mochi server X-TTS sidecar" -ForegroundColor Cyan
Write-Host "  repo:     $RepoRoot"
Write-Host "  bind:     http://${BindHost}:${Port}"
Write-Host ""

Push-Location $XTtsRoot
try {
    if ($SetupOnly) {
        & $SetupScript -Port $Port -BindHost $BindHost -NumThreads $NumThreads -SetupOnly
    } else {
        & $SetupScript -Port $Port -BindHost $BindHost -NumThreads $NumThreads
    }
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
