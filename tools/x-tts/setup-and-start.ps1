# Mochi X-TTS one-click setup + HTTP sidecar (Windows PowerShell 5.1+)
# Usage:
#   .\setup-and-start.ps1
#   .\setup-and-start.ps1 -SkipDownload
#   .\setup-and-start.ps1 -UseModelScope   # 从魔搭下载 Matcha 模型

param(
    [switch]$SkipDownload,
    [switch]$SetupOnly,
    [switch]$UseModelScope,
    [int]$Port = 8767,
    [string]$BindHost = "127.0.0.1",
    [int]$NumThreads = 2
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

$VenvDir = Join-Path $Root ".venv"
$VenvPython = Join-Path $VenvDir "Scripts\python.exe"
$ServerScript = Join-Path $Root "infer\tts_server.py"
$ModelDir = Join-Path $Root "models\matcha-zh-en"
$VocoderPath = Join-Path $Root "models\vocos-16khz-univ.onnx"
$AcousticOnnx = Join-Path $ModelDir "model-steps-3.onnx"

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

function Find-Python {
    $paths = @()
    $cmd = Get-Command python -ErrorAction SilentlyContinue
    if ($cmd) { $paths += $cmd.Source }
    if (Get-Command py -ErrorAction SilentlyContinue) {
        foreach ($ver in @("-3.11", "-3.10", "-3")) {
            try {
                $p = & py $ver -c "import sys; print(sys.executable)" 2>$null
                if ($p) { $paths += $p }
            } catch { }
        }
    }
    foreach ($path in ($paths | Select-Object -Unique)) {
        if (-not (Test-Path $path)) { continue }
        $ver = & $path -c "import sys; print('%s.%s' % (sys.version_info.major, sys.version_info.minor))"
        $parts = $ver -split '\.'
        $major = [int]$parts[0]
        $minor = [int]$parts[1]
        if ($major -eq 3 -and $minor -ge 10 -and $minor -le 12) {
            return $path
        }
        Write-Host "Skip Python $ver (need 3.10-3.12): $path" -ForegroundColor DarkYellow
    }
    throw "Python 3.10-3.12 not found. Install from https://www.python.org/downloads/"
}

function Ensure-Venv([string]$PythonExe) {
    if (Test-Path $VenvPython) { return }
    Write-Step "Create venv .venv"
    & $PythonExe -m venv $VenvDir
    if (-not (Test-Path $VenvPython)) {
        throw "Failed to create venv"
    }
}

function Invoke-Pip([string[]]$PipArgs) {
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & $VenvPython -m pip @PipArgs
        if ($LASTEXITCODE -ne 0) {
            throw ("pip failed: pip {0}" -f ($PipArgs -join " "))
        }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Ensure-PipDeps {
    Write-Step "Install Python dependencies"
    Invoke-Pip @("install", "--upgrade", "pip", "wheel")
    Invoke-Pip @("install", "-r", (Join-Path $Root "requirements.txt"))
}

function Test-ModelsReady {
    return (
        (Test-Path $ServerScript) -and
        (Test-Path $AcousticOnnx) -and
        (Test-Path $VocoderPath)
    )
}

function Invoke-TtsSidecar {
    $uri = "http://{0}:{1}" -f $BindHost, $Port
    Write-Step "Start X-TTS sidecar $uri"
    Write-Host "Keep this window open. Mochi tts_mode: local will use this server." -ForegroundColor Green
    Write-Host "Test: Invoke-RestMethod $uri/health" -ForegroundColor DarkGray
    Write-Host "Press Ctrl+C to stop." -ForegroundColor DarkGray

    & $VenvPython $ServerScript `
        --host $BindHost `
        --port $Port `
        --model-dir $ModelDir `
        --vocoder $VocoderPath `
        --num-threads $NumThreads
}

Write-Host "Mochi X-TTS setup-and-start" -ForegroundColor White

Write-Step "Check Python"
$py = Find-Python
Write-Host "Using: $py"

Ensure-Venv $py
Ensure-PipDeps

if (-not $SkipDownload -and -not (Test-ModelsReady)) {
    $dlArgs = @()
    if ($UseModelScope) { $dlArgs += "-UseModelScope" }
    & (Join-Path $Root "download-models.ps1") @dlArgs
    if ($LASTEXITCODE -ne 0) { throw "Model download failed" }
}

if (-not (Test-ModelsReady)) {
    throw "Models not ready. Run without -SkipDownload first."
}

Write-Host ""
Write-Host "OK | venv: $VenvDir" -ForegroundColor Green
Write-Host "OK | model: $ModelDir" -ForegroundColor Green
Write-Host "OK | vocoder: $VocoderPath" -ForegroundColor Green

if ($SetupOnly) {
    Write-Host "SetupOnly: skip server start." -ForegroundColor Yellow
    exit 0
}

Invoke-TtsSidecar
exit $LASTEXITCODE
