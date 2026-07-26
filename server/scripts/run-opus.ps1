# Run CGO-built server; mingw64\bin must be on PATH for libopus-0.dll at runtime.
$ErrorActionPreference = "Stop"
$MsysBin = "C:\msys64\mingw64\bin"
$MsysUsr = "C:\msys64\usr\bin"
$env:PATH = "$MsysBin;$MsysUsr;" + $env:PATH

$root = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $root "bin\server.exe"
if (-not (Test-Path $exe)) {
    Write-Error "Missing $exe — run scripts/build-opus.ps1 first."
}
Set-Location $root
& $exe
