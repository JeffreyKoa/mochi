# Mochi backend one-shot restart: kill ALL backend processes, then start ALL services.
#
# Services:
#   emotion2vec  :8091  acoustic SER
#   moondream    :8093  local vision JPEG→text
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
    [switch]$SkipMoondream,
    [switch]$SkipXasr,
    [switch]$SkipXtts,
    [switch]$FollowLogs,
    [switch]$NoFollowLogs,
    [int]$ServerPort = 8081,
    [int]$XasrPort = 8766,
    [int]$XttsPort = 8767,
    [int]$EmotionPort = 8091,
    [int]$MoondreamPort = 8093,
    [int]$HealthTimeoutSec = 180
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = (Resolve-Path (Join-Path $ScriptDir "..")).Path
$LogsRoot = Join-Path $RepoRoot "logs"
$MoondreamLegacyPort = 8092

# Align sidecar launcher with config.yaml (avoid stale shell MOONDREAM_PORT=8092)
$env:MOONDREAM_PORT = "$MoondreamPort"

New-Item -ItemType Directory -Force -Path $LogsRoot | Out-Null
. (Join-Path $RepoRoot "scripts\lib\daily-log.ps1")
. (Join-Path $RepoRoot "scripts\lib\read-config-modules.ps1")

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

function Get-PortListenerPids {
    param([int]$Port)
    $pids = New-Object System.Collections.Generic.List[int]
    foreach ($c in @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)) {
        if ($c.OwningProcess -gt 0) { [void]$pids.Add([int]$c.OwningProcess) }
    }
    foreach ($line in @(netstat -ano | Select-String "127\.0\.0\.1:$Port\s")) {
        $text = $line.ToString().Trim()
        if ($text -notmatch 'LISTENING') { continue }
        $procId = [int](($text -split '\s+')[-1])
        if ($procId -gt 0) { [void]$pids.Add($procId) }
    }
    return @($pids | Sort-Object -Unique)
}

function Stop-PortListener {
    param(
        [int]$Port,
        [string]$Label
    )
    for ($attempt = 0; $attempt -lt 5; $attempt++) {
        $pids = @(Get-PortListenerPids $Port)
        if ($pids.Count -eq 0) { return }
        foreach ($procId in $pids) {
            Write-Host "  stop $Label : port $Port PID $procId (attempt $($attempt + 1))" -ForegroundColor Yellow
            Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
            cmd /c "taskkill /PID $procId /T /F >nul 2>&1"
        }
        Start-Sleep -Milliseconds 800
    }
}

function Assert-BackendPortsFree {
    $blocked = @()
    foreach ($p in @($ServerPort, $XasrPort, $XttsPort, $EmotionPort, $MoondreamPort)) {
        $pids = @(Get-PortListenerPids $p)
        if ($pids.Count -gt 0) {
            $blocked += "port $p PID $($pids -join ',')"
        }
    }
    if ($MoondreamLegacyPort -ne $MoondreamPort) {
        $legacyPids = @(Get-PortListenerPids $MoondreamLegacyPort)
        if ($legacyPids.Count -gt 0) {
            Write-Host "  WARN legacy moondream port $MoondreamLegacyPort still held by PID $($legacyPids -join ',') (using $MoondreamPort instead)" -ForegroundColor Yellow
        }
    }
    if ($blocked.Count -gt 0) {
        throw "Ports still in use after kill: $($blocked -join '; '). Run this script as Administrator or reboot."
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

# Wait until Moondream /health reports model_loaded=true (CPU warmup ~15-60s)
function Wait-MoondreamReady {
    param(
        [string]$Url,
        [string]$Label,
        [int]$TimeoutSec = 120
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $r = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
            if ($r.StatusCode -eq 200) {
                $body = $r.Content | ConvertFrom-Json
                if ($body.model_loaded -eq $true) {
                    Write-Host "  OK  $Label $Url (model_loaded)" -ForegroundColor Green
                    return $true
                }
                if ($body.load_error) {
                    Write-Host "  FAIL $Label load_error: $($body.load_error)" -ForegroundColor Red
                    return $false
                }
            }
        } catch {
            # retry
        }
        Start-Sleep -Seconds 2
    }
    Write-Host "  FAIL $Label model not loaded after $TimeoutSec sec" -ForegroundColor Red
    return $false
}

function Stop-AllBackend {
    Write-Step "Kill all Mochi backend processes"

    Stop-PortListener -Port $ServerPort -Label "Go API"
    Stop-PortListener -Port $XasrPort -Label "x-asr"
    Stop-PortListener -Port $XttsPort -Label "x-tts"
    Stop-PortListener -Port $EmotionPort -Label "emotion2vec"
    Stop-PortListener -Port $MoondreamPort -Label "moondream"
    if ($MoondreamLegacyPort -ne $MoondreamPort) {
        Stop-PortListener -Port $MoondreamLegacyPort -Label "moondream-legacy"
    }

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
        "services/emotion2vec",
        "services\moondream",
        "services/moondream"
    ) -Label "python sidecar"

    Start-Sleep -Seconds 2

    foreach ($p in @($ServerPort, $XasrPort, $XttsPort, $EmotionPort, $MoondreamPort, $MoondreamLegacyPort)) {
        if (Test-PortListening $p) {
            Write-Host "  WARN port $p still in use, force kill again" -ForegroundColor Yellow
            Stop-PortListener -Port $p -Label "port-$p"
        }
    }

    Start-Sleep -Milliseconds 800
    Assert-BackendPortsFree

    Write-Host "All backend processes stopped." -ForegroundColor Green
}

function Start-Emotion2vecService {
    Write-Step "Start emotion2vec (:$EmotionPort)"
    $startScript = Join-Path $RepoRoot "services\emotion2vec\start.ps1"
    if (-not (Test-Path $startScript)) {
        throw "Missing $startScript"
    }

    # SetupOnly already done by Ensure-MochiServerModels; start uvicorn in background
    & $startScript -Background -RepoRoot $RepoRoot
    if ($LASTEXITCODE -ne 0) { throw "emotion2vec background start failed" }

    $mochiLog = Get-MochiDailyLogPath -RepoRoot $RepoRoot -ServiceName "mochi"
    if (-not (Wait-HttpOk "http://127.0.0.1:$EmotionPort/health" "emotion2vec" $HealthTimeoutSec)) {
        throw "emotion2vec health check failed. Sidecar API calls are logged in $mochiLog"
    }
}

function Start-MoondreamService {
    Write-Step "Start moondream (:$MoondreamPort)"
    $startScript = Join-Path $RepoRoot "services\moondream\start.ps1"
    if (-not (Test-Path $startScript)) {
        throw "Missing $startScript"
    }

    $env:MOONDREAM_PORT = "$MoondreamPort"
    Stop-PortListener -Port $MoondreamPort -Label "moondream"
    if ($MoondreamLegacyPort -ne $MoondreamPort) {
        Stop-PortListener -Port $MoondreamLegacyPort -Label "moondream-legacy"
    }

    & $startScript -Background -RepoRoot $RepoRoot -Port $MoondreamPort
    if ($LASTEXITCODE -ne 0) { throw "moondream background start failed" }

    $mochiLog = Get-MochiDailyLogPath -RepoRoot $RepoRoot -ServiceName "mochi"
    if (-not (Wait-MoondreamReady "http://127.0.0.1:$MoondreamPort/health" "moondream" 180)) {
        throw "moondream model warmup failed. See $mochiLog"
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
Write-Host "  ports:   emotion2vec=$EmotionPort moondream=$MoondreamPort (legacy=$MoondreamLegacyPort) x-asr=$XasrPort x-tts=$XttsPort go=$ServerPort"

Stop-AllBackend

# Load module switches from config (modules.*.enabled); CLI -Skip* overrides
$getModuleScript = Join-Path $RepoRoot "scripts\get-module-switches.ps1"
if (-not (Test-Path -LiteralPath $getModuleScript)) {
    throw "Missing module switch helper: $getModuleScript"
}
$moduleSwitches = & $getModuleScript -RepoRoot $RepoRoot
if ($null -eq $moduleSwitches) {
    throw "Get-MochiYamlModuleSwitches returned null (RepoRoot=$RepoRoot)"
}

if ($KillOnly) {
    Write-Host ""
    Write-Host "KillOnly: done." -ForegroundColor Green
    exit 0
}

if (-not $SkipXasr -and -not $moduleSwitches['asr']) {
    $SkipXasr = $true
    Write-Host "  config: modules.asr.enabled=false -> skip x-asr" -ForegroundColor DarkGray
}
if (-not $SkipXtts -and -not $moduleSwitches['tts']) {
    $SkipXtts = $true
    Write-Host "  config: modules.tts.enabled=false -> skip x-tts" -ForegroundColor DarkGray
}
if (-not $SkipMoondream -and -not $moduleSwitches['vision']) {
    $SkipMoondream = $true
    Write-Host "  config: modules.vision.enabled=false -> skip moondream" -ForegroundColor DarkGray
}
if (-not $SkipEmotion2vec -and -not $moduleSwitches['emotion']) {
    $SkipEmotion2vec = $true
    Write-Host "  config: modules.emotion.enabled=false -> skip emotion2vec" -ForegroundColor DarkGray
}
# 远程 provider 无需本地 sidecar
if (-not $SkipMoondream -and -not (Test-MochiModuleNeedsLocalSidecar -RepoRoot $RepoRoot -ModuleName 'vision')) {
    $SkipMoondream = $true
    Write-Host "  config: vision provider=remote -> skip moondream sidecar" -ForegroundColor DarkGray
}

Write-Step "Ensure server voice models / venv"
. (Join-Path $RepoRoot "scripts\lib\ensure-models.ps1")
Ensure-MochiServerModels -RepoRoot $RepoRoot `
    -SkipEmotion2vec:$SkipEmotion2vec `
    -SkipMoondream:$SkipMoondream `
    -SkipXasr:$SkipXasr `
    -SkipXtts:$SkipXtts

try {
    if (-not $SkipEmotion2vec) {
        Start-Emotion2vecService
    } else {
        Write-Step "Skip emotion2vec"
    }

    if (-not $SkipMoondream) {
        Start-MoondreamService
    } else {
        Write-Step "Skip moondream"
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
        Write-Host "  moondream   : http://127.0.0.1:$MoondreamPort/health" -ForegroundColor Green
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
    Write-Host "  moondream   : http://127.0.0.1:$MoondreamPort/health" -ForegroundColor Green
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
