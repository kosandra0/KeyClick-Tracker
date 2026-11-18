package main

// v2.2.0: make the chosen logo the icon of the program file itself.
//
// Task Manager, the Start menu (including pinned tiles) and the pinned taskbar button all read
// the icon stored inside KeyClick-Tracker.exe, not the icon a running window sets at run time.
// A running program cannot rewrite its own file, so the change is made by a short-lived helper:
// the app copies itself to a temporary file, starts it with "--helper", and quits. The helper
// waits for the app to exit, replaces the icon resources of the installed file (UpdateResource),
// refreshes the Windows icon cache and starts the app again. The same helper also does a plain
// restart (used by Lite mode, whose browser engine options only apply at start).
//
// Only the icon resources of the program file are changed. The statistics are never touched.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	pBeginUpdateResource = kernel32.NewProc("BeginUpdateResourceW")
	pUpdateResource      = kernel32.NewProc("UpdateResourceW")
	pEndUpdateResource   = kernel32.NewProc("EndUpdateResourceW")
	pLoadLibraryExW      = kernel32.NewProc("LoadLibraryExW")
	pFreeLibrary         = kernel32.NewProc("FreeLibrary")
	pEnumResLangs        = kernel32.NewProc("EnumResourceLanguagesW")
	pWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	pSHChangeNotify      = shell32.NewProc("SHChangeNotify")
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	// Resource ids for the icon images. The group icon keeps id 1 (the id app.rc uses), so the
	// first icon of the file stays the application icon; the images get new ids.
	iconImageBase = 100
	iconGroupID   = 1
)

// iconSizes are the image sizes written into the program file.
var iconSizes = []int{16, 24, 32, 48, 64, 256}

// buildIconGroup makes an RT_GROUP_ICON directory for PNG images of the given sizes. The image
// resource ids are iconImageBase, iconImageBase+1, ...
func buildIconGroup(sizes []int, lengths []int) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint16(0))
	binary.Write(&b, binary.LittleEndian, uint16(1)) // icon type
	binary.Write(&b, binary.LittleEndian, uint16(len(sizes)))
	for i, s := range sizes {
		w := byte(s)
		if s >= 256 {
			w = 0 // 0 means 256 in an icon directory
		}
		b.WriteByte(w)
		b.WriteByte(w)
		b.WriteByte(0) // colour count
		b.WriteByte(0) // reserved
		binary.Write(&b, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&b, binary.LittleEndian, uint16(32)) // bit count
		binary.Write(&b, binary.LittleEndian, uint32(lengths[i]))
		binary.Write(&b, binary.LittleEndian, uint16(iconImageBase+i))
	}
	return b.Bytes()
}

// groupIconLang finds the language id the program file uses for its icon group, so the new
// resources replace the old ones instead of being added beside them.
func groupIconLang(exe string) uint16 {
	h, _, _ := pLoadLibraryExW.Call(uintptr(unsafe.Pointer(utf16p(exe))), 0, 0x2) // LOAD_LIBRARY_AS_DATAFILE
	if h == 0 {
		return 0
	}
	defer pFreeLibrary.Call(h)
	var lang uint16
	cb := syscall.NewCallback(func(hMod, typ, name, l, param uintptr) uintptr {
		lang = uint16(l)
		return 0 // the first language is enough
	})
	pEnumResLangs.Call(h, rtGroupIcon, iconGroupID, cb, 0)
	return lang
}

// patchExeIcon replaces the icon of the program file exe with the logo in pngBytes.
// The file must not be running.
func patchExeIcon(exe string, pngBytes []byte) error {
	images := make([][]byte, len(iconSizes))
	lengths := make([]int, len(iconSizes))
	for i, s := range iconSizes {
		d, err := scaleLogo(pngBytes, s)
		if err != nil {
			return err
		}
		images[i], lengths[i] = d, len(d)
	}
	group := buildIconGroup(iconSizes, lengths)
	lang := groupIconLang(exe)
	h, _, e := pBeginUpdateResource.Call(uintptr(unsafe.Pointer(utf16p(exe))), 0)
	if h == 0 {
		return fmt.Errorf("BeginUpdateResource: %v", e)
	}
	for i, d := range images {
		r, _, e := pUpdateResource.Call(h, rtIcon, uintptr(iconImageBase+i), uintptr(lang), uintptr(unsafe.Pointer(&d[0])), uintptr(len(d)))
		if r == 0 {
			pEndUpdateResource.Call(h, 1) // discard
			return fmt.Errorf("UpdateResource image %d: %v", i, e)
		}
	}
	r, _, e := pUpdateResource.Call(h, rtGroupIcon, iconGroupID, uintptr(lang), uintptr(unsafe.Pointer(&group[0])), uintptr(len(group)))
	if r == 0 {
		pEndUpdateResource.Call(h, 1)
		return fmt.Errorf("UpdateResource group: %v", e)
	}
	if r, _, e := pEndUpdateResource.Call(h, 0); r == 0 {
		return fmt.Errorf("EndUpdateResource: %v", e)
	}
	return nil
}

// ----------------------------------------------------------------- what the file carries

// exeIconMarker records which logo was written into which version of the program file. The
// file's size and modification time identify the version: a reinstall or update replaces the
// file, the marker no longer matches, and the app offers to apply the logo again.
type exeIconMarker struct {
	logo  string
	size  int64
	mtime int64
}

func markerPath() string { return filepath.Join(dataDir, "exe-icon.txt") }

func formatMarker(m exeIconMarker) string {
	return fmt.Sprintf("%s|%d|%d", m.logo, m.size, m.mtime)
}

func parseMarker(s string) (exeIconMarker, bool) {
	p := strings.Split(strings.TrimSpace(s), "|")
	if len(p) != 3 {
		return exeIconMarker{}, false
	}
	size, e1 := strconv.ParseInt(p[1], 10, 64)
	mt, e2 := strconv.ParseInt(p[2], 10, 64)
	if e1 != nil || e2 != nil {
		return exeIconMarker{}, false
	}
	return exeIconMarker{logo: p[0], size: size, mtime: mt}, true
}

func readMarker() (exeIconMarker, bool) {
	b, err := os.ReadFile(markerPath())
	if err != nil {
		return exeIconMarker{}, false
	}
	return parseMarker(string(b))
}

func writeMarker(logo, exe string) {
	st, err := os.Stat(exe)
	if err != nil {
		return
	}
	_ = os.WriteFile(markerPath(), []byte(formatMarker(exeIconMarker{logo, st.Size(), st.ModTime().UnixNano()})), 0600)
}

// iconPending decides whether the program file still needs the chosen logo. It is pure so it can
// be tested: want is the chosen logo ("" = default), m is the marker, sameFile tells whether the
// marker belongs to the file as it is now.
func iconPending(want string, m exeIconMarker, haveMarker, sameFile bool) bool {
	if want == "" {
		// The default logo is what a fresh file carries. Only a file that still carries an
		// older custom logo needs resetting.
		return haveMarker && sameFile && m.logo != ""
	}
	return !(haveMarker && sameFile && m.logo == want)
}

func exeIconPending() bool {
	storeMu.RLock()
	want := ""
	if store.Settings != nil {
		want = store.Settings.AppLogo
	}
	storeMu.RUnlock()
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	st, err := os.Stat(exe)
	if err != nil {
		return false
	}
	m, ok := readMarker()
	same := ok && m.size == st.Size() && m.mtime == st.ModTime().UnixNano()
	return iconPending(want, m, ok, same)
}

// ----------------------------------------------------------------- restart through a helper

// startHelperAndExit copies this program to a temporary file, starts that copy as the helper and
// then asks the app to quit (which saves the counts). applyIcon also asks the helper to write the
// chosen logo into the program file.
func startHelperAndExit(applyIcon bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("KeyClickTracker-helper-%d.exe", os.Getpid()))
	if err := copyFile(exe, tmp); err != nil {
		return err
	}
	mode := "restart"
	if applyIcon {
		storeMu.RLock()
		mode = "icon:" + store.Settings.AppLogo
		storeMu.RUnlock()
	}
	cmd := exec.Command(tmp, "--helper", strconv.Itoa(os.Getpid()), exe, mode)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	logLine("restart helper started (" + mode + ")")
	if onExitRequest != nil {
		onExitRequest()
	}
	return nil
}

// cleanHelperCopies removes the temporary helper files left by earlier restarts.
func cleanHelperCopies() {
	files, _ := filepath.Glob(filepath.Join(os.TempDir(), "KeyClickTracker-helper-*.exe"))
	for _, f := range files {
		_ = os.Remove(f) // a copy that is still running cannot be removed; that is fine
	}
}

// runHelper is the "--helper <pid> <exe> <mode>" entry point. mode is "restart" or "icon:<logo>".
func runHelper(args []string) {
	if len(args) < 3 {
		return
	}
	pid, _ := strconv.Atoi(args[0])
	exe, mode := args[1], args[2]
	openDataDir() // opens the log file; the helper never touches the statistics
	logLine("helper: waiting for the app to exit (" + mode + ")")
	if pid > 0 {
		if h, _, _ := pOpenProc.Call(0x00100000, 0, uintptr(pid)); h != 0 { // SYNCHRONIZE
			pWaitForSingleObject.Call(h, 30000)
			pCloseHdl.Call(h)
		}
	}
	time.Sleep(600 * time.Millisecond)
	if strings.HasPrefix(mode, "icon:") {
		logo := strings.TrimPrefix(mode, "icon:")
		var err error
		for try := 0; try < 12; try++ { // the file can stay locked for a moment after the exit
			if err = patchExeIcon(exe, logoPNG(logo)); err == nil {
				break
			}
			time.Sleep(400 * time.Millisecond)
		}
		if err != nil {
			logLine("helper: could not change the icon of the program file: " + err.Error())
		} else {
			writeMarker(logo, exe)
			logLine("helper: icon of the program file changed")
			pSHChangeNotify.Call(0x08000000, 0, 0, 0) // SHCNE_ASSOCCHANGED: refresh Explorer's icons
			c := exec.Command("ie4uinit.exe", "-show")  // refreshes the Start menu icon cache on Windows 10
			c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			_ = c.Run()
		}
	}
	if err := exec.Command(exe).Start(); err != nil {
		logLine("helper: could not start the app: " + err.Error())
	}
}
