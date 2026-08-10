@echo off
REM Mochi backend restart (kill all, then start all)
REM Services: emotion2vec :8091 | moondream :8093 | x-asr :8766 | x-tts :8767 | Go :8081
REM Keeps this console open; Go logs print here AND save to logs/mochi/mochi-YYYYMMDD.log
REM Usage: scripts\restart-backend.bat
REM        scripts\restart-backend.bat -KillOnly
REM        scripts\restart-backend.bat -BuildOpus
REM        scripts\restart-backend.bat -NoFollowLogs

setlocal
set "ROOT=%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%restart-backend.ps1" -FollowLogs %*
set "EXITCODE=%ERRORLEVEL%"
if %EXITCODE% neq 0 (
    echo.
    echo Backend restart failed with exit code %EXITCODE%
    pause
)
exit /b %EXITCODE%
