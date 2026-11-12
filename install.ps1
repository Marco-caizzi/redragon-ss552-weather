#Requires -Version 5.1
<#
.SYNOPSIS
    Installs the Open-Meteo weather plugin for Redragon SS-552 / StreamDock.

.DESCRIPTION
    1. Checks that StreamDock is installed and the plugin files are complete.
    2. Verifies weather.exe against SHA256SUMS.txt when present.
    3. Closes StreamDock and any running copy of the plugin, replaces the plugin folder and restarts StreamDock.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$PluginId          = "com.marco.weather.sdPlugin"
$PluginSource      = Join-Path $PSScriptRoot $PluginId
$StreamDockDataDir = Join-Path $env:APPDATA "HotSpot\StreamDock"
$StreamDockPlugins = Join-Path $StreamDockDataDir "plugins"
$PluginTarget      = Join-Path $StreamDockPlugins $PluginId
$ChecksumFile      = Join-Path $PSScriptRoot "SHA256SUMS.txt"

function Write-Ok([string]$Message)   { Write-Host "  OK  $Message" -ForegroundColor Green }
function Write-Info([string]$Message) { Write-Host "      $Message" -ForegroundColor Gray }
function Stop-Install([string]$Message) {
    Write-Host ""
    Write-Host "  ERROR  $Message" -ForegroundColor Red
    Write-Host "  La instalación se detuvo sin modificar nada." -ForegroundColor Red
    exit 1
}

Write-Host "Clima Open-Meteo para Redragon SS-552 - instalador" -ForegroundColor White
Write-Host ""

# Prerequisites
if (-not (Test-Path $StreamDockDataDir)) {
    Stop-Install "No se encontró StreamDock en '$StreamDockDataDir'. Instalalo y abrilo al menos una vez."
}
Write-Ok "StreamDock instalado"

foreach ($file in @("manifest.json", "weather.exe", "images\knob.png", "images\action.png", "images\plugin.png")) {
    if (-not (Test-Path (Join-Path $PluginSource $file))) {
        Stop-Install "Falta $file en '$PluginSource'. Ejecutá el instalador desde la carpeta descomprimida completa."
    }
}
Write-Ok "Archivos del plugin presentes"

# Integrity check against the checksum published with the release
$exe = Join-Path $PluginSource "weather.exe"
$exeHash = (Get-FileHash -Path $exe -Algorithm SHA256).Hash.ToUpperInvariant()
if (Test-Path $ChecksumFile) {
    $expected = (Get-Content $ChecksumFile | Where-Object { $_ -match "weather\.exe" } | Select-Object -First 1) -replace "\s.*$", ""
    if (-not $expected) { Stop-Install "SHA256SUMS.txt no tiene la línea de weather.exe." }
    if ($expected.ToUpperInvariant() -ne $exeHash) {
        Stop-Install "El SHA-256 de weather.exe no coincide con SHA256SUMS.txt.`n    Esperado: $expected`n    Obtenido: $exeHash"
    }
    Write-Ok "SHA-256 de weather.exe verificado ($($exeHash.Substring(0, 12))...)"
} else {
    Write-Info "Sin SHA256SUMS.txt: SHA-256 de weather.exe = $exeHash"
}

# Close StreamDock and the running plugin (it would keep weather.exe locked)
$streamDock = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -like "*StreamDock*" })
$streamDockExe = $null
if ($streamDock.Count -gt 0) {
    $streamDockExe = ($streamDock | Where-Object { $_.Path } | Select-Object -First 1).Path
    $streamDock | Stop-Process -Force
    Write-Ok "StreamDock cerrado"
}
$running = @(Get-Process -Name "weather" -ErrorAction SilentlyContinue)
if ($running.Count -gt 0) {
    $running | Stop-Process -Force
    Write-Ok "Versión anterior del plugin detenida"
}
Start-Sleep -Seconds 2

# Install
New-Item -ItemType Directory -Path $StreamDockPlugins -Force | Out-Null
if (Test-Path $PluginTarget) {
    Remove-Item -Path $PluginTarget -Recurse -Force
    Write-Info "Se reemplaza la versión anterior del plugin"
}
Copy-Item -Path $PluginSource -Destination $StreamDockPlugins -Recurse -Force
$version = (Get-Content (Join-Path $PluginTarget "manifest.json") -Raw | ConvertFrom-Json).Version
Write-Ok "Plugin v$version instalado en $PluginTarget"

if ($streamDockExe) {
    Start-Process -FilePath $streamDockExe
    Write-Ok "StreamDock reiniciado"
} else {
    Write-Info "Abrí StreamDock para cargar el plugin."
}

Write-Host ""
Write-Host "Instalación terminada." -ForegroundColor Green
Write-Host ""
Write-Host "En StreamDock:" -ForegroundColor White
Write-Host "  1. Buscá la categoría 'Clima Open-Meteo'."
Write-Host "  2. Arrastrá 'Clima (tecla)' a una tecla o 'Clima (perilla)' a una perilla."
Write-Host "  3. En la configuración, buscá tu ciudad y elegila de la lista (se muestra provincia y país)."
Write-Host ""
Write-Host "Diagnóstico: $PluginTarget\weather.log" -ForegroundColor Gray
