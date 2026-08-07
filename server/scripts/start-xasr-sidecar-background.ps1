# 后台启动 X-ASR sidecar（setup 完成后调用）
param(
    [int]$Port = 8766,
    [string]$BindHost = "127.0.0.1",
    [string]$LogDir = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$XAsrRoot = Join-Path $RepoRoot "tools\x-asr"
$VenvPy = Join-Path $XAsrRoot ".venv\Scripts\python.exe"
$ModelDir = Join-Path $XAsrRoot "models\chunk-160ms-model"
$ChunkMs = "160ms"
$ServerScript = Join-Path $XAsrRoot "infer\sherpa_streaming_server.py"

if (-not (Test-Path $VenvPy)) { throw "X-ASR venv missing. Run start-xasr-sidecar.ps1 -SetupOnly first." }
if (-not (Test-Path $ServerScript)) { throw "X-ASR server script missing: $ServerScript" }

if (-not $env:MOCHI_XASR_LOG_DIR) {
    $env:MOCHI_XASR_LOG_DIR = Join-Path $RepoRoot "server\logs\x-asr"
    New-Item -ItemType Directory -Force -Path $env:MOCHI_XASR_LOG_DIR | Out-Null
}

if ($LogDir -ne "") {
    New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
    $outLog = Join-Path $LogDir "xasr-launcher-out.log"
    $errLog = Join-Path $LogDir "xasr-launcher-err.log"
} else {
    $outLog = Join-Path $RepoRoot "server\logs\x-asr\launcher-out.log"
    $errLog = Join-Path $RepoRoot "server\logs\x-asr\launcher-err.log"
}

$pyArgs = @(
    $ServerScript,
    "--host", $BindHost,
    "--port", "$Port",
    "--tokens", (Join-Path $ModelDir "tokens.txt"),
    "--encoder", (Join-Path $ModelDir "encoder-$ChunkMs.onnx"),
    "--decoder", (Join-Path $ModelDir "decoder-$ChunkMs.onnx"),
    "--joiner", (Join-Path $ModelDir "joiner-$ChunkMs.onnx"),
    "--provider", "cpu",
    "--sample-rate", "16000",
    "--feature-dim", "80",
    "--num-threads", "4",
    "--decoding-method", "greedy_search",
    "--model-type", "zipformer2",
    "--enable-endpoint-detection", "0",
    "--text-format", "none"
)

$proc = Start-Process -FilePath $VenvPy `
    -ArgumentList $pyArgs `
    -WorkingDirectory $XAsrRoot `
    -WindowStyle Hidden `
    -RedirectStandardOutput $outLog `
    -RedirectStandardError $errLog `
    -PassThru

Write-Output $proc.Id
