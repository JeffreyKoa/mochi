# 停止占用 bundle/voice DLL 的 X-ASR / X-TTS sidecar（tauri dev 重建前调用）
$ErrorActionPreference = "SilentlyContinue"

# 优先结束 bundle 内置 Python（会锁住 site-packages 下 DLL）
Get-CimInstance Win32_Process -Filter "Name='python.exe'" |
    Where-Object { $_.ExecutablePath -like '*\bundle\voice\runtime\python.exe' } |
    ForEach-Object {
        Write-Host "Stop bundled python PID $($_.ProcessId)" -ForegroundColor Yellow
        Stop-Process -Id $_.ProcessId -Force
    }

# 结束 tools/x-asr、tools/x-tts 的 sidecar（含系统 Python 误启动的副本）
Get-CimInstance Win32_Process -Filter "Name='python.exe'" |
    Where-Object {
        $_.CommandLine -like '*sherpa_streaming_server.py*' -or
        $_.CommandLine -like '*tts_server.py*'
    } |
    ForEach-Object {
        Write-Host "Stop sidecar python PID $($_.ProcessId)" -ForegroundColor Yellow
        Stop-Process -Id $_.ProcessId -Force
    }

function Stop-PortListener([int]$Port) {
    $conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    foreach ($c in $conn) {
        $pid = $c.OwningProcess
        if ($pid -gt 0) {
            Write-Host "Stop PID $pid (port $Port listen)" -ForegroundColor Yellow
            Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
            taskkill /PID $pid /T /F 2>$null | Out-Null
        }
    }
}

Stop-PortListener 8766
Stop-PortListener 8767

Start-Sleep -Milliseconds 400
Write-Host "Voice sidecars stopped." -ForegroundColor Green
