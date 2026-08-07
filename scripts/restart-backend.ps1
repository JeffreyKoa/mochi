# Mochi backend one-shot restart: kill ALL backend processes, then start ALL services.
#
# Services:
#   emotion2vec  :8091  acoustic SER
#   x-asr sidecar :8766  server ASR
#   Go API server :8081  main backend
#
# Usage:
#   .\scripts\restart-backend.ps1
#   .\scripts\restart-backend.ps1 -KillOnly
#   .\scripts\restart-backend.ps1 -BuildOpus
#   .\scripts\restart-backend.ps1 -SkipEmotion2vec
#   .\scripts\restart-backend.ps1 -SkipXasr
#
# Logs: logs/backend/

param(
    [switch]$KillOnly,
    [switch]$BuildOpus,
    [switch]$SkipEmotion2vec,
    [switch]$SkipXasr,
    [int]$ServerPort = 8081,
    [int]$XasrPort = 8766,
    [int]$EmotionPort = 8091,
    [int]$HealthTimeoutSec = 180
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..")
$LogDir = Join-Path $RepoRoot "logs\backend"
$BackendPidFile = Join-Path $LogDir "pids.json"

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

function Stop-PortListener {
    param(
        [int]$Port,
        [string]$Label
    )
    for ($attempt = 0; $attempt -lt 3; $attempt++) {
        $conns = @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
        if ($conns.Count -eq 0) { return }
        foreach ($c in $conns) {
            $procId = $c.OwningProcess
            if ($procId -le 0) { continue }
            Write-Host "  stop $Label : port $Port PID $procId (attempt $($attempt + 1))" -ForegroundColor Yellow
            Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
            cmd /c "taskkill /PID $procId /T /F >nul 2>&1"
        }
        Start-Sleep -Milliseconds 600
    }
}

function Stop-ProcessByCommandLine {
    param(
        [string]$ProcessName,
        [string[]]$Patterns,
        [string]$Label
    )
    Get-CimInstance Win32_Process -Filter "Name='$ProcessName'" -ErrorAction SilentlyContinue |
        Where-Object {
            $cmd = $_.CommandLine
            if (-not $cmd) { return $false }
            foreach ($p in $Patterns) {
                if ($cmd -like "*$p*") { return $true }
            }
            return $false
        } |
        ForEach-Object {
            Write-Host "  stop $Label : PID $($_.ProcessId)" -ForegroundColor Yellow
            Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
            cmd /c "taskkill /PID $($_.ProcessId) /T /F >nul 2>&1"
        }
}

function Test-PortListening {
    param([int]$Port)
    $c = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    return ($null -ne $c)
}

function Wait-PortListening {
    param(
        [int]$Port,
        [string]$Label,
        [int]$TimeoutSec = 60
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-PortListening $Port) {
            Write-Host "  OK  $Label listening on $Port" -ForegroundColor Green
            return $true
        }
        Start-Sleep -Milliseconds 500
    }
    Write-Host "  FAIL $Label not listening on $Port after $TimeoutSec sec" -ForegroundColor Red
    return $false
}

function Wait-HttpOk {
    param(
        [string]$Url,
        [string]$Label,
        [int]$TimeoutSec = 60
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $r = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3
            if ($r.StatusCode -eq 200) {
                Write-Host "  OK  $Label $Url" -ForegroundColor Green
                return $true
            }
        } catch {
            # retry
        }
        Start-Sleep -Milliseconds 800
    }
    Write-Host "  FAIL $Label $Url after $TimeoutSec sec" -ForegroundColor Red
    return $false
}

function Stop-AllBackend {
    Write-Step "Kill all Mochi backend processes"

    Stop-PortListener -Port $ServerPort -Label "Go API"
    Stop-PortListener -Port $XasrPort -Label "x-asr"
    Stop-PortListener -Port $EmotionPort -Label "emotion2vec"

    cmd /c "taskkill /IM server.exe /F >nul 2>&1"

    Stop-ProcessByCommandLine -ProcessName "go.exe" -Patterns @(
        "cmd\server",
        "cmd/server"
    ) -Label "go run server"

    Stop-ProcessByCommandLine -ProcessName "python.exe" -Patterns @(
        "sherpa_streaming_server.py",
        "uvicorn app:app",
        "services\emotion2vec",
        "services/emotion2vec"
    ) -Label "python sidecar"

    Start-Sleep -Seconds 2

    foreach ($p in @($ServerPort, $XasrPort, $EmotionPort)) {
        if (Test-PortListening $p) {
            Write-Host "  WARN port $p still in use, force kill again" -ForegroundColor Yellow
            Stop-PortListener -Port $p -Label "port-$p"
        }
    }

    Start-Sleep -Milliseconds 800

    if (Test-Path $BackendPidFile) {
        Remove-Item $BackendPidFile -Force -ErrorAction SilentlyContinue
    }

    Write-Host "All backend processes stopped." -ForegroundColor Green
}

function Start-Emotion2vecService {
    Write-Step "Start emotion2vec (:$EmotionPort)"
    $startScript = Join-Path $RepoRoot "services\emotion2vec\start.ps1"
    if (-not (Test-Path $startScript)) {
        throw "Missing $startScript"
    }

    & $startScript -SetupOnly
    if ($LASTEXITCODE -ne 0) { throw "emotion2vec setup failed" }

    & $startScript -Background -LogDir $LogDir
    if ($LASTEXITCODE -ne 0) { throw "emotion2vec background start failed" }

    if (-not (Wait-HttpOk "http://127.0.0.1:$EmotionPort/health" "emotion2vec" $HealthTimeoutSec)) {
        throw "emotion2vec health check failed. See $LogDir\emotion2vec-err.log"
    }
}

function Start-XAsrService {
    Write-Step "Start x-asr sidecar (:$XasrPort)"
    $setupScript = Join-Path $RepoRoot "server\scripts\start-xasr-sidecar.ps1"
    $bgScript = Join-Path $RepoRoot "server\scripts\start-xasr-sidecar-background.ps1"

    & $setupScript -SetupOnly -Port $XasrPort
    if ($LASTEXITCODE -ne 0) { throw "x-asr setup failed" }

    $xasrProcId = & $bgScript -Port $XasrPort -LogDir $LogDir
    if (-not $xasrProcId) { throw "x-asr background start failed" }
    Write-Host "  x-asr PID $xasrProcId" -ForegroundColor Green

    if (-not (Wait-PortListening $XasrPort "x-asr" 60)) {
        throw "x-asr port not open. See $LogDir\xasr-launcher-err.log"
    }

    $probeScript = Join-Path $RepoRoot "server\scripts\probe-xasr.ps1"
    if (Test-Path $probeScript) {
        & $probeScript -WsUrl "ws://127.0.0.1:$XasrPort"
        if ($LASTEXITCODE -ne 0) {
            throw "x-asr probe failed"
        }
    }
}

function Start-GoServer {
    Write-Step "Start Go API server (:$ServerPort)"
    $serverDir = Join-Path $RepoRoot "server"
    $outLog = Join-Path $LogDir "go-server-out.log"
    $errLog = Join-Path $LogDir "go-server-err.log"

    if ($BuildOpus) {
        $buildBat = Join-Path $serverDir "build-opus.bat"
        if (-not (Test-Path $buildBat)) { throw "Missing $buildBat" }
        Write-Host "  building Opus server.exe ..."
        cmd /c "`"$buildBat`""
        if ($LASTEXITCODE -ne 0) { throw "build-opus.bat failed" }
        $exe = Join-Path $serverDir "bin\server.exe"
        if (-not (Test-Path $exe)) { throw "Missing $exe after build" }
        $proc = Start-Process -FilePath $exe `
            -WorkingDirectory $serverDir `
            -WindowStyle Hidden `
            -RedirectStandardOutput $outLog `
            -RedirectStandardError $errLog `
            -PassThru
    } else {
        $go = Get-Command go -ErrorAction SilentlyContinue
        if (-not $go) { throw "go not found in PATH" }
        $proc = Start-Process -FilePath $go.Source `
            -ArgumentList @("run", "./cmd/server") `
            -WorkingDirectory $serverDir `
            -WindowStyle Hidden `
            -RedirectStandardOutput $outLog `
            -RedirectStandardError $errLog `
            -PassThru
    }

    Write-Host "  Go server PID $($proc.Id)" -ForegroundColor Green

    if (-not (Wait-PortListening $ServerPort "Go API" 90)) {
        throw "Go server not listening. See $errLog"
    }

    if (-not (Wait-HttpOk "http://127.0.0.1:$ServerPort/api/v1/public/config" "Go API" 30)) {
        throw "Go API health check failed. See $errLog"
    }

    return $proc.Id
}

Write-Host "Mochi backend restart" -ForegroundColor White
Write-Host "  repo:    $RepoRoot"
Write-Host "  logs:    $LogDir"
Write-Host "  ports:   emotion2vec=$EmotionPort x-asr=$XasrPort go=$ServerPort"

Stop-AllBackend

if ($KillOnly) {
    Write-Host ""
    Write-Host "KillOnly: done." -ForegroundColor Green
    exit 0
}

$pids = @{}

try {
    if (-not $SkipEmotion2vec) {
        Start-Emotion2vecService
        $pids.emotion2vec_port = $EmotionPort
    } else {
        Write-Step "Skip emotion2vec"
    }

    if (-not $SkipXasr) {
        Start-XAsrService
        $pids.xasr_port = $XasrPort
    } else {
        Write-Step "Skip x-asr"
    }

    $goProcId = Start-GoServer
    $pids.go_server = $goProcId
    $pids.server_port = $ServerPort
    $pids.restarted_at = (Get-Date).ToString("o")

    $pids | ConvertTo-Json | Set-Content -Path $BackendPidFile -Encoding UTF8

    Write-Step "All backend services started"
    Write-Host "  emotion2vec : http://127.0.0.1:$EmotionPort/health" -ForegroundColor Green
    Write-Host "  x-asr       : ws://127.0.0.1:$XasrPort" -ForegroundColor Green
    Write-Host "  Go API      : http://127.0.0.1:$ServerPort" -ForegroundColor Green
    Write-Host "  Logs        : $LogDir" -ForegroundColor Green
    Write-Host "  PIDs        : $BackendPidFile" -ForegroundColor Green

    exit 0
} catch {
    Write-Host ""
    Write-Host "FAILED: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Check logs under $LogDir" -ForegroundColor Yellow
    exit 1
}
