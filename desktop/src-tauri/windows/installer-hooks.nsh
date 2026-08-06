; Tauri NSIS：安装/卸载前结束 Mochi 及 bundled 语音 sidecar，避免 .pyd 被占用无法覆盖。
; 典型报错：Error opening file for writing ...\_cffi_backend.cp311-win_amd64.pyd

!macro KillMochiVoiceSidecars
  DetailPrint "Stopping Mochi and local voice sidecars (pass 1)..."
  nsExec::Exec 'taskkill /F /T /IM Mochi.exe'
  Pop $0
  nsExec::Exec 'taskkill /F /T /IM mochi-desktop.exe'
  Pop $0
  ; bundled voice python（按路径 + 命令行）
  nsExec::Exec 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-CimInstance Win32_Process -Filter \"Name=''python.exe''\" -ErrorAction SilentlyContinue | Where-Object { $$_.ExecutablePath -like ''*\bundle\voice\runtime\python.exe'' -or $$_.CommandLine -match ''sherpa_streaming|tts_server|bundle\\voice'' } | ForEach-Object { Stop-Process -Id $$_.ProcessId -Force -ErrorAction SilentlyContinue }"'
  Pop $0
  ; 8766 / 8767 端口监听进程
  nsExec::Exec 'powershell -NoProfile -ExecutionPolicy Bypass -Command "8766,8767 | ForEach-Object { Get-NetTCPConnection -LocalPort $$_ -State Listen -ErrorAction SilentlyContinue | ForEach-Object { Stop-Process -Id $$_.OwningProcess -Force -ErrorAction SilentlyContinue } }"'
  Pop $0
  Sleep 2000

  DetailPrint "Stopping Mochi and local voice sidecars (pass 2)..."
  nsExec::Exec 'taskkill /F /T /IM Mochi.exe'
  Pop $0
  nsExec::Exec 'taskkill /F /T /IM mochi-desktop.exe'
  Pop $0
  nsExec::Exec 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-CimInstance Win32_Process -Filter \"Name=''python.exe''\" -ErrorAction SilentlyContinue | Where-Object { $$_.ExecutablePath -like ''*\bundle\voice\runtime\python.exe'' } | ForEach-Object { Stop-Process -Id $$_.ProcessId -Force -ErrorAction SilentlyContinue }"'
  Pop $0
  ; 等待 DLL/.pyd 句柄释放
  Sleep 3000
!macroend

!macro NSIS_HOOK_PREINSTALL
  !insertmacro KillMochiVoiceSidecars
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  !insertmacro KillMochiVoiceSidecars
!macroend
