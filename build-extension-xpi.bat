@echo off
setlocal
cd /d "%~dp0"
set CGO_ENABLED=0
rem Packs the extension into an installable .xpi (a zip with manifest.json at the
rem root, forward-slash paths). Firefox needs a file like this to install it
rem permanently - a folder can only ever be loaded temporarily.
set OUT=dist\session-injector.xpi

go run -mod=mod ./tools/xpi extension "%OUT%" || goto :err

echo.
echo Built %OUT%
echo.
echo Firefox, quick test (any version, no signing needed):
echo   about:debugging -^> This Firefox -^> Load Temporary Add-on -^> extension\manifest.json
echo   (must be redone after every Firefox restart)
echo.
echo Firefox, permanent install from the xpi:
echo   about:addons -^> gear icon -^> Install Add-on From File -^> pick %OUT%
echo   Release Firefox only installs *signed* add-ons. If it refuses, either
echo   upload it at addons.mozilla.org as an "unlisted" add-on to get a free
echo   signature (then install the signed file it hands back), or use Developer
echo   Edition / ESR with about:config -^> xpinstall.signatures.required = false
echo.
exit /b 0

:err
echo.
echo *** BUILD FAILED ***
exit /b 1
