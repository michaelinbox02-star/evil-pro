@echo off
setlocal
cd /d "%~dp0"

if not exist "build\evilpro-panel.exe" (
  echo Control panel binary not found - building it first...
  call build-panel.bat || goto :err
)

if "%PANEL_BIND%"=="" set PANEL_BIND=127.0.0.1:8080
if "%PANEL_USER%"=="" set PANEL_USER=admin

echo.
echo   evil-pro control panel
echo   ----------------------
echo   URL   : http://%PANEL_BIND%
echo   user  : %PANEL_USER%  (password from PANEL_PASS or empty = none)
echo   store : %%APPDATA%%\evilpro-panel\profiles.json
echo.
echo   TIP: to host it, set PANEL_BIND=0.0.0.0:8080 and PANEL_PASS=secret
echo.

"build\evilpro-panel.exe" -bind "%PANEL_BIND%" -user "%PANEL_USER%" -pass "%PANEL_PASS%"
goto :eof

:err
echo.
echo Build failed. Make sure Go is installed and in PATH.
pause
