@echo off
setlocal
cd /d "%~dp0"

if not exist "evilpro-panel.exe" (
  echo evilpro-panel.exe not found in this folder.
  pause
  exit /b 1
)

if not exist "panel.json" (
  echo.
  echo   First run - let's set up your login and port.
  echo.
  "evilpro-panel.exe" -setup
)

echo   starting control panel...
"evilpro-panel.exe"
pause
