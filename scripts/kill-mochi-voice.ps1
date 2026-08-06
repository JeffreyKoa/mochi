# 安装/升级 Mochi 前运行：结束 Mochi 与 X-ASR/X-TTS sidecar，释放 bundle\voice 下 .pyd 文件锁
# 用法: powershell -ExecutionPolicy Bypass -File scripts\kill-mochi-voice.ps1

$ErrorActionPreference = 'SilentlyContinue'

Write-Host "Stopping Mochi.exe / mochi-desktop.exe ..."
taskkill /F /T /IM Mochi.exe 2>$null
taskkill /F /T /IM mochi-desktop.exe 2>$null

Write-Host "Stopping bundled voice python (path + command line) ..."
Get-CimInstance Win32_Process -Filter "Name='python.exe'" |
  Where-Object {
    $_.ExecutablePath -like '*\bundle\voice\runtime\python.exe' -or
    $_.CommandLine -match 'sherpa_streaming|tts_server|bundle\\voice'
  } |
  ForEach-Object {
    Write-Host "  kill PID $($_.ProcessId) $($_.CommandLine)"
    Stop-Process -Id $_.ProcessId -Force
  }

Write-Host "Stopping listeners on 8766 / 8767 ..."
8766, 8767 | ForEach-Object {
  Get-NetTCPConnection -LocalPort $_ -State Listen -ErrorAction SilentlyContinue |
    ForEach-Object {
      Write-Host "  kill port $_ PID $($_.OwningProcess)"
      Stop-Process -Id $_.OwningProcess -Force
    }
}

Start-Sleep -Seconds 2
Write-Host "Done. You can retry Mochi Setup now."
