# Builds Exo into a clean dist/ directory.
# Usage:  .\build.ps1  [-Version 1.0.0]
param(
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$dist = Join-Path $root "dist"

# Clean dist/
if (Test-Path $dist) { Remove-Item -Recurse -Force $dist }
New-Item -ItemType Directory -Force $dist | Out-Null

# Build (static, stripped, windows/amd64)
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$out = Join-Path $dist "exo.exe"
Write-Host "Building exo.exe -> dist\ ..." -ForegroundColor Magenta
go build -ldflags "-s -w" -o $out ./src
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

# Versioned copy for releases
$verOut = Join-Path $dist ("exo-v{0}.exe" -f $Version)
Copy-Item $out $verOut -Force

$hash = (Get-FileHash $out -Algorithm SHA256).Hash
Write-Host "Done." -ForegroundColor Green
Get-ChildItem $dist | Select-Object Name, Length
Write-Host ("SHA-256: {0}" -f $hash)
