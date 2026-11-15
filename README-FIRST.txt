KEYSCOPE - RESTORED UI PACKAGE
==============================

START HERE
----------
1. Extract this ZIP to a normal folder (do not run from inside the ZIP preview).
2. Double-click KeyScope.exe.
3. The original plain-grey test window has been replaced with a premium,
   dark KeyScope analytics dashboard inspired by your Arena AI preview.
4. Use the system-tray KeyScope icon to reopen the dashboard, pause/resume
   monitoring, or exit the application.
5. If the dashboard does not appear, run KeyScope-Debug.exe and inspect
   %LOCALAPPDATA%\KeyScope\keyscope.log.

WHAT IS INCLUDED
----------------
- KeyScope.exe: Windows 10/11 x64 portable desktop build, with dashboard assets
  embedded into the executable.
- Native-Windows-Source: complete Go monitoring/tray/API source and UI assets.
- Original-Dashboard-Source: the original React/Next.js Arena dashboard source,
  preserved in the package rather than deleted or overwritten.
- UI-RESTORATION-NOTES.txt: explains what changed and what is preserved.

The new portable build does not require Node.js, PostgreSQL or Rust. It uses
Microsoft Edge in app-window mode to render the embedded interface. The native
Windows host does the input counting, local storage and system-tray work.

DATA LOCATION
-------------
%LOCALAPPDATA%\KeyScope\activity.json
%LOCALAPPDATA%\KeyScope\keyscope.log

This is a portable test build, not an installer. Microsoft Edge is used in app
window mode when installed; otherwise the default browser is opened. Windows
runtime behavior still needs to be verified on your PC.
