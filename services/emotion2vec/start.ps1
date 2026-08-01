# Mochi emotion2vec Sidecar launcher (Windows PowerShell 5.1+)
# Usage: .\start.ps1   or double-click start.bat
# NOTE: keep this file ASCII-only for PS 5.1 on zh-CN Windows

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

$VenvDir = Join-Path $Root ".venv"
$Python = Join-Path $VenvDir "Scripts\python.exe"

Write-Host "== Mochi emotion2vec Sidecar ==" -ForegroundColor Cyan

function Test-PythonModule {
    param([string]$ModuleName)
    if (-not (Test-Path $Python)) { return $false }
    & $Python -c "import $ModuleName" 2>$null
    return ($LASTEXITCODE -eq 0)
}

function Test-NvidiaGpu {
    if (-not (Get-Command nvidia-smi -ErrorAction SilentlyContinue)) { return $false }
    & nvidia-smi 1>$null 2>$null
    return ($LASTEXITCODE -eq 0)
}

function Test-TorchCuda {
    if (-not (Test-Path $Python)) { return $false }
    $pyCode = "import torch; print(1 if torch.cuda.is_available() else 0)"
    $out = & $Python -c $pyCode 2>$null
    return ($out -eq "1")
}

function Resolve-EmotionDevice {
    if ($env:EMOTION2VEC_DEVICE) {
        if ($env:EMOTION2VEC_DEVICE -eq "cuda" -and -not (Test-TorchCuda)) {
            Write-Host "  EMOTION2VEC_DEVICE=cuda but PyTorch CUDA unavailable; using cpu." -ForegroundColor Yellow
            return "cpu"
        }
        return $env:EMOTION2VEC_DEVICE
    }
    if (Test-TorchCuda) { return "cuda" }
    return "cpu"
}

function Test-SidecarHealthy {
    param([string]$Port)
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/health" -UseBasicParsing -TimeoutSec 3
        return ($resp.StatusCode -eq 200)
    } catch {
        return $false
    }
}

function Get-PortOwnerPid {
    param([string]$Port)
    $lines = netstat -ano | Select-String "127.0.0.1:$Port\s"
    if (-not $lines) { return $null }
    $line = ($lines | Select-Object -First 1).ToString().Trim()
    $parts = $line -split '\s+'
    return [int]$parts[-1]
}

function Install-CudaTorchIfNeeded {
    if (-not (Test-NvidiaGpu)) { return }
    if (Test-TorchCuda) { return }

    $CudaMarker = Join-Path $VenvDir ".torch_cuda"
    if ((Test-Path $CudaMarker) -and ((Get-Content $CudaMarker -Raw).Trim() -eq "failed")) {
        Write-Host "  CUDA PyTorch install previously failed; using cpu." -ForegroundColor Yellow
        return
    }

    Write-Host "[2b] NVIDIA GPU found; installing CUDA PyTorch (cu121, ~2.4GB)..." -ForegroundColor Yellow
    & $Python -m pip install --force-reinstall torch torchaudio --index-url https://download.pytorch.org/whl/cu121
    if ($LASTEXITCODE -ne 0) {
        Set-Content -Path $CudaMarker -Value "failed" -NoNewline -Encoding ASCII
        Write-Host "  CUDA PyTorch install failed; using cpu." -ForegroundColor Yellow
        return
    }
    if (Test-TorchCuda) {
        Set-Content -Path $CudaMarker -Value "cu121" -NoNewline -Encoding ASCII
        Write-Host "[2b] CUDA PyTorch ready" -ForegroundColor Green
    } else {
        Set-Content -Path $CudaMarker -Value "failed" -NoNewline -Encoding ASCII
        Write-Host "  CUDA PyTorch installed but torch.cuda.is_available() is false; using cpu." -ForegroundColor Yellow
    }
}

# Step 1: create venv
if (-not (Test-Path $Python)) {
    Write-Host "[1/4] Creating virtual environment..." -ForegroundColor Yellow
    $py = Get-Command python -ErrorAction SilentlyContinue
    if (-not $py) {
        Write-Error "Python not found. Install Python 3.10+ and add it to PATH."
    }
    & python -m venv $VenvDir
    if ($LASTEXITCODE -ne 0) {
        Write-Error "python -m venv failed."
    }
} else {
    Write-Host "[1/4] Virtual environment OK" -ForegroundColor Green
}

# Step 2: install deps (always verify uvicorn is importable)
$Marker = Join-Path $VenvDir ".deps_installed"
$ReqHash = (Get-FileHash (Join-Path $Root "requirements.txt") -Algorithm MD5).Hash
$SavedHash = ""
if (Test-Path $Marker) {
    $SavedHash = (Get-Content $Marker -Raw).Trim()
}

$depsOk = ($ReqHash -eq $SavedHash) -and (Test-PythonModule "uvicorn") -and (Test-PythonModule "fastapi")
if (-not $depsOk) {
    Write-Host "[2/4] Installing dependencies (first run may take several minutes)..." -ForegroundColor Yellow
    & $Python -m pip install -U pip wheel
    if ($LASTEXITCODE -ne 0) { Write-Error "pip upgrade failed." }
    & $Python -m pip install -r (Join-Path $Root "requirements.txt")
    if ($LASTEXITCODE -ne 0) { Write-Error "pip install -r requirements.txt failed." }
    if (-not (Test-PythonModule "uvicorn")) {
        Write-Error "uvicorn still not installed after pip install."
    }
    Set-Content -Path $Marker -Value $ReqHash -NoNewline -Encoding ASCII
    Write-Host "[2/4] Dependencies installed" -ForegroundColor Green
} else {
    Write-Host "[2/4] Dependencies OK" -ForegroundColor Green
}

Install-CudaTorchIfNeeded

# Step 3: env vars (auto-detect cuda/cpu)
Write-Host "[3/4] Environment..." -ForegroundColor Yellow
if (-not $env:EMOTION2VEC_MODEL) { $env:EMOTION2VEC_MODEL = "iic/emotion2vec_plus_base" }
if (-not $env:EMOTION2VEC_HUB) { $env:EMOTION2VEC_HUB = "ms" }
if (-not $env:EMOTION2VEC_PORT) { $env:EMOTION2VEC_PORT = "8091" }
$env:EMOTION2VEC_DEVICE = Resolve-EmotionDevice

Write-Host "  Model : $env:EMOTION2VEC_MODEL"
Write-Host "  Device: $env:EMOTION2VEC_DEVICE"
Write-Host "  Port  : $env:EMOTION2VEC_PORT"

# Step 4: start server (skip if already running)
if (Test-SidecarHealthy $env:EMOTION2VEC_PORT) {
    Write-Host "[4/4] Sidecar already running on http://127.0.0.1:$env:EMOTION2VEC_PORT" -ForegroundColor Green
    Write-Host "  Health: OK. No need to start again." -ForegroundColor DarkGray
    exit 0
}

$portPid = Get-PortOwnerPid $env:EMOTION2VEC_PORT
if ($portPid) {
    Write-Host "[ERROR] Port $env:EMOTION2VEC_PORT is already in use by PID $portPid." -ForegroundColor Red
    Write-Host "  Stop it: taskkill /PID $portPid /F" -ForegroundColor DarkGray
    Write-Host "  Or use another port: set EMOTION2VEC_PORT=8092" -ForegroundColor DarkGray
    exit 1
}

Write-Host "[4/4] Starting uvicorn on http://127.0.0.1:$env:EMOTION2VEC_PORT ..." -ForegroundColor Green
Write-Host "First start downloads model weights. Press Ctrl+C to stop." -ForegroundColor DarkGray

& $Python -m uvicorn app:app --host 127.0.0.1 --port $env:EMOTION2VEC_PORT
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] uvicorn exited with code $LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
}
