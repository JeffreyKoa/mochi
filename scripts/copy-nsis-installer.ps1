# 将 Tauri NSIS 产物复制到 bundle/nsis/Mochi_x.x.x_x64-setup.exe
# Tauri 2 输出：target/release/nsis/x64/nsis-output.exe（非 bundle/nsis/）
$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $PSCommandPath
$RepoRoot = Split-Path -Parent $ScriptDir
$ReleaseRoot = Join-Path $RepoRoot "desktop\src-tauri\target\release"
$Src = Join-Path $ReleaseRoot "nsis\x64\nsis-output.exe"
$DstDir = Join-Path $ReleaseRoot "bundle\nsis"
$TauriConf = Join-Path $RepoRoot "desktop\src-tauri\tauri.conf.json"

function Get-AppVersion {
    if (-not (Test-Path $TauriConf)) { return "0.1.0" }
    try {
        $conf = Get-Content $TauriConf -Raw | ConvertFrom-Json
        if ($conf.version) { return [string]$conf.version }
    } catch {
        Write-Host "WARN: parse tauri.conf.json failed, use 0.1.0" -ForegroundColor Yellow
    }
    return "0.1.0"
}

function Wait-NsisOutputStable {
    param(
        [string]$Path,
        [int]$TimeoutSec = 600,
        [int]$PollSec = 3
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $lastLen = -1
    $stable = 0
    while ((Get-Date) -lt $deadline) {
        if (-not (Test-Path $Path)) {
            Start-Sleep -Seconds $PollSec
            continue
        }
        $len = (Get-Item $Path).Length
        if ($len -eq $lastLen -and $len -gt 0) {
            $stable++
            if ($stable -ge 2) { return $true }
        } else {
            $stable = 0
            $lastLen = $len
        }
        Start-Sleep -Seconds $PollSec
    }
    return $false
}

$Version = Get-AppVersion

if (-not (Test-Path $Src)) {
    Write-Host "NSIS output not found: $Src" -ForegroundColor Red
    Write-Host "Run: cd desktop; npm run tauri:build" -ForegroundColor Yellow
    exit 1
}

if (-not (Wait-NsisOutputStable -Path $Src)) {
    Write-Host "NSIS output still growing or missing: $Src" -ForegroundColor Red
    exit 1
}

$srcItem = Get-Item $Src
$srcMb = [math]::Round($srcItem.Length / 1048576.0, 1)
# NSIS 使用 LZMA，体积远小于 MSI（~180MB vs ~900MB）属正常；仅做 sanity check
$MinBytes = 20 * 1024 * 1024
if ($srcItem.Length -lt $MinBytes) {
    Write-Host "NSIS output too small (${srcMb} MB) — makensis may have failed." -ForegroundColor Red
    exit 1
}

New-Item -ItemType Directory -Force -Path $DstDir | Out-Null
$Dst = Join-Path $DstDir "Mochi_${Version}_x64-setup.exe"
Copy-Item -Force $Src $Dst

Write-Host "Copied installer:" -ForegroundColor Green
Write-Host "  $Dst"
Write-Host "  Size: $([math]::Round((Get-Item $Dst).Length / 1048576.0, 1)) MB (NSIS LZMA; MSI is larger but same payload)"

# 若存在 MSI，提示备用路径
$msi = Get-ChildItem -Path (Join-Path $ReleaseRoot "bundle\msi") -Filter "*.msi" -ErrorAction SilentlyContinue |
    Select-Object -First 1
if ($msi) {
    Write-Host "MSI also available: $($msi.FullName)" -ForegroundColor DarkGray
}
