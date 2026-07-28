@echo off
setlocal EnableExtensions
set "ROOT=%~dp0"
set "MSYS2_MINGW=C:\msys64\mingw64\bin"

cd /d "%ROOT%"
for %%I in ("%ROOT%..\logs") do set "LOG_DIR=%%~fI"
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"

set "PATH=%MSYS2_MINGW%;%PATH%"
start "" /B bin\server.exe >> "%LOG_DIR%\server-out.log" 2>> "%LOG_DIR%\server-err.log"

echo [start-server] started, logs: %LOG_DIR%
