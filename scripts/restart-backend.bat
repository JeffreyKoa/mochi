@echo off
REM Mochi backend restart (kill all, then start all)
REM Usage: scripts\restart-backend.bat
REM        scripts\restart-backend.bat -KillOnly
REM        scripts\restart-backend.bat -BuildOpus

setlocal
set "ROOT=%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%restart-backend.ps1" %*
exit /b %ERRORLEVEL%
