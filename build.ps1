# Builds Exo into a clean dist/ directory:
#   dist\exo.exe              the desktop widget (Wails + WebView2)
#   dist\exo-v<version>.exe   versioned copy for releases
#   dist\exo-cli.exe          the terminal version
# Usage:  .\build.ps1  [-Version 1.2.0]
param(
    [string]$Version = "1.2.0"
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$dist = Join-Path $root "dist"

$wails = (Get-Command wails -ErrorAction SilentlyContinue).Source
if (-not $wails) {
    $wails = Join-Path (go env GOPATH) "bin\wails.exe"
}
if (-not (Test-Path $wails)) {
    throw "Wails CLI not found. Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
}

# A copy of Exo running from dist/ locks its exe; say so instead of failing halfway.
$running = Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith($dist + "\", [StringComparison]::OrdinalIgnoreCase) }
if ($running) {
    throw ("Close Exo first - running from dist\: " + (($running | ForEach-Object Name) -join ", "))
}

# Clean dist/
if (Test-Path $dist) { Remove-Item -Recurse -Force $dist }
New-Item -ItemType Directory -Force $dist | Out-Null

Push-Location $root
try {
    Write-Host "Building desktop app (Wails) ..." -ForegroundColor Magenta
    & $wails build -clean -trimpath -ldflags "-s -w"
    if ($LASTEXITCODE -ne 0) { throw "wails build failed" }
    Copy-Item (Join-Path $root "build\bin\exo.exe") (Join-Path $dist "exo.exe")
    Copy-Item (Join-Path $dist "exo.exe") (Join-Path $dist ("exo-v{0}.exe" -f $Version))

    Write-Host "Building terminal version ..." -ForegroundColor Magenta
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-s -w" -o (Join-Path $dist "exo-cli.exe") ./cmd/exo-cli
    if ($LASTEXITCODE -ne 0) { throw "go build (cli) failed" }
}
finally {
    Pop-Location
}

Write-Host "Done." -ForegroundColor Green
Get-ChildItem $dist | ForEach-Object {
    "{0,-22} {1,6:N1} MB  SHA-256 {2}" -f $_.Name, ($_.Length / 1MB), (Get-FileHash $_.FullName -Algorithm SHA256).Hash
}
