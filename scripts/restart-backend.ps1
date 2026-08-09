# Mochi backend one-shot restart: kill ALL backend processes, then start ALL services.
#
# Services:
#   emotion2vec  :8091  acoustic SER
#   x-asr sidecar :8766  server ASR
#   x-tts sidecar :8767  server TTS (Matcha)
#   Go API server :8081  main backend
#
# Usage:
#   .\scripts\restart-backend.ps1
#   .\scripts\restart-backend.ps1 -KillOnly
#   .\scripts\restart-backend.ps1 -BuildOpus
#   .\scripts\restart-backend.ps1 -SkipEmotion2vec
#   .\scripts\restart-backend.ps1 -SkipXasr
#   .\scripts\restart-backend.ps1 -SkipXtts
#   .\scripts\restart-backend.ps1 -FollowLogs    # Go foreground: console + logs/mochi/mochi-YYYYMMDD.log
#   .\scripts\restart-backend.ps1 -NoFollowLogs # Start all and exit (CI/scripts)
#
# Logs: logs/mochi/mochi-YYYYMMDD.log (Go + sidecar API calls)

param(
    [switch]$KillOnly,
    [switch]$BuildOpus,
    [switch]$SkipEmotion2vec,
    [switch]$SkipXasr,
    [switch]$SkipXtts,
    [switch]$FollowLogs,
    [switch]$NoFollowLogs,
    [int]$ServerPort = 8081,
    [int]$XasrPort = 8766,
    [int]$XttsPort = 8767,
    [int]$EmotionPort = 8091,
    [int]$HealthTimeoutSec = 180
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..")
$LogsRoot = Join-Path $RepoRoot "logs"

New-Item -ItemType Directory -Force -Path $LogsRoot | Out-Null
. (Join-Path $RepoRoot "scripts\lib\daily-log.ps1")

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
    Stop-PortListener -Port $XttsPort -Label "x-tts"
    Stop-PortListener -Port $EmotionPort -Label "emotion2vec"

    cmd /c "taskkill /IM server.exe /F >nul 2>&1"

    Stop-ProcessByCommandLine -ProcessName "go.exe" -Patterns @(
        "cmd\server",
        "cmd/server"
    ) -Label "go run server"

    Stop-ProcessByCommandLine -ProcessName "python.exe" -Patterns @(
        "sherpa_streaming_server.py",
        "tts_server.py",
        "uvicorn app:app",
        "services\emotion2vec",
        "services/emotion2vec"
    ) -Label "python sidecar"

    Start-Sleep -Seconds 2

    foreach ($p in @($ServerPort, $XasrPort, $XttsPort, $EmotionPort)) {
        if (Test-PortListening $p) {
            Write-Host "  WARN port $p still in use, force kill again" -ForegroundColor Yellow
            Stop-PortListener -Port $p -Label "port-$p"
        }
    }

    Start-Sleep -Milliseconds 800

    Write-Host "All backend processes stopped." -ForegroundColor Green
}

function Start-Emotion2vecService {
    Write-Step "Start emotion2vec (:$EmotionPort)"
    $startScript = Join-Path $RepoRoot "services\emotion2vec\start.ps1"
    if (-not (Test-Path $startScript)) {
        throw "Missing $startScript"
    }

    if ($LASTEXITCODE -ne 0) { throw "emotion2vec background start failed" }

    $mochiLog = Get-MochiDailyLogPath -RepoRoot $RepoRoot -ServiceName "mochi"
    if (-not (Wait-HttpOk "http://127.0.0.1:$EmotionPort/health" "emotion2vec" $HealthTimeoutSec)) {
        throw "emotion2vec health check failed. Sidecar API calls are logged in $mochiLog"
    }
}

function Start-XAsrService {
    Write-Step "Start x-asr sidecar (:$XasrPort)"
    $bgScript = Join-Path $RepoRoot "scripts\start-xasr-sidecar-background.ps1"

    $xasrProcId = & $bgScript -Port $XasrPort -RepoRoot $RepoRoot
    if (-not $xasrProcId) { throw "x-asr background start failed" }
    Write-Host "  x-asr PID $xasrProcId" -ForegroundColor Green

    $mochiLog = Get-MochiDailyLogPath -RepoRoot $RepoRoot -ServiceName "mochi"
    if (-not (Wait-PortListening $XasrPort "x-asr" 60)) {
        throw "x-asr port not open. Sidecar API calls are logged in $mochiLog"
    }

    $probeScript = Join-Path $RepoRoot "scripts\probe-xasr.ps1"
    if (Test-Path $probeScript) {
        & $probeScript -WsUrl "ws://127.0.0.1:$XasrPort"
        if ($LASTEXITCODE -ne 0) {
            throw "x-asr probe failed"
        }
    }
}

function Start-XTtsService {
    Write-Step "Start x-tts sidecar (:$XttsPort)"
    $bgScript = Join-Path $RepoRoot "scripts\start-xtts-sidecar-background.ps1"

    $xttsProcId = & $bgScript -Port $XttsPort -RepoRoot $RepoRoot
    if (-not $xttsProcId) { throw "x-tts background start failed" }
    Write-Host "  x-tts PID $xttsProcId" -ForegroundColor Green

    $mochiLog = Get-MochiDailyLogPath -RepoRoot $RepoRoot -ServiceName "mochi"
    if (-not (Wait-HttpOk "http://127.0.0.1:$XttsPort/health" "x-tts" 120)) {
        throw "x-tts health check failed. Sidecar API calls are logged in $mochiLog"
    }
}

function Start-GoServer {
    param(
        [switch]$Foreground
    )

    Write-Step "Start Go API server (:$ServerPort)"
    $serverDir = Join-Path $RepoRoot "server"
    $mochiLog = Get-MochiDailyLogPath -RepoRoot $RepoRoot -ServiceName "mochi"

    New-Item -ItemType Directory -Force -Path (Split-Path $mochiLog -Parent) | Out-Null

    # FollowLogs: foreground Go; logging.Setup writes logs/mochi/mochi-YYYYMMDD.log too
    if ($Foreground) {
        Write-Host "  Mode     : foreground (console + persistent file)" -ForegroundColor Green
        Write-Host "  Log file : $mochiLog" -ForegroundColor Green
        Write-Host "  Ctrl+C stops Go only; emotion2vec / x-asr keep running" -ForegroundColor DarkGray
        Write-Host ""

        Push-Location $serverDir
        try {
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
                    -NoNewWindow `
                    -Wait `
                    -PassThru
            } else {
                $go = Get-Command go -ErrorAction SilentlyContinue
                if (-not $go) { throw "go not found in PATH" }
                $proc = Start-Process -FilePath $go.Source `
                    -ArgumentList @("run", "./cmd/server") `
                    -WorkingDirectory $serverDir `
                    -NoNewWindow `
                    -Wait `
                    -PassThru
            }
        } finally {
            Pop-Location
        }

        Write-Host ""
        Write-Host "Go server exited (code $($proc.ExitCode))" -ForegroundColor Yellow
        Write-Host "Log saved : $mochiLog" -ForegroundColor DarkGray
        return $proc.Id
    }

    # Background: Go logs only via logging.Setup (no separate out/err files)
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
            -PassThru
    } else {
        $go = Get-Command go -ErrorAction SilentlyContinue
        if (-not $go) { throw "go not found in PATH" }
        $proc = Start-Process -FilePath $go.Source `
            -ArgumentList @("run", "./cmd/server") `
            -WorkingDirectory $serverDir `
            -WindowStyle Hidden `
            -PassThru
    }

    Write-Host "  Go server PID $($proc.Id)" -ForegroundColor Green
    Write-Host "  Log file    : $mochiLog" -ForegroundColor DarkGray

    if (-not (Wait-PortListening $ServerPort "Go API" 90)) {
        throw "Go server not listening. See $mochiLog"
    }

    if (-not (Wait-HttpOk "http://127.0.0.1:$ServerPort/api/v1/public/config" "Go API" 30)) {
        throw "Go API health check failed. See $mochiLog"
    }

    return $proc.Id
}

Write-Host "Mochi backend restart" -ForegroundColor White
Write-Host "  repo:    $RepoRoot"
Write-Host "  logs:    $LogsRoot"
$mochiLogHint = Get-MochiDailyLogPath -RepoRoot $RepoRoot -ServiceName "mochi"
Write-Host "  mochi:   $mochiLogHint (Go + sidecar API calls)" -ForegroundColor DarkGray
Write-Host "  ports:   emotion2vec=$EmotionPort x-asr=$XasrPort x-tts=$XttsPort go=$ServerPort"

Write-Step "Ensure server voice models / venv"
. (Join-Path $RepoRoot "scripts\lib\ensure-models.ps1")
Ensure-MochiServerModels -RepoRoot $RepoRoot `
    -SkipEmotion2vec:$SkipEmotion2vec `
    -SkipXasr:$SkipXasr `
    -SkipXtts:$SkipXtts

Stop-AllBackend

if ($KillOnly) {
    Write-Host ""
    Write-Host "KillOnly: done." -ForegroundColor Green
    exit 0
}

try {
    if (-not $SkipEmotion2vec) {
        Start-Emotion2vecService
    } else {
        Write-Step "Skip emotion2vec"
    }

    if (-not $SkipXasr) {
        Start-XAsrService
    } else {
        Write-Step "Skip x-asr"
    }

    if (-not $SkipXtts) {
        Start-XTtsService
    } else {
        Write-Step "Skip x-tts"
    }

    $useForegroundGo = ($FollowLogs -and -not $NoFollowLogs)

    if ($useForegroundGo) {
        Write-Step "All sidecars started; starting Go in foreground"
        Write-Host "  emotion2vec : http://127.0.0.1:$EmotionPort/health" -ForegroundColor Green
        Write-Host "  x-asr       : ws://127.0.0.1:$XasrPort" -ForegroundColor Green
        Write-Host "  x-tts       : http://127.0.0.1:$XttsPort/health" -ForegroundColor Green
        Write-Host "  Mochi log   : $mochiLogHint" -ForegroundColor DarkGray

        $null = Start-GoServer -Foreground
        Read-Host "Press Enter to close this window"
        exit 0
    }

    $null = Start-GoServer

    Write-Step "All backend services started"
    Write-Host "  emotion2vec : http://127.0.0.1:$EmotionPort/health" -ForegroundColor Green
    Write-Host "  x-asr       : ws://127.0.0.1:$XasrPort" -ForegroundColor Green
    Write-Host "  x-tts       : http://127.0.0.1:$XttsPort/health" -ForegroundColor Green
    Write-Host "  Go API      : http://127.0.0.1:$ServerPort" -ForegroundColor Green
    Write-Host "  Mochi log   : $mochiLogHint" -ForegroundColor Green

    exit 0
} catch {
    Write-Host ""
    Write-Host "FAILED: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Check $mochiLogHint" -ForegroundColor Yellow
    Read-Host "Press Enter to exit"
    exit 1
}
