@echo off
REM Build Mochi server with Opus (CGO + libopus). Requires MSYS2 mingw-w64-opus.

setlocal EnableExtensions
set "ROOT=%~dp0"
set "MSYS2_MINGW=C:\msys64\mingw64"

cd /d "%ROOT%"

if not exist "%MSYS2_MINGW%\include\opus\opus.h" (
  echo [build-opus] ERROR: libopus not found.
  echo Install: pacman -S mingw-w64-x86_64-opus mingw-w64-x86_64-gcc
  exit /b 1
)

echo [build-opus] building with CGO + libopus ...
C:\msys64\usr\bin\bash.exe -lc "cd /d/ocr/Mochi/server && bash build-opus.sh"
if errorlevel 1 (
  echo [build-opus] ERROR: compile failed
  exit /b 1
)

copy /Y "%MSYS2_MINGW%\bin\libopus-0.dll" "bin\" >nul
for %%D in (libgcc_s_seh-1.dll libwinpthread-1.dll libstdc++-6.dll) do (
  if exist "%MSYS2_MINGW%\bin\%%D" copy /Y "%MSYS2_MINGW%\bin\%%D" "bin\" >nul
)

for %%I in ("bin\server.exe") do set "SIZE=%%~zI"
echo [build-opus] OK  bin\server.exe (%SIZE% bytes^) + libopus-0.dll

endlocal
