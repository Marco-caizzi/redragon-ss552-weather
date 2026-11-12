@echo off
rem Double-click launcher: runs install.ps1 for this process only, without changing the system execution policy.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*
echo.
pause
