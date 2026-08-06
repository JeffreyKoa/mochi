# 安装/覆盖升级前：结束 Mochi 与 bundled 语音 sidecar，释放 bundle/voice 下 .pyd 文件锁
# 用法：.\scripts\stop-mochi-for-install.ps1

$ErrorActionPreference = "SilentlyContinue"

Write-Host "Stopping Mochi..." -ForegroundColor Yellow
foreach ($name in @("Mochi", "mochi-desktop")) {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
        Write-Host "  taskkill /T /F PID $($_.Id) ($name)"
        taskkill /PID $_.Id /T /F 2>$null | Out-Null
    }
}

Write-Host "Stopping bundled voice python..." -ForegroundColor Yellow
Get-CimInstance Win32_Process -Filter "Name='python.exe'" |
    Where-Object { $_.ExecutablePath -like '*\bundle\voice\runtime\python.exe' } |
    ForEach-Object {
        Write-Host "  Stop PID $($_.ProcessId) $($_.ExecutablePath)"
        Stop-Process -Id $_.ProcessId -Force
    }

& (Join-Path $PSScriptRoot "stop-voice-sidecars.ps1")

Start-Sleep -Milliseconds 800
Write-Host "Done. You can retry the installer now." -ForegroundColor Green
