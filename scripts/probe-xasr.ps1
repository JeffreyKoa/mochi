# X-ASR sidecar 健康探测（connect + ping + start/end 空会话）
# 用法:
#   .\scripts\probe-xasr.ps1
#   .\scripts\probe-xasr.ps1 -WsUrl ws://127.0.0.1:8766
#   .\scripts\probe-xasr.ps1 -GoProbe   # 同时跑 go run ./cmd/xasrprobe

param(
    [string]$WsUrl = "ws://127.0.0.1:8766",
    [switch]$GoProbe
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..")

function Test-TcpPort([string]$HostName, [int]$Port) {
    try {
        $client = New-Object System.Net.Sockets.TcpClient
        $iar = $client.BeginConnect($HostName, $Port, $null, $null)
        $ok = $iar.AsyncWaitHandle.WaitOne(2000, $false)
        if ($ok) { $client.EndConnect($iar) }
        $client.Close()
        return $ok
    } catch {
        return $false
    }
}

$uri = [Uri]$WsUrl
$hostName = $uri.Host
$port = $uri.Port
if ($port -le 0) { $port = 8766 }

Write-Host "==> TCP $hostName`:$port" -ForegroundColor Cyan
if (-not (Test-TcpPort $hostName $port)) {
    Write-Host "FAIL: port not open. Start sidecar: .\scripts\start-xasr-sidecar.ps1" -ForegroundColor Red
    exit 1
}
Write-Host "OK  port open" -ForegroundColor Green

$pyProbe = Join-Path $RepoRoot "tools\x-asr\test_ws_probe.py"
$venvPy = Join-Path $RepoRoot "tools\x-asr\.venv\Scripts\python.exe"
$python = if (Test-Path $venvPy) { $venvPy } else { "python" }

Write-Host "==> WebSocket probe (test_ws_probe.py)" -ForegroundColor Cyan
& $python $pyProbe
if ($LASTEXITCODE -ne 0) {
    Write-Host "FAIL: websocket probe" -ForegroundColor Red
    exit 1
}
Write-Host "OK  websocket probe" -ForegroundColor Green

if ($GoProbe) {
    Write-Host "==> Go ASR adapter probe (xasrprobe)" -ForegroundColor Cyan
    Push-Location (Join-Path $RepoRoot "server")
    try {
        go run ./cmd/xasrprobe -ws $WsUrl
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
        Pop-Location
    }
    Write-Host "OK  go xasrprobe" -ForegroundColor Green
}

Write-Host ""
Write-Host "All probes passed for $WsUrl" -ForegroundColor Green
