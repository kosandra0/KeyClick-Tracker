//go:build windows

// KeyClick Tracker Setup: installs (or, run as Uninstall.exe, removes) the app.
//
// It closes a running copy cleanly (so counts are saved), installs the program to
// Program Files, adds a Start Menu shortcut and an "Apps and features" entry, registers
// start-at-sign-in, and starts the app. Your data in %LOCALAPPDATA%\KeyScope is never touched.
//
// Switches: /S silent, /DIR=<folder>, /NOSTARTUP, /NOLAUNCH, /NOREGISTER (no shortcut and no
// "Apps and features" entry), /UNINSTALL.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed payload/KeyClick-Tracker.exe
var appExe []byte

// version is set by the build script: -ldflags "-X main.version=2.0.0".
var version = "dev"

const (
	appName    = "KeyClick Tracker"
	exeName    = "KeyClick-Tracker.exe"
	uninstName = "Uninstall.exe"
	uninstKey  = `Software\Microsoft\Windows\CurrentVersion\Uninstall\KeyClickTracker`
	hostClass  = "KeyClickTrackerHost" // hidden host window of the running app
	wmExit     = 0x8000 + 19           // the app's own exit message (same as the tray Exit)
	idOK       = 1                     // MessageBox result for the OK button
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	pEnumThreadWindows = user32.NewProc("EnumThreadWindows")
	pGetClassNameW     = user32.NewProc("GetClassNameW")
	pPostMessageW      = user32.NewProc("PostMessageW")

	silent  bool
	logFile = filepath.Join(os.TempDir(), "KeyClickTracker-Setup.log")
)

func logf(format string, a ...any) {
	line := time.Now().Format("2006-01-02 15:04:05 ") + fmt.Sprintf(format, a...) + "\n"
	if f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		f.WriteString(line)
		f.Close()
	}
}

func box(text string, flags uint32) int32 {
	if silent {
		return idOK
	}
	r, _ := windows.MessageBox(0, windows.StringToUTF16Ptr(text), windows.StringToUTF16Ptr(appName+" Setup"), flags)
	return r
}

func hidden(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return c
}

func fail(err error) {
	logf("FAILED: %v", err)
	box(fmt.Sprintf("%s could not be set up.\n\n%v\n\nA log is at %s", appName, err, logFile), windows.MB_OK|windows.MB_ICONERROR)
	os.Exit(1)
}

// ------------------------------------------------------------ closing a running copy

var (
	foundWindows []uintptr
	enumCallback = syscall.NewCallback(func(h, _ uintptr) uintptr {
		buf := make([]uint16, 64)
		n, _, _ := pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if syscall.UTF16ToString(buf[:n]) == hostClass {
			foundWindows = append(foundWindows, h)
		}
		return 1
	})
)

// runningPIDs lists running copies of the app (any version), never this setup program.
func runningPIDs() []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var pids []uint32
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if strings.HasPrefix(name, "keyclick-tracker") && !strings.Contains(name, "setup") && pe.ProcessID != uint32(os.Getpid()) {
			pids = append(pids, pe.ProcessID)
		}
	}
	return pids
}

func hostWindowsOf(pid uint32) []uintptr {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	foundWindows = nil
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID == pid {
			pEnumThreadWindows.Call(uintptr(te.ThreadID), enumCallback, 0)
		}
	}
	return foundWindows
}

// stopRunning asks every running copy to exit through its own exit path (which saves the
// counts) and waits for it. Only if a copy ignores the request for 20 s is it ended.
func stopRunning() {
	for _, pid := range runningPIDs() {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			logf("cannot open pid %d: %v", pid, err)
			continue
		}
		hosts := hostWindowsOf(pid)
		for _, w := range hosts {
			pPostMessageW.Call(w, wmExit, 0, 0)
		}
		logf("asked pid %d to exit (host windows: %d)", pid, len(hosts))
		if ev, _ := windows.WaitForSingleObject(h, 20000); ev == uint32(windows.WAIT_TIMEOUT) {
			logf("pid %d did not exit in 20 s; ending it", pid)
			_ = windows.TerminateProcess(h, 1)
			windows.WaitForSingleObject(h, 5000)
		}
		windows.CloseHandle(h)
	}
}

// ------------------------------------------------------------------- install steps

func startMenuLink() string {
	return filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`, appName+".lnk")
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func makeShortcut(lnk, target, dir string) error {
	script := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut(%s);$s.TargetPath=%s;$s.WorkingDirectory=%s;$s.IconLocation=%s;$s.Description=%s;$s.Save()`,
		psQuote(lnk), psQuote(target), psQuote(dir), psQuote(target+",0"), psQuote(appName))
	out, err := hidden("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("shortcut: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeFileRetry(path string, data []byte) error {
	var err error
	for i := 0; i < 8; i++ {
		if err = os.WriteFile(path, data, 0755); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return err
}

func register(dir, exe string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	uninst := filepath.Join(dir, uninstName)
	set := func(name, v string) { _ = k.SetStringValue(name, v) }
	set("DisplayName", appName)
	set("DisplayVersion", version)
	set("Publisher", appName)
	set("InstallLocation", dir)
	set("DisplayIcon", exe)
	set("UninstallString", `"`+uninst+`" /UNINSTALL`)
	set("QuietUninstallString", `"`+uninst+`" /UNINSTALL /S`)
	_ = k.SetDWordValue("EstimatedSize", uint32(len(appExe)*2/1024))
	_ = k.SetDWordValue("NoModify", 1)
	_ = k.SetDWordValue("NoRepair", 1)
	return makeShortcut(startMenuLink(), exe, dir)
}

func install(dir string, noStartup, noLaunch, noRegister bool) {
	exe := filepath.Join(dir, exeName)
	self, _ := os.Executable()
	if box(fmt.Sprintf("Install %s %s to:\n%s\n\n"+
		"The app runs with administrator rights so it can count keys, clicks and scrolls in every window, and it starts when you sign in. "+
		"Your existing data is kept.", appName, version, dir), windows.MB_OKCANCEL|windows.MB_ICONINFORMATION) != idOK {
		logf("cancelled by the user")
		os.Exit(1)
	}
	logf("installing %s to %s", version, dir)
	stopRunning()
	if err := os.MkdirAll(dir, 0755); err != nil {
		fail(err)
	}
	if err := writeFileRetry(exe, appExe); err != nil {
		fail(fmt.Errorf("writing %s: %w", exe, err))
	}
	if data, err := os.ReadFile(self); err == nil { // the uninstaller is this same program
		if err := writeFileRetry(filepath.Join(dir, uninstName), data); err != nil {
			logf("warning: could not write the uninstaller: %v", err)
		}
	}
	if !noRegister {
		if err := register(dir, exe); err != nil {
			logf("warning: registration: %v", err)
			box("The program was installed, but the Start Menu shortcut or the Apps and features entry could not be created:\n\n"+err.Error(), windows.MB_OK|windows.MB_ICONWARNING)
		}
	}
	if !noStartup {
		if out, err := hidden(exe, "--register-startup").CombinedOutput(); err != nil {
			logf("warning: start-at-sign-in: %v %s", err, strings.TrimSpace(string(out)))
			box("The program was installed, but start-with-Windows could not be set up. You can turn it on in the app's Settings.", windows.MB_OK|windows.MB_ICONWARNING)
		}
	}
	if !noLaunch {
		c := exec.Command(exe)
		c.Dir = dir
		if err := c.Start(); err != nil {
			fail(fmt.Errorf("starting %s: %w", exe, err))
		}
		logf("started %s (pid %d)", exe, c.Process.Pid)
	}
	logf("install finished")
}

func uninstall(dir string, self string) {
	if box(fmt.Sprintf("Remove %s from this computer?\n\nYour counts and settings in %%LOCALAPPDATA%%\\KeyScope are kept.", appName), windows.MB_OKCANCEL|windows.MB_ICONQUESTION) != idOK {
		os.Exit(1)
	}
	logf("uninstalling from %s", dir)
	stopRunning()
	exe := filepath.Join(dir, exeName)
	if _, err := os.Stat(exe); err == nil {
		_ = hidden(exe, "--unregister-startup").Run()
	}
	_ = os.Remove(startMenuLink())
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, uninstKey)
	if err := os.Remove(exe); err != nil && !os.IsNotExist(err) {
		logf("warning: could not remove %s: %v", exe, err)
	}
	// This program is running from the folder it must delete, so a helper finishes the job.
	script := fmt.Sprintf(`ping 127.0.0.1 -n 3 >nul & del /f /q "%s" & rmdir "%s"`, self, dir)
	_ = hidden("cmd.exe", "/C", script).Start()
	box(appName+" was removed. Your data was kept.", windows.MB_OK|windows.MB_ICONINFORMATION)
}

func main() {
	dir := filepath.Join(os.Getenv("ProgramFiles"), appName)
	var noStartup, noLaunch, noRegister, doUninstall bool
	self, _ := os.Executable()
	if strings.EqualFold(filepath.Base(self), uninstName) {
		doUninstall = true
	}
	for _, a := range os.Args[1:] {
		u := strings.ToUpper(a)
		switch {
		case u == "/S":
			silent = true
		case u == "/UNINSTALL":
			doUninstall = true
		case u == "/NOSTARTUP":
			noStartup = true
		case u == "/NOLAUNCH":
			noLaunch = true
		case u == "/NOREGISTER":
			noRegister = true
		case strings.HasPrefix(u, "/DIR="):
			dir = strings.Trim(a[5:], `"`)
		}
	}
	if doUninstall {
		if d := filepath.Dir(self); strings.EqualFold(filepath.Base(self), uninstName) {
			dir = d
		}
		uninstall(dir, self)
		return
	}
	install(dir, noStartup, noLaunch, noRegister)
}
