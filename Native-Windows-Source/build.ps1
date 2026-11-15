# Builds "KeyClick Tracker.exe" into the parent folder.
# Uses the portable Go in ..\_tools\go when present, otherwise the Go on PATH.
param([string]$Out = '')   # default: ..\KeyClick-Tracker-Setup-v<version>.exe
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$tools = Join-Path (Split-Path $PSScriptRoot -Parent) '_tools'
$goExe = Join-Path $tools 'go\bin\go.exe'
if (Test-Path $goExe) {
  $env:GOROOT = Join-Path $tools 'go'; $env:GOPATH = Join-Path $tools 'gopath'; $env:GOCACHE = Join-Path $tools 'gocache'
  $env:GOTOOLCHAIN = 'local'
} else {
  $goExe = (Get-Command go -ErrorAction Stop).Source
}
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
& $goExe version

# Icon: uses logo-source.png (square-ish PNG) when you place one here, otherwise the placeholder mark.
if (Test-Path 'logo-source.png') { & $goExe run ./tools/mkicon -in logo-source.png -crop 0.075 -round 0.23 -out app.ico -png ui/logo.png }
elseif (-not (Test-Path 'app.ico')) { & $goExe run ./tools/mkicon -out app.ico -png ui/logo.png }

# Windows resources (icon + DPI manifest) -> .syso picked up automatically by go build.
# Uses windres from MinGW (installed locally); no third-party download is needed.
$windres = (Get-Command windres -ErrorAction Stop).Source
& $windres -O coff -F pe-x86-64 -o rsrc_windows_amd64.syso app.rc
if ($LASTEXITCODE -ne 0) { throw 'windres failed' }

# The version comes from store.go so the app, the setup program and the update log agree.
$ver = [regex]::Match((Get-Content 'store.go' -Raw), 'appVersion\s*=\s*"([^"]+)"').Groups[1].Value
if (-not $ver) { throw 'appVersion not found in store.go' }
# Output folders (project root\installers): one folder for the setup program, one for the
# standalone app. Each build replaces the previous file in its folder.
$instRoot   = Join-Path (Split-Path $PSScriptRoot -Parent) 'installers'
$setupDir   = Join-Path $instRoot 'Setup'
$standDir   = Join-Path $instRoot 'Standalone'
if (-not $Out) { $Out = Join-Path $setupDir "KeyClick-Tracker-Setup-v$ver.exe" }
New-Item -ItemType Directory -Force $setupDir, $standDir | Out-Null

# 1. The app. It is embedded in the setup program and also copied to the Standalone folder.
$payload = 'installer\payload\KeyClick-Tracker.exe'
New-Item -ItemType Directory -Force 'installer\payload' | Out-Null
& $goExe build -trimpath -ldflags '-s -w -H windowsgui' -o $payload .
if ($LASTEXITCODE -ne 0) { throw 'go build (app) failed' }
$p = Get-Item $payload
Write-Host ("App   {0:N0} bytes SHA-256 {1}" -f $p.Length, (Get-FileHash $p.FullName -Algorithm SHA256).Hash)
Get-ChildItem $standDir -Filter '*.exe' | Remove-Item -Force
$standOut = Join-Path $standDir "KeyClick-Tracker-v$ver.exe"
Copy-Item $payload $standOut -Force

# 2. The setup program (administrator manifest and icon come from installer\setup.rc).
Push-Location installer
& $windres -O coff -F pe-x86-64 -o rsrc_windows_amd64.syso setup.rc
$wr = $LASTEXITCODE
Pop-Location
if ($wr -ne 0) { throw 'windres (setup) failed' }
& $goExe build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$ver" -o $Out ./installer
if ($LASTEXITCODE -ne 0) { throw 'go build (setup) failed' }
Remove-Item $payload -Force
Get-ChildItem $setupDir -Filter '*.exe' | Where-Object { $_.FullName -ne (Resolve-Path -LiteralPath $Out -ErrorAction SilentlyContinue).Path } | Remove-Item -Force
$f = Get-Item $Out
Write-Host ("Setup {0} ({1:N0} bytes) SHA-256 {2}" -f $f.FullName, $f.Length, (Get-FileHash $f.FullName -Algorithm SHA256).Hash)
$s = Get-Item $standOut
Write-Host ("Standalone {0} ({1:N0} bytes) SHA-256 {2}" -f $s.FullName, $s.Length, (Get-FileHash $s.FullName -Algorithm SHA256).Hash)
