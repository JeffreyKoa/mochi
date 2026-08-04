param(
    [switch]$UseModelScope,
    [string]$ModelDir = ""
)
# 从 ModelScope / HuggingFace 下载 Matcha zh-en TTS 模型（sherpa-onnx 格式）

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

$VenvPython = Join-Path $Root ".venv\Scripts\python.exe"
$MatchaDir = if ($ModelDir) { $ModelDir } else { Join-Path $Root "models\matcha-zh-en" }
$VocoderPath = Join-Path $Root "models\vocos-16khz-univ.onnx"
$VocoderUrl = "https://github.com/k2-fsa/sherpa-onnx/releases/download/vocoder-models/vocos-16khz-univ.onnx"

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

function Ensure-Venv {
    if (-not (Test-Path $VenvPython)) {
        throw "Run setup-and-start.ps1 first to create .venv"
    }
}

function Invoke-DownloadPy([string]$ScriptBody) {
    $tmpPy = Join-Path $env:TEMP ("mochi-xtts-{0}.py" -f ([guid]::NewGuid().ToString("N")))
    Set-Content -Path $tmpPy -Value $ScriptBody -Encoding UTF8
    try {
        & $VenvPython $tmpPy
        if ($LASTEXITCODE -ne 0) { throw "Python download script failed" }
    } finally {
        Remove-Item -Force $tmpPy -ErrorAction SilentlyContinue
    }
}

function Download-Vocoder {
    if (Test-Path $VocoderPath) {
        Write-Host "Vocoder exists: $VocoderPath" -ForegroundColor DarkGray
        return
    }
    Write-Step "Download vocos-16khz-univ.onnx"
    New-Item -ItemType Directory -Force -Path (Join-Path $Root "models") | Out-Null
    Invoke-WebRequest -Uri $VocoderUrl -OutFile $VocoderPath -UseBasicParsing
    Write-Host "Saved: $VocoderPath" -ForegroundColor Green
}

function Download-Matcha-HF {
    Write-Step "Download Matcha zh-en from HuggingFace (csukuangfj/matcha-icefall-zh-en)"
    Write-Host "Source mirrors ModelScope: dengcunqin/matcha_tts_zh_en_20251010" -ForegroundColor DarkGray

    $cache = (Join-Path $Root "_hf_cache") -replace '\\', '/'
    $dest = $MatchaDir -replace '\\', '/'
    $py = @"
from pathlib import Path
import shutil
from huggingface_hub import snapshot_download

cache = Path(r'$cache')
dest = Path(r'$dest')
cache.mkdir(parents=True, exist_ok=True)
dest.mkdir(parents=True, exist_ok=True)
snapshot_download('csukuangfj/matcha-icefall-zh-en', local_dir=str(cache))
for item in cache.iterdir():
    target = dest / item.name
    if item.is_dir():
        if target.exists():
            shutil.rmtree(target)
        shutil.copytree(item, target)
    else:
        shutil.copy2(item, target)
print('OK', dest)
"@
    Invoke-DownloadPy $py
}

function Download-Matcha-MS {
    Write-Step "Download Matcha zh-en from ModelScope (dengcunqin/matcha_tts_zh_en_20251010)"
    $dest = $MatchaDir -replace '\\', '/'
    $py = @"
from pathlib import Path
from modelscope import snapshot_download

dest = Path(r'$dest')
dest.mkdir(parents=True, exist_ok=True)
snapshot_download('dengcunqin/matcha_tts_zh_en_20251010', local_dir=str(dest))
print('OK', dest)
"@
    Invoke-DownloadPy $py
}

function Test-MatchaReady {
    $acoustic = Join-Path $MatchaDir "model-steps-3.onnx"
    $tokens = Join-Path $MatchaDir "tokens.txt"
    return ((Test-Path $acoustic) -and (Test-Path $tokens) -and (Test-Path $VocoderPath))
}

Write-Host "Mochi X-TTS model download" -ForegroundColor White
Ensure-Venv

if (Test-MatchaReady) {
    Write-Host "Models already present." -ForegroundColor Green
    exit 0
}

Download-Vocoder
if ($UseModelScope) {
    Download-Matcha-MS
} else {
    try {
        Download-Matcha-HF
    } catch {
        Write-Host "HF download failed, trying ModelScope ..." -ForegroundColor Yellow
        Download-Matcha-MS
    }
}

if (-not (Test-MatchaReady)) {
    throw "Matcha model incomplete after download. Check models/matcha-zh-en/"
}

Write-Host ""
Write-Host "OK | matcha: $MatchaDir" -ForegroundColor Green
Write-Host "OK | vocoder: $VocoderPath" -ForegroundColor Green
