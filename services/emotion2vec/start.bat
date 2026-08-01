@echo off
chcp 65001 >nul 2>&1
cd /d "%~dp0"
echo.
echo Starting Mochi emotion2vec Sidecar...
echo.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0start.ps1"
set ERR=%ERRORLEVEL%
echo.
if not "%ERR%"=="0" (
  echo [ERROR] Script failed with exit code %ERR%
)
pause
