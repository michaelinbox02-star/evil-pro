@echo off
setlocal
cd /d "%~dp0"

echo === evil-pro build ===
set CGO_ENABLED=0

echo [1/3] evilginx engine  linux/amd64
set GOOS=linux
set GOARCH=amd64
go build -mod=mod -o build\evilginx-linux . || goto :err

echo [2/3] evilginx engine  linux/arm64
set GOARCH=arm64
go build -mod=mod -o build\evilginx-linux-arm64 . || goto :err

echo [3/3] control panel     windows/amd64
set GOOS=windows
set GOARCH=amd64
go build -mod=mod -o build\evilpro-panel.exe .\controlpanel || goto :err

echo.
echo Done. Binaries:
dir /b build\evilginx-linux build\evilginx-linux-arm64 build\evilpro-panel.exe
exit /b 0

:err
echo.
echo *** BUILD FAILED ***
exit /b 1
