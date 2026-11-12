@echo off
rem Double-click launcher: removes the weather plugin from StreamDock.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0uninstall.ps1" %*
echo.
pause
