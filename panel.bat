@echo off
setlocal
cd /d "%~dp0"

REM Dev launcher: builds if needed, then runs from the repo.
REM For a portable copy, run build-portable.bat and use dist\evil-pro-control-panel\START.bat

if not exist "build\evilpro-panel.exe" (
  echo building the control panel...
  call build-panel.bat || goto :err
)

set BIND=%~1
if "%BIND%"=="" set BIND=127.0.0.1:8080

echo.
echo   evil-pro control panel  ^-^>  http://%BIND%
echo   login is stored in build\panel.json (empty password = open)
echo.
"build\evilpro-panel.exe" -bind "%BIND%" -repo "%~dp0"
goto :eof

:err
echo build failed - is Go installed?
pause
