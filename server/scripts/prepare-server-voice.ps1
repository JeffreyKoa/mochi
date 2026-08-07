# Prepare server-side voice runtimes (x-asr + emotion2vec venv).
# Does NOT stage client installer bundle (Phase3 slim client).
#
# Usage (repo root):
#   .\server\scripts\prepare-server-voice.ps1
#   .\server\scripts\prepare-server-voice.ps1 -SkipEmotion2vec
#
# Then start all backends:
#   .\scripts\restart-backend.ps1

param(
    [switch]$SkipEmotion2vec,
    [switch]$SkipXasr
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..\..")

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

Write-Host "Mochi prepare-server-voice" -ForegroundColor White
Write-Host "  repo: $RepoRoot"
Write-Host "  target: server host (tools/x-asr, services/emotion2vec)"
Write-Host "  note: client installer no longer bundles voice models (Phase3)"

if (-not $SkipXasr) {
    Write-Step "Setup x-asr (tools/x-asr venv + models)"
    & (Join-Path $RepoRoot "server\scripts\start-xasr-sidecar.ps1") -SetupOnly
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} else {
    Write-Step "Skip x-asr setup"
}

if (-not $SkipEmotion2vec) {
    Write-Step "Setup emotion2vec (services/emotion2vec venv + deps)"
    & (Join-Path $RepoRoot "services\emotion2vec\start.ps1") -SetupOnly
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} else {
    Write-Step "Skip emotion2vec setup"
}

Write-Host ""
Write-Host "OK | server voice runtimes ready" -ForegroundColor Green
Write-Host "Next: .\scripts\restart-backend.ps1" -ForegroundColor DarkGray
