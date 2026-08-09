# 服务端 voice 模型/venv 门禁：缺则 setup，已有则 skip（幂等）。
# 不含桌宠前端 ONNX（见 desktop/scripts/download-models.ps1）。
# 用法: . scripts/lib/ensure-models.ps1; Ensure-MochiServerModels -RepoRoot $repo

function Ensure-MochiServerModels {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RepoRoot,
        [switch]$SkipEmotion2vec,
        [switch]$SkipXasr,
        [switch]$SkipXtts
    )

    $ErrorActionPreference = "Stop"

    function Write-StepLocal([string]$Msg) {
        Write-Host ""
        Write-Host "==> $Msg" -ForegroundColor Cyan
    }

    Write-StepLocal "Ensure server voice runtimes (tools/x-asr, tools/x-tts, services/emotion2vec)"

    if (-not $SkipXasr) {
        Write-StepLocal "Setup x-asr (tools/x-asr venv + models)"
        & (Join-Path $RepoRoot "scripts\start-xasr-sidecar.ps1") -SetupOnly
        if ($LASTEXITCODE -ne 0) { throw "x-asr setup failed" }
    }

    if (-not $SkipXtts) {
        Write-StepLocal "Setup x-tts (tools/x-tts venv + models)"
        & (Join-Path $RepoRoot "scripts\start-xtts-sidecar.ps1") -SetupOnly
        if ($LASTEXITCODE -ne 0) { throw "x-tts setup failed" }
    }

    if (-not $SkipEmotion2vec) {
        Write-StepLocal "Setup emotion2vec (services/emotion2vec venv + deps)"
        $emoScript = Join-Path $RepoRoot "services\emotion2vec\start.ps1"
        if (-not (Test-Path $emoScript)) { throw "Missing $emoScript" }
        & $emoScript -SetupOnly
        if ($LASTEXITCODE -ne 0) { throw "emotion2vec setup failed" }
    }

    Write-Host ""
    Write-Host "OK | server voice runtimes ready" -ForegroundColor Green
}
