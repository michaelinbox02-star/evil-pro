@echo off
setlocal
cd /d "%~dp0"
echo === building the native host (enables "Own profile" in the extension) ===
set CGO_ENABLED=0
mkdir extension\host\bin 2>nul

set GOOS=windows
set GOARCH=amd64
go build -mod=mod -o "extension\host\bin\evilpro-host.exe" .\extension\host || goto :err
set GOOS=linux
go build -mod=mod -o "extension\host\bin\evilpro-host-linux" .\extension\host || goto :err
set GOOS=darwin
go build -mod=mod -o "extension\host\bin\evilpro-host-mac" .\extension\host || goto :err

echo.
echo Done. Now run extension\host\install.bat once to register it.
echo.
dir /b extension\host\bin
exit /b 0

:err
echo.
echo *** BUILD FAILED ***
exit /b 1
