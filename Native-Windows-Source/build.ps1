# Builds "KeyClick Tracker.exe" into the parent folder.
# Uses the portable Go in ..\_tools\go when present, otherwise the Go on PATH.
param([string]$Out = '', [switch]$Install)   # -Install: also silently update the installed copy (the one pinned at sign-in)
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

# The version comes from store.go so the app, the setup program and the update log agree.
$ver = [regex]::Match((Get-Content 'store.go' -Raw), 'appVersion\s*=\s*"([^"]+)"').Groups[1].Value
if (-not $ver) { throw 'appVersion not found in store.go' }

# Windows resources (icon + DPI manifest + file version information) -> .syso picked up
# automatically by go build. Uses windres from MinGW (installed locally); no download is needed.
# The version information (what Explorer shows under Properties > Details) is generated from
# store.go and appended to a temporary copy of the .rc file.
$windres = (Get-Command windres -ErrorAction Stop).Source
function Build-Resource([string]$rcFile, [string]$sysoFile, [string]$description, [string]$original) {
  $dir = Split-Path $rcFile -Parent
  $v4 = (($ver -split '\.') + @('0','0','0','0'))[0..3] -join ','
  $block = @"

1 VERSIONINFO
FILEVERSION $v4
PRODUCTVERSION $v4
FILEOS 0x4
FILETYPE 0x1
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "040904B0"
    BEGIN
      VALUE "FileDescription", "$description"
      VALUE "FileVersion", "$ver"
      VALUE "InternalName", "KeyClick-Tracker"
      VALUE "OriginalFilename", "$original"
      VALUE "ProductName", "KeyClick Tracker"
      VALUE "ProductVersion", "$ver"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x409, 1200
  END
END
"@
  $tmp = Join-Path $dir 'zz-build-temp.rc'
  Set-Content -Path $tmp -Value ((Get-Content $rcFile -Raw) + $block) -Encoding ascii
  Push-Location $dir
  try { & $windres -O coff -F pe-x86-64 -o $sysoFile 'zz-build-temp.rc'; $code = $LASTEXITCODE } finally { Pop-Location; Remove-Item $tmp -Force -ErrorAction SilentlyContinue }
  if ($code -ne 0) { throw "windres failed for $rcFile" }
}
Build-Resource (Join-Path $PSScriptRoot 'app.rc') 'rsrc_windows_amd64.syso' 'KeyClick Tracker' 'KeyClick-Tracker.exe'
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
Build-Resource (Join-Path $PSScriptRoot 'installer\setup.rc') 'rsrc_windows_amd64.syso' 'KeyClick Tracker Setup' 'KeyClick-Tracker-Setup.exe'
& $goExe build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$ver" -o $Out ./installer
if ($LASTEXITCODE -ne 0) { throw 'go build (setup) failed' }
Remove-Item $payload -Force
Get-ChildItem $setupDir -Filter '*.exe' | Where-Object { $_.FullName -ne (Resolve-Path -LiteralPath $Out -ErrorAction SilentlyContinue).Path } | Remove-Item -Force
$f = Get-Item $Out
Write-Host ("Setup {0} ({1:N0} bytes) SHA-256 {2}" -f $f.FullName, $f.Length, (Get-FileHash $f.FullName -Algorithm SHA256).Hash)
$s = Get-Item $standOut
Write-Host ("Standalone {0} ({1:N0} bytes) SHA-256 {2}" -f $s.FullName, $s.Length, (Get-FileHash $s.FullName -Algorithm SHA256).Hash)
if ($Install) {
  # Silent update of the installed copy. The setup closes the running copy first (its counts are saved).
  $p2 = Start-Process -FilePath $f.FullName -ArgumentList '/S' -PassThru
  Write-Host "Installing silently (setup pid $($p2.Id)). Check %TEMP%\KeyClickTracker-Setup.log."
}
