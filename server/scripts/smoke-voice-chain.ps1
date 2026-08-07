# 云端语音链路联调：x-asr sidecar + Go server + 客户端 PCM 路径
# 用法:
#   .\server\scripts\smoke-voice-chain.ps1
#   .\server\scripts\smoke-voice-chain.ps1 -SkipSidecarStart

param(
    [switch]$SkipSidecarStart,
    [string]$WsUrl = "ws://127.0.0.1:8766",
    [string]$ApiBase = "http://localhost:8081"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..\..")

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

Write-Host "Mochi voice chain smoke test" -ForegroundColor White
Write-Host "  ws:   $WsUrl"
Write-Host "  api:  $ApiBase"

if (-not $SkipSidecarStart) {
    Write-Step "Check x-asr sidecar (start if port closed)"
    $uri = [Uri]$WsUrl
    $port = if ($uri.Port -gt 0) { $uri.Port } else { 8766 }
    $tcp = New-Object System.Net.Sockets.TcpClient
    $open = $false
    try {
        $iar = $tcp.BeginConnect($uri.Host, $port, $null, $null)
        $open = $iar.AsyncWaitHandle.WaitOne(1500, $false)
        if ($open) { $tcp.EndConnect($iar) }
    } catch { $open = $false }
    finally { $tcp.Close() }

    if (-not $open) {
        Write-Host "Sidecar not running. Start in a separate terminal:" -ForegroundColor Yellow
        Write-Host "  .\server\scripts\start-xasr-sidecar.ps1" -ForegroundColor Yellow
        Write-Host "Then re-run this script with -SkipSidecarStart" -ForegroundColor Yellow
        exit 2
    }
}

Write-Step "Probe x-asr (Python + Go adapter)"
& (Join-Path $ScriptDir "probe-xasr.ps1") -WsUrl $WsUrl -GoProbe
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Step "Check Go API health"
try {
    $res = Invoke-WebRequest -Uri "$ApiBase/api/v1/public/config" -UseBasicParsing -TimeoutSec 5
    if ($res.StatusCode -ne 200) {
        throw "HTTP $($res.StatusCode)"
    }
    $cfg = $res.Content | ConvertFrom-Json
    $rt = $cfg.realtime
    if (-not $rt) { throw "missing realtime block in public config" }
    Write-Host "OK  public config | stt_mode=$($rt.stt_mode) tts_mode=$($rt.tts_mode) voiceprint.required=$($rt.voiceprint.required)"
    if ($rt.stt_mode -ne "cloud") {
        Write-Host "WARN: expected stt_mode=cloud (got $($rt.stt_mode)). Restart Go server after config change." -ForegroundColor Yellow
    }
    if ($rt.tts_mode -ne "cloud") {
        Write-Host "WARN: expected tts_mode=cloud (got $($rt.tts_mode))" -ForegroundColor Yellow
    }
    if ($rt.voiceprint.required -eq $true) {
        Write-Host "WARN: Phase2 expects voiceprint.required=false" -ForegroundColor Yellow
    }
} catch {
    Write-Host "FAIL: Go server not reachable at $ApiBase" -ForegroundColor Red
    Write-Host "  $_" -ForegroundColor Red
    Write-Host "Start server from repo root: cd server; go run ./cmd/server" -ForegroundColor Yellow
    exit 1
}

Write-Step "Checklist (manual)"
Write-Host @"
  [ ] Desktop startTalk -> status 正在听... (not 休息中)
  [ ] Speak -> server log: [realtime] asr.provider=xasr
  [ ] server log: session audio_bytes > 0
  [ ] Client receives tts_audio / reply
  [ ] x-asr.log at `$env:MOCHI_XASR_LOG_DIR or server/logs/x-asr
  [ ] emotion2vec: MergeAcousticHint when crying (optional)
"@

Write-Host ""
Write-Host "Smoke checks passed (automated portion)." -ForegroundColor Green
