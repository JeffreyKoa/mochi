# Mochi X-ASR one-click setup + sidecar (Windows PowerShell 5.1+)
# Usage:
#   .\setup-and-start.ps1
#   .\setup-and-start.ps1 -SkipDownload
#   .\setup-and-start.ps1 -SlowModel     # 480ms 高精度模型（更慢）

param(
    [switch]$SkipDownload,
    [switch]$SetupOnly,
    [switch]$SlowModel,
    [int]$Port = 8766,
    [string]$BindHost = "127.0.0.1"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

$Repo = "GilgameshWind/X-ASR-zh-en"
$VenvDir = Join-Path $Root ".venv"
$VenvPython = Join-Path $VenvDir "Scripts\python.exe"
$InferDir = Join-Path $Root "infer"
$ModelDir = if ($SlowModel) {
    Join-Path $Root "models\chunk-480ms-model"
} else {
    Join-Path $Root "models\chunk-160ms-model"
}
$ChunkMs = if ($SlowModel) { "480ms" } else { "160ms" }
$EncoderOnnx = Join-Path $ModelDir ("encoder-{0}.onnx" -f $ChunkMs)
$ServerScript = Join-Path $InferDir "sherpa_streaming_server.py"

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
    # pip 未安装包时会把 WARNING 写到 stderr；Stop 模式下会误中断脚本
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

function Ensure-HfCli {
    $hfExe = Join-Path $VenvDir "Scripts\hf.exe"
    if (Test-Path $hfExe) { return }
    Write-Host "Install huggingface_hub CLI ..."
    Invoke-Pip @("install", "-U", "huggingface_hub[cli]")
}

function Invoke-HfDownload([string[]]$HfArgs) {
    Ensure-HfCli
    $hfExe = Join-Path $VenvDir "Scripts\hf.exe"
    if (-not (Test-Path $hfExe)) {
        throw "hf CLI not found after install: $hfExe"
    }
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & $hfExe @HfArgs
        if ($LASTEXITCODE -ne 0) {
            throw ("hf download failed (exit {0})" -f $LASTEXITCODE)
        }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Test-ModelsReady {
    return ((Test-Path $ServerScript) -and (Test-Path $EncoderOnnx))
}

function Install-Models {
    $modelFolder = if ($SlowModel) { "chunk-480ms-model" } else { "chunk-160ms-model" }
    Write-Step ("Download X-ASR scripts and {0} from Hugging Face" -f $modelFolder)
    Ensure-HfCli

    New-Item -ItemType Directory -Force -Path $InferDir | Out-Null
    New-Item -ItemType Directory -Force -Path (Join-Path $Root "models") | Out-Null

    $hfCache = Join-Path $Root "_hf_cache"

    Write-Host "Download infer scripts ..."
    Invoke-HfDownload @(
        "download", $Repo,
        "--include", "deployment/infer_and_client/*",
        "--local-dir", $hfCache
    )

    $srcInfer = Join-Path $hfCache "deployment\infer_and_client"
    Copy-Item -Force (Join-Path $srcInfer "sherpa_streaming_server.py") $InferDir
    Copy-Item -Force (Join-Path $srcInfer "sherpa_streaming_infer.py") $InferDir
    Copy-Item -Force (Join-Path $srcInfer "sherpa_streaming_client.py") $InferDir

    Write-Host ("Download {0} (may take several minutes) ..." -f $modelFolder)
    Invoke-HfDownload @(
        "download", $Repo,
        "--include", ("deployment/models/{0}/*" -f $modelFolder),
        "--local-dir", $hfCache
    )

    $srcModel = Join-Path $hfCache ("deployment\models\{0}" -f $modelFolder)
    New-Item -ItemType Directory -Force -Path $ModelDir | Out-Null
    Copy-Item -Force -Recurse (Join-Path $srcModel "*") $ModelDir

    if (-not (Test-Path $EncoderOnnx)) {
        throw "Model missing after download: $EncoderOnnx"
    }
    Write-Host "Models ready: $ModelDir" -ForegroundColor Green
}

function Invoke-XAsrSidecar {
    $uri = "ws://{0}:{1}" -f $BindHost, $Port
    Write-Step "Start X-ASR sidecar $uri (chunk=$ChunkMs)"
    Write-Host "Keep this window open. Mochi: Settings -> Voice -> Local/Auto STT." -ForegroundColor Green
    Write-Host "Press Ctrl+C to stop." -ForegroundColor DarkGray

    & $VenvPython $ServerScript `
        --host $BindHost `
        --port $Port `
        --tokens (Join-Path $ModelDir "tokens.txt") `
        --encoder (Join-Path $ModelDir ("encoder-{0}.onnx" -f $ChunkMs)) `
        --decoder (Join-Path $ModelDir ("decoder-{0}.onnx" -f $ChunkMs)) `
        --joiner (Join-Path $ModelDir ("joiner-{0}.onnx" -f $ChunkMs)) `
        --provider cpu `
        --sample-rate 16000 `
        --feature-dim 80 `
        --num-threads 4 `
        --decoding-method greedy_search `
        --model-type zipformer2 `
        --enable-endpoint-detection 0 `
        --text-format none
}

Write-Host "Mochi X-ASR setup-and-start" -ForegroundColor White

Write-Step "Check Python"
$py = Find-Python
Write-Host "Using: $py"

Ensure-Venv $py
Ensure-PipDeps

if (-not $SkipDownload -and -not (Test-ModelsReady)) {
    Install-Models
}
if (-not (Test-ModelsReady) -and -not $SlowModel) {
    $fallback480 = Join-Path $Root "models\chunk-480ms-model\encoder-480ms.onnx"
    if (Test-Path $fallback480) {
        Write-Host "160ms model not found, falling back to 480ms. Re-run without -SkipDownload to fetch 160ms." -ForegroundColor Yellow
        $SlowModel = $true
        $ModelDir = Join-Path $Root "models\chunk-480ms-model"
        $ChunkMs = "480ms"
        $EncoderOnnx = Join-Path $ModelDir "encoder-480ms.onnx"
    }
}
if (-not (Test-ModelsReady)) {
    throw "Models not ready. Run without -SkipDownload first."
}

Write-Host ""
Write-Host "OK | venv: $VenvDir" -ForegroundColor Green
Write-Host "OK | model: $ModelDir" -ForegroundColor Green

if ($SetupOnly) {
    Write-Host "SetupOnly: skip server start." -ForegroundColor Yellow
    exit 0
}

Invoke-XAsrSidecar
exit $LASTEXITCODE
