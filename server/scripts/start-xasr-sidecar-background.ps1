# Background start X-ASR sidecar (after setup)
param(
    [int]$Port = 8766,
    [string]$BindHost = "127.0.0.1",
    [string]$RepoRoot = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if ($RepoRoot -eq "") {
    $RepoRoot = (Resolve-Path (Join-Path $ScriptDir "..\..")).Path
}
. (Join-Path $RepoRoot "scripts\lib\daily-log.ps1")

$XAsrRoot = Join-Path $RepoRoot "tools\x-asr"
$VenvPy = Join-Path $XAsrRoot ".venv\Scripts\python.exe"
$ModelDir = Join-Path $XAsrRoot "models\chunk-160ms-model"
$ChunkMs = "160ms"
$ServerScript = Join-Path $XAsrRoot "infer\sherpa_streaming_server.py"

if (-not (Test-Path $VenvPy)) { throw "X-ASR venv missing. Run start-xasr-sidecar.ps1 -SetupOnly first." }
if (-not (Test-Path $ServerScript)) { throw "X-ASR server script missing: $ServerScript" }

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

$proc = Start-MochiSidecarProcess `
    -FilePath $VenvPy `
    -ArgumentList $pyArgs `
    -WorkingDirectory $XAsrRoot

Write-Output $proc.Id
