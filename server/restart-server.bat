@echo off
REM Stop, rebuild Opus server, and start.
REM Usage: restart-server.bat

setlocal EnableExtensions EnableDelayedExpansion
set "ROOT=%~dp0"
set "MSYS2_MINGW=C:\msys64\mingw64"

cd /d "%ROOT%"
for %%I in ("%ROOT%..\logs") do set "LOG_DIR=%%~fI"
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"

echo.
echo === Mochi Server Restart (Opus) ===
echo.

echo [1/5] Stopping server.exe ...
taskkill /IM server.exe /F >nul 2>&1
ping -n 2 127.0.0.1 >nul
tasklist /FI "IMAGENAME eq server.exe" 2>nul | find /I "server.exe" >nul
if not errorlevel 1 (
  echo       WARN: server.exe still running, retrying ...
  taskkill /IM server.exe /F >nul 2>&1
  ping -n 3 127.0.0.1 >nul
)
tasklist /FI "IMAGENAME eq server.exe" 2>nul | find /I "server.exe" >nul
if not errorlevel 1 (
  echo       ERROR: cannot stop server.exe. Close it manually.
  exit /b 1
)
echo       Stopped.

echo [2/5] Building Opus server (CGO) ...
call "%ROOT%build-opus.bat"
if errorlevel 1 (
  echo.
  echo FAILED: build-opus.bat failed - cannot start without Opus binary.
  exit /b 1
)

echo [3/5] Verifying libopus-0.dll ...
if not exist "bin\libopus-0.dll" (
  echo       ERROR: bin\libopus-0.dll missing
  exit /b 1
)
echo       OK

echo [4/5] Starting server ...
set "PATH=%MSYS2_MINGW%\bin;%PATH%"
start "" /B bin\server.exe >> "%LOG_DIR%\server-out.log" 2>> "%LOG_DIR%\server-err.log"
ping -n 3 127.0.0.1 >nul

set "PID="
for /f "tokens=2" %%p in ('tasklist /FI "IMAGENAME eq server.exe" /FO LIST 2^>nul ^| find "PID:"') do set "PID=%%p"
if not defined PID (
  echo FAILED: server did not start. See %LOG_DIR%\server-err.log
  exit /b 1
)

set "PORT_OK=0"
netstat -ano 2>nul | findstr /C:":8081" | findstr "LISTENING" | findstr "%PID%" >nul && set "PORT_OK=1"

for %%I in ("bin\server.exe") do set "SIZE=%%~zI"
set "OPUS_OK=no"
if defined SIZE if !SIZE! GEQ 25000000 set "OPUS_OK=yes"

echo.
echo ============================
echo   Restart OK
echo   PID     : %PID%
echo   Port    : 8081
if "%PORT_OK%"=="1" (echo   Listen  : yes) else (echo   Listen  : waiting)
echo   Binary  : !SIZE! bytes (Opus build ^>= 25MB)
echo   Opus    : !OPUS_OK!
echo   Logs    : %LOG_DIR%
echo ============================
if "!OPUS_OK!"=="no" (
  echo.
  echo WARN: binary is not Opus build. build-opus.bat may have failed.
)
echo.

endlocal
