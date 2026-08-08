# Background start X-TTS sidecar (after setup)
param(
    [int]$Port = 8767,
    [string]$BindHost = "127.0.0.1",
    [int]$NumThreads = 2,
    [string]$RepoRoot = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if ($RepoRoot -eq "") {
    $RepoRoot = (Resolve-Path (Join-Path $ScriptDir "..\..")).Path
}
. (Join-Path $RepoRoot "scripts\lib\daily-log.ps1")

$XTtsRoot = Join-Path $RepoRoot "tools\x-tts"
$VenvPy = Join-Path $XTtsRoot ".venv\Scripts\python.exe"
$ServerScript = Join-Path $XTtsRoot "infer\tts_server.py"
$ModelDir = Join-Path $XTtsRoot "models\matcha-zh-en"
$VocoderPath = Join-Path $XTtsRoot "models\vocos-16khz-univ.onnx"

if (-not (Test-Path $VenvPy)) { throw "X-TTS venv missing. Run start-xtts-sidecar.ps1 -SetupOnly first." }
if (-not (Test-Path $ServerScript)) { throw "X-TTS server script missing: $ServerScript" }

$pyArgs = @(
    $ServerScript,
    "--host", $BindHost,
    "--port", "$Port",
    "--model-dir", $ModelDir,
    "--vocoder", $VocoderPath,
    "--num-threads", "$NumThreads"
)

$proc = Start-MochiSidecarProcess `
    -FilePath $VenvPy `
    -ArgumentList $pyArgs `
    -WorkingDirectory $XTtsRoot

Write-Output $proc.Id
