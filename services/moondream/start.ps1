# Mochi Moondream Sidecar launcher (Windows PowerShell 5.1+)
# Usage: .\start.ps1 -SetupOnly | .\start.ps1 -Background -RepoRoot D:\ocr\Mochi

param(
    [switch]$SetupOnly,
    [switch]$Background,
    [string]$RepoRoot = "",
    [int]$Port = 0
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# 兼容 & script.ps1 调用（无 -File 时 MyCommand.Path / PSCommandPath 可能为空）
$Root = $null
if ($RepoRoot -ne "") {
    $Root = Join-Path $RepoRoot "services\moondream"
}
if (-not $Root -and $PSCommandPath) {
    $Root = Split-Path -Parent $PSCommandPath
}
if (-not $Root -and $MyInvocation.MyCommand.Path) {
    $Root = Split-Path -Parent $MyInvocation.MyCommand.Path
}
if (-not $Root) { throw "Cannot resolve moondream script directory (pass -RepoRoot)" }
Set-Location $Root

Write-Host "== Mochi Moondream Sidecar ==" -ForegroundColor Cyan

function Test-PythonModule {
    param([string]$ModuleName)
    if (-not (Test-Path $Python)) { return $false }
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    & $Python -c "import $ModuleName" 2>&1 | Out-Null
    $ok = ($LASTEXITCODE -eq 0)
    $ErrorActionPreference = $prevEap
    return $ok
}

function Test-SidecarHealthy {
    param([string]$Port)
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/health" -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -ne 200) { return $false }
        # New /health includes model_loaded; old sidecar may report ok before model is ready
        $body = $resp.Content | ConvertFrom-Json
        if ($null -ne $body.PSObject.Properties['model_loaded']) {
            return [bool]$body.model_loaded
        }
        return $true
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

function Stop-PortOwner {
    param([string]$Port)
    $ownerPid = Get-PortOwnerPid $Port
    if (-not $ownerPid) { return $false }
    Write-Host "  kill stale listener on port $Port PID $ownerPid" -ForegroundColor Yellow
    Stop-Process -Id $ownerPid -Force -ErrorAction SilentlyContinue
    cmd /c "taskkill /PID $ownerPid /T /F >nul 2>&1"
    Start-Sleep -Milliseconds 800
    return ($null -eq (Get-PortOwnerPid $Port))
}

function Test-NvidiaGpu {
    # Quick check without importing torch (works before venv deps exist)
    $nvidia = Get-Command nvidia-smi -ErrorAction SilentlyContinue
    if (-not $nvidia) { return $false }
    & nvidia-smi --query-gpu=name --format=csv,noheader 2>$null | Out-Null
    return ($LASTEXITCODE -eq 0)
}

function Test-VenvTorchCuda {
    param([string]$PyExe)
    if (-not (Test-Path $PyExe)) { return $false }
    $out = & $PyExe -c "import importlib.util; spec=importlib.util.find_spec('torch'); exec('import torch\nprint(1 if torch.cuda.is_available() else 0)') if spec else print(0)" 2>$null
    return ($LASTEXITCODE -eq 0 -and ($out | Select-Object -Last 1).ToString().Trim() -eq "1")
}

function Get-VenvTorchTag {
    param([string]$PyExe)
    if (-not (Test-Path $PyExe)) { return "" }
    $out = & $PyExe -c "import torch; print(getattr(torch.version, 'cuda', None) or 'cpu')" 2>$null
    if ($LASTEXITCODE -ne 0) { return "" }
    return ($out -join "").Trim()
}

function Ensure-TorchBackend {
    param([string]$PyExe)
    if (-not (Test-NvidiaGpu)) {
        Write-Host "  GPU   : not detected (using CPU torch)" -ForegroundColor DarkGray
        return
    }
    $gpuName = (& nvidia-smi --query-gpu=name --format=csv,noheader 2>$null | Select-Object -First 1)
    if ($gpuName) {
        Write-Host "  GPU   : $gpuName" -ForegroundColor Green
    }
    if (Test-VenvTorchCuda $PyExe) {
        Write-Host "  Torch : CUDA ready ($((Get-VenvTorchTag $PyExe)))" -ForegroundColor Green
        return
    }
    Write-Host "  Torch : CPU build detected, installing CUDA wheels (cu124)..." -ForegroundColor Yellow
    & $PyExe -m pip install -U pip wheel
    # Free CPU torch first — force-reinstall otherwise needs 2x disk during swap
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    & $PyExe -m pip uninstall -y torch torchvision torchaudio 2>&1 | Out-Null
    $ErrorActionPreference = $prevEap
    $cudaTorchVer = "2.5.1+cu124"
    $cudaVisionVer = "0.20.1+cu124"
    # Stream install without keeping a 2.5GB wheel copy (saves disk on tight drives)
    & $PyExe -m pip install --force-reinstall --no-cache-dir "torch==$cudaTorchVer" "torchvision==$cudaVisionVer" `
        --index-url https://download.pytorch.org/whl/cu124
    if ($LASTEXITCODE -ne 0) { Write-Error "CUDA torch install failed." }
    if (-not (Test-VenvTorchCuda $PyExe)) {
        Write-Error "CUDA torch installed but torch.cuda.is_available() is still false."
    }
    Write-Host "  Torch : CUDA ready ($((Get-VenvTorchTag $PyExe)))" -ForegroundColor Green
}

function Resolve-MoondreamDevice {
    param([string]$PyExe)
    if ($env:MOONDREAM_DEVICE) { return $env:MOONDREAM_DEVICE }
    # Auto CUDA when NVIDIA GPU + CUDA torch are available in venv
    if (Test-VenvTorchCuda $PyExe) { return "cuda" }
    return "cpu"
}

function Resolve-MoondreamVenv {
    # Prefer legacy project venv when it has enough free disk; else LocalAppData on C:
    $defaultVenv = Join-Path $env:LOCALAPPDATA "Mochi\moondream-venv"
    $legacyVenv = Join-Path $Root ".venv"
    if ($env:MOONDREAM_VENV) { return $env:MOONDREAM_VENV }
    if (Test-Path $legacyVenv) { return $legacyVenv }
    return $defaultVenv
}

$VenvDir = Resolve-MoondreamVenv
$Python = Join-Path $VenvDir "Scripts\python.exe"

if (-not (Test-Path $Python)) {
    Write-Host "[1/3] Creating virtual environment..." -ForegroundColor Yellow
    $py = Get-Command python -ErrorAction SilentlyContinue
    if (-not $py) { Write-Error "Python not found." }
    & python -m venv $VenvDir
} else {
    Write-Host "[1/3] Virtual environment OK" -ForegroundColor Green
}

$Marker = Join-Path $VenvDir ".deps_installed"
$ReqHash = (Get-FileHash (Join-Path $Root "requirements.txt") -Algorithm MD5).Hash
$SavedHash = ""
if (Test-Path $Marker) { $SavedHash = (Get-Content $Marker -Raw).Trim() }

$depsOk = ($ReqHash -eq $SavedHash) -and (Test-PythonModule "uvicorn") -and (Test-PythonModule "transformers")
if (-not $depsOk) {
    Write-Host "[2/3] Installing dependencies (first run may take several minutes)..." -ForegroundColor Yellow
    & $Python -m pip install -U pip wheel
    & $Python -m pip install -r (Join-Path $Root "requirements.txt")
    if ($LASTEXITCODE -ne 0) { Write-Error "pip install failed." }
    Set-Content -Path $Marker -Value $ReqHash -NoNewline -Encoding ASCII
    Write-Host "[2/3] Dependencies installed" -ForegroundColor Green
} else {
    Write-Host "[2/3] Dependencies OK" -ForegroundColor Green
}

# Upgrade CPU torch to CUDA build when a local NVIDIA GPU is present
Ensure-TorchBackend -PyExe $Python

Write-Host "[3/3] Environment..." -ForegroundColor Yellow
if ($Port -gt 0) { $env:MOONDREAM_PORT = "$Port" }
elseif (-not $env:MOONDREAM_PORT) { $env:MOONDREAM_PORT = "8093" }
if (-not $env:MOONDREAM_MODEL) { $env:MOONDREAM_MODEL = "moondream2" }
if (-not $env:MOONDREAM_LOCAL_ONLY) { $env:MOONDREAM_LOCAL_ONLY = "1" }
# Optional for download-model.ps1: HF_ENDPOINT=https://hf-mirror.com
if (-not $env:HF_ENDPOINT) { $env:HF_ENDPOINT = "https://hf-mirror.com" }
if (-not $env:PYTORCH_CUDA_ALLOC_CONF) { $env:PYTORCH_CUDA_ALLOC_CONF = "expandable_segments:True" }
$env:MOONDREAM_DEVICE = Resolve-MoondreamDevice -PyExe $Python
Write-Host "  Model : $env:MOONDREAM_MODEL"
Write-Host "  Device: $env:MOONDREAM_DEVICE"
Write-Host "  Port  : $env:MOONDREAM_PORT"

# 启动前检测可用内存，避免 safetensors mmap 失败却误报「权重缺失」
$memCheckPy = Join-Path $Root "weights_util.py"
if (-not [string]::IsNullOrWhiteSpace($memCheckPy) -and (Test-Path -LiteralPath $memCheckPy)) {
    $env:MOONDREAM_ROOT = $Root
    $memWarn = & $Python -c "import os; os.chdir(os.environ['MOONDREAM_ROOT']); from weights_util import check_virtual_memory; print(check_virtual_memory() or '')" 2>$null
    Remove-Item Env:MOONDREAM_ROOT -ErrorAction SilentlyContinue
    if ($LASTEXITCODE -eq 0 -and $memWarn) {
        Write-Host "  WARN  : $memWarn" -ForegroundColor Yellow
    }
}

if ($SetupOnly) {
    Write-Host "SetupOnly: skip server start." -ForegroundColor Yellow
    exit 0
}

if (Test-SidecarHealthy $env:MOONDREAM_PORT) {
    Write-Host "Sidecar already running on http://127.0.0.1:$env:MOONDREAM_PORT" -ForegroundColor Green
    exit 0
}

$portPid = Get-PortOwnerPid $env:MOONDREAM_PORT
if ($portPid) {
    # Port occupied but sidecar not healthy: try kill stale process once
    if (-not (Stop-PortOwner $env:MOONDREAM_PORT)) {
        $portPid = Get-PortOwnerPid $env:MOONDREAM_PORT
        if ($portPid) {
            Write-Host "[ERROR] Port $env:MOONDREAM_PORT in use by PID $portPid" -ForegroundColor Red
            Write-Host "  Run scripts\restart-backend.ps1 -KillOnly as admin, or reboot." -ForegroundColor Yellow
            exit 1
        }
    }
}

Write-Host "Starting uvicorn on http://127.0.0.1:$env:MOONDREAM_PORT ..." -ForegroundColor Green

if ($Background) {
    if ($RepoRoot -eq "") {
        $RepoRoot = (Resolve-Path (Join-Path $Root "..\..")).Path
    }
    . (Join-Path $RepoRoot "scripts\lib\daily-log.ps1")
    $pyArgs = @("-m", "uvicorn", "app:app", "--host", "127.0.0.1", "--port", $env:MOONDREAM_PORT)
    $proc = Start-MochiSidecarProcess -FilePath $Python -ArgumentList $pyArgs -WorkingDirectory $Root
    Write-Host "  PID $($proc.Id)" -ForegroundColor Green
    exit 0
}

& $Python -m uvicorn app:app --host 127.0.0.1 --port $env:MOONDREAM_PORT
