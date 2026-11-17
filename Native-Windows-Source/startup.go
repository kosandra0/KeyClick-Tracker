//go:build windows

package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

// Start with Windows.
//
// KeyClick Tracker runs as administrator (see app.manifest) so that it can count input in
// administrator windows too: Windows hides those from normal programs. Windows does not
// start an administrator program from the ordinary "Run" registry list at sign-in, so the
// app registers a Task Scheduler task instead: it runs at sign-in with the highest
// privileges and shows no prompt. Before v1.2.2 a "Run" registry value was used; it is
// still recognised (and replaced) so an existing setting is kept.

const (
	legacyRunKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	legacyRunValue = "KeyClickTracker"
	taskName       = "KeyClick Tracker"
)

// testCopy reports whether this process is a test copy (KEYCLICK_DEV=1). A test copy uses its
// own data folder and must not change anything outside it: not the sign-in task, not the registry.
// Added in v2.0.0 after a test copy repointed the real startup task at a temporary file.
func testCopy() bool { return os.Getenv("KEYCLICK_DEV") == "1" }

// startupOn caches whether start-with-Windows is on, so /api/status never has to run schtasks.
var startupOn atomic.Bool

func startupEnabled() bool { return startupOn.Load() }

func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return c
}

func schtasksPath() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "schtasks.exe")
}

var taskCommandRe = regexp.MustCompile(`(?s)<Command>(.*?)</Command>`)

// taskCommand returns the program the startup task runs, or ok=false when there is no task.
func taskCommand() (string, bool) {
	out, err := hiddenCmd(schtasksPath(), "/Query", "/TN", taskName, "/XML").Output()
	if err != nil {
		return "", false
	}
	out = bytes.ReplaceAll(out, []byte{0}, nil) // tolerate UTF-16 output
	m := taskCommandRe.FindSubmatch(out)
	if m == nil {
		return "", true
	}
	var s string
	if xml.Unmarshal([]byte("<c>"+string(m[1])+"</c>"), &s) != nil {
		s = string(m[1])
	}
	return strings.TrimSpace(s), true
}

func legacyStartupEntry() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, legacyRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(legacyRunValue)
	return err == nil
}

func removeLegacyStartupEntry() {
	k, err := registry.OpenKey(registry.CURRENT_USER, legacyRunKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(legacyRunValue)
}

func taskXML(exe string) string {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	who := ""
	if u, err := user.Current(); err == nil {
		who = u.Username
	}
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Starts ` + appName + ` at sign-in with the privileges it needs to count input in administrator windows.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + esc(who) + `</UserId>
      <Delay>PT5S</Delay>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + esc(who) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>6</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + esc(exe) + `</Command>
      <Arguments>--background</Arguments>
      <WorkingDirectory>` + esc(filepath.Dir(exe)) + `</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`
}

// setStartup turns start-with-Windows on or off. Turning it on needs administrator rights,
// which the app always has (it is started with them).
func setStartup(on bool) error {
	if testCopy() {
		return fmt.Errorf("start with Windows is disabled in test copies")
	}
	if !on {
		if _, exists := taskCommand(); exists {
			if out, err := hiddenCmd(schtasksPath(), "/Delete", "/TN", taskName, "/F").CombinedOutput(); err != nil {
				return fmt.Errorf("could not remove the startup task: %v: %s", err, strings.TrimSpace(string(out)))
			}
		}
		removeLegacyStartupEntry()
		startupOn.Store(false)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	u := utf16.Encode([]rune(taskXML(exe)))
	buf := make([]byte, 0, 2+len(u)*2)
	buf = append(buf, 0xFF, 0xFE) // UTF-16 LE byte-order mark
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	f, err := os.CreateTemp("", "keyclick-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, werr := f.Write(buf)
	f.Close()
	if werr != nil {
		return werr
	}
	if out, err := hiddenCmd(schtasksPath(), "/Create", "/TN", taskName, "/XML", f.Name(), "/F").CombinedOutput(); err != nil {
		return fmt.Errorf("could not create the startup task: %v: %s", err, strings.TrimSpace(string(out)))
	}
	removeLegacyStartupEntry()
	startupOn.Store(true)
	return nil
}

// ensureStartup runs at every launch. It reads the current state and, when start-with-Windows
// is on but points at a program that is no longer this one (for example after the app was
// moved or reinstalled) or still uses the old registry entry, it repairs it.
func ensureStartup() {
	if testCopy() {
		return // a test copy must never repoint the real sign-in task at itself
	}
	exe, _ := os.Executable()
	cmd, haveTask := taskCommand()
	legacy := legacyStartupEntry()
	startupOn.Store(haveTask || legacy)
	if !haveTask && !legacy {
		return
	}
	if haveTask && !legacy && strings.EqualFold(filepath.Clean(cmd), filepath.Clean(exe)) {
		return // already correct
	}
	if err := setStartup(true); err != nil {
		logLine("start-with-Windows repair failed: " + err.Error())
		return
	}
	logLine("start-with-Windows repaired to point at " + exe)
}
