#Requires -Version 5.1
<#
.SYNOPSIS
Create and build the phase-0 ApexVoid Photobooth Windows portable preview.
.DESCRIPTION
Uses the pinned Wails 3 React TypeScript template, then overlays tracked
ApexVoid shell sources. Does not modify or execute the ERP-era application.
#>
param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$desktopRoot = Join-Path $root 'desktop'
$desktop = Join-Path $desktopRoot 'apexvoidphotobooth'
$overlay = Join-Path $root 'desktop-bootstrap'

if (-not (Get-Command 'wails3' -ErrorAction SilentlyContinue)) {
    throw 'wails3 was not found. Install with: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0'
}
if (-not (Get-Command 'node' -ErrorAction SilentlyContinue)) { throw 'Node.js 22+ is required' }
if (-not (Get-Command 'go' -ErrorAction SilentlyContinue)) { throw 'Go 1.26+ is required' }

Push-Location $root
try {
    if (-not (Test-Path (Join-Path $desktop 'Taskfile.yml'))) {
        if (Test-Path $desktopRoot) {
            throw 'desktop/ exists but is not a complete Wails project. Move it aside before bootstrapping.'
        }
        & wails3 init -q -n apexvoidphotobooth -t react -d desktop -mod github.com/st-mich43l/apexvoid-photobooth/desktop -productname 'ApexVoid Photobooth' -productdescription 'Offline-first desktop photo booth' -productversion '0.1.0' -productcompany 'ApexVoid' -productidentifier 'net.apexvoid.photobooth'
        if ($LASTEXITCODE -ne 0) { throw 'Wails project generation failed' }
    }

    Copy-Item (Join-Path $overlay 'main.go.tmpl') (Join-Path $desktop 'main.go') -Force
    Copy-Item (Join-Path $overlay 'desktopservice.go.tmpl') (Join-Path $desktop 'desktopservice.go') -Force
    Copy-Item (Join-Path $overlay 'App.tsx') (Join-Path $desktop 'frontend/src/App.tsx') -Force
    Copy-Item (Join-Path $overlay 'style.css') (Join-Path $desktop 'frontend/public/style.css') -Force
    Copy-Item (Join-Path $overlay 'index.html') (Join-Path $desktop 'frontend/index.html') -Force

    $oldGreet = Join-Path $desktop 'greetservice.go'
    if (Test-Path $oldGreet) { Remove-Item $oldGreet -Force }

    Push-Location $desktop
    try {
        & go mod tidy
        if ($LASTEXITCODE -ne 0) { throw 'go mod tidy failed' }
        & npm --prefix frontend install --no-audit --no-fund
        if ($LASTEXITCODE -ne 0) { throw 'npm install failed' }
        if (-not $SkipBuild) {
            & wails3 build GOOS=windows GOARCH=amd64
            if ($LASTEXITCODE -ne 0) { throw 'Wails Windows build failed' }
            $output = Get-ChildItem -Path (Join-Path $desktop 'bin') -Filter '*.exe' -ErrorAction Stop
            if (-not $output) { throw 'No portable EXE found in desktop/bin' }
            $output | ForEach-Object { Write-Host ('Portable build: ' + $_.FullName) }
        }
    }
    finally { Pop-Location }
}
finally { Pop-Location }
