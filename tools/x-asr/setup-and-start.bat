@echo off
chcp 65001 >nul 2>&1
cd /d "%~dp0"
title Mochi X-ASR Sidecar

echo.
echo  Mochi X-ASR setup-and-start
echo  ===========================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup-and-start.ps1" %*
set RC=%ERRORLEVEL%

echo.
if not "%RC%"=="0" (
    echo [FAILED] exit code %RC% - see errors above.
) else (
    echo [DONE] sidecar stopped.
)
echo.
pause
exit /b %RC%
