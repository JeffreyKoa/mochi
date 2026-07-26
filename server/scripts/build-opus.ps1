# Build Mochi server with real libopus (CGO). Requires MSYS2 + mingw-w64 packages:
#   pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-opus mingw-w64-x86_64-opusfile mingw-w64-x86_64-pkg-config
$ErrorActionPreference = "Stop"
$MsysBin = "C:\msys64\mingw64\bin"
$MsysUsr = "C:\msys64\usr\bin"
$PkgConfig = "C:\msys64\mingw64\lib\pkgconfig"

if (-not (Test-Path "$MsysBin\gcc.exe")) {
    Write-Error "gcc not found at $MsysBin. Install MSYS2 and mingw-w64-x86_64-gcc."
}

$env:PATH = "$MsysBin;$MsysUsr;" + $env:PATH
$env:CGO_ENABLED = "1"
$env:CC = "gcc"
$env:PKG_CONFIG_PATH = $PkgConfig

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

New-Item -ItemType Directory -Force -Path "bin" | Out-Null
go build -o bin/server.exe ./cmd/server
Write-Host "Built bin/server.exe (CGO_ENABLED=1, libopus via MSYS2)"
