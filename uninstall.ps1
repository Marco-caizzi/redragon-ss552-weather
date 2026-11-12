#Requires -Version 5.1
<# Removes the weather plugin from StreamDock. #>
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$PluginTarget = Join-Path $env:APPDATA "HotSpot\StreamDock\plugins\com.marco.weather.sdPlugin"

$streamDock = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -like "*StreamDock*" })
$streamDockExe = $null
if ($streamDock.Count -gt 0) {
    $streamDockExe = ($streamDock | Where-Object { $_.Path } | Select-Object -First 1).Path
    $streamDock | Stop-Process -Force
}
Get-Process -Name "weather" -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2

if (Test-Path $PluginTarget) {
    Remove-Item -Path $PluginTarget -Recurse -Force
    Write-Host "  OK  Plugin eliminado" -ForegroundColor Green
} else {
    Write-Host "      El plugin no estaba instalado" -ForegroundColor Gray
}
if ($streamDockExe) {
    Start-Process -FilePath $streamDockExe
    Write-Host "  OK  StreamDock reiniciado" -ForegroundColor Green
}
