@echo off
setlocal
cd /d "%~dp0"
echo Building KeyClick Tracker for Windows x64...
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1"
if errorlevel 1 (
  echo Build failed. Review the messages above.
  pause
  exit /b 1
)
echo.
echo Build finished. The setup program is KeyClick-Tracker-Setup-v(version).exe in the parent folder.
pause
