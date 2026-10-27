@echo off
setlocal
cd /d "%~dp0"

if not exist "bin\evilpro-host.exe" (
  echo.
  echo   host binary not found at bin\evilpro-host.exe
  echo   build it with:  go build -o "bin\evilpro-host.exe" ./extension/host
  echo   (or run build-portable.bat, which builds all of them)
  echo.
  pause
  exit /b 1
)

echo Registering the evil-pro native host with your browsers...
echo.
"bin\evilpro-host.exe" install
set RC=%ERRORLEVEL%
echo.

if not "%RC%"=="0" (
  echo Registration failed - see the message above.
  pause
  exit /b 1
)

echo Done. "Own profile" is now available in the extension popup.
echo Restart the browser if it was already running.
echo.
pause
