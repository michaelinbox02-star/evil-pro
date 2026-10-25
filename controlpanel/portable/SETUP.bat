@echo off
setlocal
cd /d "%~dp0"
if not exist "evilpro-panel.exe" (
  echo evilpro-panel.exe not found in this folder.
  pause
  exit /b 1
)
"evilpro-panel.exe" -setup
