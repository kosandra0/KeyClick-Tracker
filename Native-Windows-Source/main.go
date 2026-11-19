//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	pRegisterClassExW        = user32.NewProc("RegisterClassExW")
	pCreateWindowExW         = user32.NewProc("CreateWindowExW")
	pDefWindowProcW          = user32.NewProc("DefWindowProcW")
	pGetMessageW             = user32.NewProc("GetMessageW")
	pTranslateMessage        = user32.NewProc("TranslateMessage")
	pDispatchMessageW        = user32.NewProc("DispatchMessageW")
	pPostQuitMessage         = user32.NewProc("PostQuitMessage")
	pPostMessageW            = user32.NewProc("PostMessageW")
	pSendMessageW            = user32.NewProc("SendMessageW")
	pLoadCursorW             = user32.NewProc("LoadCursorW")
	pLoadImageW              = user32.NewProc("LoadImageW")
	pDestroyWindow           = user32.NewProc("DestroyWindow")
	pSetTimer                = user32.NewProc("SetTimer")
	pRegisterRawInputDevices = user32.NewProc("RegisterRawInputDevices")
	pGetRawInputData         = user32.NewProc("GetRawInputData")
	pGetCursorPos            = user32.NewProc("GetCursorPos")
	pCreatePopupMenu         = user32.NewProc("CreatePopupMenu")
	pAppendMenuW             = user32.NewProc("AppendMenuW")
	pTrackPopupMenu          = user32.NewProc("TrackPopupMenu")
	pDestroyMenu             = user32.NewProc("DestroyMenu")
	pSetForegroundWindow     = user32.NewProc("SetForegroundWindow")
	pMessageBoxW             = user32.NewProc("MessageBoxW")
	pFindWindowW             = user32.NewProc("FindWindowW")
	pShowWindow              = user32.NewProc("ShowWindow")
	pSetWindowLongPtrW       = user32.NewProc("SetWindowLongPtrW")
	pGetWindowLongPtrW       = user32.NewProc("GetWindowLongPtrW")
	pCallWindowProcW         = user32.NewProc("CallWindowProcW")
	pSetWindowPos            = user32.NewProc("SetWindowPos")
	pGetWindowRect           = user32.NewProc("GetWindowRect")
	pSetWindowRgn            = user32.NewProc("SetWindowRgn")
	pReleaseCapture          = user32.NewProc("ReleaseCapture")
	pIsZoomed                = user32.NewProc("IsZoomed")
	pIsWindowVisible         = user32.NewProc("IsWindowVisible")
	pGetDpiForWindow         = user32.NewProc("GetDpiForWindow")
	pGetSystemMetrics        = user32.NewProc("GetSystemMetrics")
	pGetSystemMetricsForDp   = user32.NewProc("GetSystemMetricsForDpi")
	pCreateRoundRectRgn      = gdi32.NewProc("CreateRoundRectRgn")
	pGetModuleHandleW        = kernel32.NewProc("GetModuleHandleW")
	pGetLastError            = kernel32.NewProc("GetLastError")
	pCreateMutexW            = kernel32.NewProc("CreateMutexW")
	pShellNotifyIconW        = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW           = shell32.NewProc("ShellExecuteW")
)

const (
	wmInput         = 0x00FF // WM_INPUT: Raw Input message
	ridInput        = 0x10000003
	rimTypeMouse    = 0
	rimTypeKeyboard = 1
	usageKeyboard   = 6
	usageMouse      = 2
	ridevInputSink  = 0x00000100
	ridevRemove     = 0x00000001
	riKeyBreak      = 0x01 // RI_KEY_BREAK: key release
	rmbLeftDown     = 0x0001
	rmbRightDown    = 0x0004
	rmbMiddleDown   = 0x0010
	rmbX1Down       = 0x0040
	rmbX2Down       = 0x0100
	rmbWheel        = 0x0400
	rmbHWheel       = 0x0800
	wmDestroy       = 0x0002
	wmTimer         = 0x0113
	wmSize          = 0x0005
	wmClose         = 0x0010
	wmEndSession    = 0x0016
	wmGetMinMax     = 0x0024
	wmNCCalcSize    = 0x0083
	wmNCLButton     = 0x00A1
	wmSetIcon       = 0x0080
	wmApp           = 0x8000
	wmTray          = wmApp + 17
	wmOpen          = wmApp + 18
	wmExit          = wmApp + 19
	wmLBtnDbl       = 0x0203
	wmRButtonUp     = 0x0205
	wmContextMen    = 0x007B

	nimAdd, nimModify, nimDelete = 0, 1, 2
	nifMessage, nifIcon, nifTip  = 1, 2, 4
	nifInfo                      = 0x10
	niifInfo                     = 1
	mfString, mfSeparator        = 0, 0x800
	tpmRightButton, tpmReturnCmd = 0x2, 0x100
	swHide, swShow, swNormal     = 0, 5, 1
	swMinimize, swMaximize       = 6, 3
	swRestore                    = 9

	gwlStyle        = -16
	gwlpWndProc     = -4
	wsThickFrame    = 0x00040000
	wsSysMenu       = 0x00080000
	wsMinimizeBox   = 0x00020000
	wsMaximizeBox   = 0x00010000
	wsClipChildren  = 0x02000000
	wsVisible       = 0x10000000
	wsOverlapped    = 0x00CF0000
	swpNoSize       = 0x1
	swpNoMove       = 0x2
	swpNoZOrder     = 0x4
	swpFrameChanged = 0x20
	swpShowWindow   = 0x40
	sizeMaximized   = 2
	sizeMinimized   = 1
	cornerRadiusDIP = 20
	iconResourceID  = 1
)

const (
	menuOpen   = 501
	menuToggle = 502
	menuExit   = 503
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}
type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  uintptr
}
type keyboardLL struct {
	VKCode, ScanCode, Flags, Time uint32
	ExtraInfo                     uintptr
}
type mouseLL struct {
	Pt        point
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}
type minMaxInfo struct{ Reserved, MaxSize, MaxPosition, MinTrack, MaxTrack point }
type notifyIconData struct {
	Size             uint32
	HWnd             uintptr
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUID             [16]byte
	BalloonIcon      uintptr
}

var (
	hostHWND, mainHWND, oldMainProc uintptr
	appInstance                     uintptr
	trayAdded                       bool
	hostCallback, mainCallback      uintptr
	pausedFlag                      atomic.Bool
	rawOK                           atomic.Bool          // true while Raw Input is registered (reported as "recording")
	rawCallback, inputWnd           uintptr              // input thread only: window procedure and hidden window
	pressedAt                       = map[uint32]int64{} // hook thread only: last key-down time per key
	hookEvents                      = make(chan hookEvent, 8192)
	exiting                         bool
	view                            webview2.WebView
	startHidden                     bool
	lastEvent                       atomic.Int64 // unix nanoseconds of the last recorded input
)

func utf16p(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func fixed(dst []uint16, s string) {
	v, _ := syscall.UTF16FromString(s)
	copy(dst, v)
	dst[len(dst)-1] = 0
}
func isPaused() bool { return pausedFlag.Load() }
func setPaused(v bool) {
	pausedFlag.Store(v)
	if !v {
		resetIdleAnchor() // v2.0.0: time spent paused is not idle time
	}
}
func hooksOK() bool { return rawOK.Load() }
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func openFolder(dir string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16p("open"))), uintptr(unsafe.Pointer(utf16p("explorer.exe"))), uintptr(unsafe.Pointer(utf16p(dir))), 0, swShow)
}

// openContactLink opens only the contact links shown in About, in the default browser.
func openContactLink(u string) {
	allowed := []string{"https://www.instagram.com/", "https://t.me/"}
	for _, p := range allowed {
		if strings.HasPrefix(u, p) && len(u) < 200 && !strings.ContainsAny(u, " \t\r\n\"") {
			pShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16p("open"))), uintptr(unsafe.Pointer(utf16p(u))), 0, 0, swNormal)
			return
		}
	}
}

// ----------------------------------------------------------------- key naming

func keyName(vk, scan, flags uint32) string {
	switch {
	case vk == 0x10:
		if scan == 0x36 {
			return "ShiftRight"
		}
		return "ShiftLeft"
	case vk == 0x11:
		if flags&1 != 0 {
			return "ControlRight"
		}
		return "ControlLeft"
	case vk == 0x12:
		if flags&1 != 0 {
			return "AltRight"
		}
		return "AltLeft"
	case vk >= 0x41 && vk <= 0x5A, vk >= 0x30 && vk <= 0x39:
		return string(rune(vk))
	case vk >= 0x60 && vk <= 0x69:
		return fmt.Sprintf("Numpad%d", vk-0x60)
	case vk >= 0x70 && vk <= 0x87:
		return fmt.Sprintf("F%d", vk-0x6F)
	}
	m := map[uint32]string{0x08: "Backspace", 0x09: "Tab", 0x0D: "Enter", 0x13: "Pause", 0x14: "CapsLock", 0x1B: "Escape", 0x20: "Space", 0x21: "PageUp", 0x22: "PageDown", 0x23: "End", 0x24: "Home", 0x25: "ArrowLeft", 0x26: "ArrowUp", 0x27: "ArrowRight", 0x28: "ArrowDown", 0x2C: "PrintScreen", 0x2D: "Insert", 0x2E: "Delete", 0x5B: "MetaLeft", 0x5C: "MetaRight", 0x5D: "ContextMenu", 0x6A: "NumpadMultiply", 0x6B: "NumpadAdd", 0x6D: "NumpadSubtract", 0x6E: "NumpadDecimal", 0x6F: "NumpadDivide", 0x90: "NumLock", 0x91: "ScrollLock", 0xA0: "ShiftLeft", 0xA1: "ShiftRight", 0xA2: "ControlLeft", 0xA3: "ControlRight", 0xA4: "AltLeft", 0xA5: "AltRight", 0xBA: "Semicolon", 0xBB: "Equal", 0xBC: "Comma", 0xBD: "Minus", 0xBE: "Period", 0xBF: "Slash", 0xC0: "Backquote", 0xDB: "BracketLeft", 0xDC: "Backslash", 0xDD: "BracketRight", 0xDE: "Quote", 0xE2: "IntlBackslash"}
	if s, ok := m[vk]; ok {
		return s
	}
	return fmt.Sprintf("VK_%02X", vk)
}

// ------------------------------------------------------------- input (Raw Input)

// Input is read with Raw Input (RegisterRawInputDevices) instead of the low-level
// keyboard/mouse hooks. Raw Input is delivered to a hidden window for keyboard and mouse
// input from every application, whatever has focus, and Windows never removes it for
// being slow. The old WH_KEYBOARD_LL / WH_MOUSE_LL hooks could be silently dropped by
// Windows when a callback ran past its timeout, which stopped counting until restart.
// Raw Input does not tell injected input (SendInput) apart from physical input, so
// injected events are counted too (deliberate change in v1.2.1).

// rawDevice is RAWINPUTDEVICE (usage page/usage, flags, target window).
type rawDevice struct {
	UsagePage, Usage uint16
	Flags            uint32
	Target           uintptr
}

// rawHeader is RAWINPUTHEADER; the device data follows it.
type rawHeader struct {
	Type, Size uint32
	Device     uintptr
	Param      uintptr
}

// rawKeyboard is the RAWKEYBOARD part of RAWINPUT.
type rawKeyboard struct {
	MakeCode, Flags, Reserved, VKey uint16
	Message, Extra                  uint32
}

// rawMouse is the RAWMOUSE part of RAWINPUT (ButtonFlags/ButtonData share the union slot).
type rawMouse struct {
	Flags       uint16
	_           uint16
	ButtonFlags uint16
	ButtonData  uint16
	RawButtons  uint32
	LastX       int32
	LastY       int32
	Extra       uint32
}

// hookEvent is one physical input, queued by the input handler and recorded by recorder().
type hookEvent struct {
	kind, id string
	units    uint64
	// v2.4.0: when the input happened (monotonic), the foreground program then, and a sub-kind.
	app string
	t   time.Time
	sub string
}

// queueEvent never blocks: the input handler must return quickly, and recorder() does the work.
func queueEvent(kind, id string, units uint64) {
	select {
	case hookEvents <- hookEvent{kind: kind, id: id, units: units, app: appNow(), t: time.Now()}:
	default: // queue full (should never happen): drop rather than stall
	}
}

// recorder drains the input queue and records each event. A 250 ms tick also closes idle
// scroll sessions and drops stuck holds (v2.4.0).
func recorder() {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case ev := <-hookEvents:
			processEvent(ev)
		case <-tick.C:
			now := time.Now()
			sweepHolds(now)
			closeStaleScroll(now)
		}
	}
}

func processEvent(ev hookEvent) {
	if isPaused() || quietNow() {
		return
	}
	if ev.kind == "shortcut" || ev.kind == "modkey" { // F4: counted on the day, not as extra keys
		recordCombo(ev.kind)
		return
	}
	if ev.kind == "hold" { // v2.4.0: a finished key or button hold
		recordHold(ev)
		return
	}
	record(ev.kind, ev.id, ev.units)
	if ev.kind == "scroll" {
		scrollEvent(ev)
	}
	lastEvent.Store(time.Now().UnixNano())
}

// repeatGapNS: a key that is still held sends auto-repeat key-downs well inside this
// gap. A key-down after a longer pause is a new press, even if its release was missed.
const repeatGapNS = int64(1500 * time.Millisecond)

// handleKey counts a key-down as a new press unless it is auto-repeat of a key still held.
// Runs on the input thread only (pressedAt is not locked).
// mods is the held-modifier state for shortcut counting (F4). Input thread only.
var mods modState

func handleKey(vk, scan uint32, down bool, ext bool) {
	t := time.Now()
	isMod := mods.apply(vk, ext, down, t)
	if !down {
		_, sawDown := pressedAt[vk]
		delete(pressedAt, vk)
		if !isMod || countedModifier(vk) || vk == 0x10 || vk == 0xA0 || vk == 0xA1 {
			holdUp("k", keyNameFor(vk, scan, ext)) // v2.4.0: measured hold ends
		}
		// v2.3.0 fix: with Win or Alt held, Windows consumes many shortcut keys itself (Win+2,
		// Win+T, Alt+Tab ...) and Raw Input then delivers only the key-UP, never the key-down.
		// Those keys were lost. A key-up with no counted key-down, while Win or Alt is held,
		// is therefore the press. Ctrl and Shift never lose key-downs, so they are untouched.
		if !isMod && !sawDown && mods.winOrAltHeld() {
			queuePress(vk, scan, ext, false)
		}
		return
	}
	now := t.UnixNano()
	last, held := pressedAt[vk]
	if !held || now-last > repeatGapNS {
		queuePress(vk, scan, ext, isMod)
		holdDown("k", keyNameFor(vk, scan, ext)) // v2.4.0: measured hold starts (auto-repeat does not restart it)
	}
	pressedAt[vk] = now
}

// queuePress queues the events for one counted key press (the key, and the shortcut or
// modifier-key marker). Input thread only.
func keyNameFor(vk, scan uint32, ext bool) string {
	var flags uint32
	if ext && (vk == 0x11 || vk == 0x12) {
		flags = 1
	}
	return keyName(vk, scan, flags)
}

func queuePress(vk, scan uint32, ext, isMod bool) {
	// v2.0.0: the extended-key flag tells right Ctrl/Alt from left. It was never passed
	// before, so right-hand Ctrl and Alt were counted as the left-hand keys.
	var flags uint32
	if ext && (vk == 0x11 || vk == 0x12) {
		flags = 1
	}
	queueEvent("key", keyName(vk, scan, flags), 0)
	if isMod {
		if countedModifier(vk) {
			queueEvent("modkey", "", 0)
		}
	} else if mods.isShortcut() {
		queueEvent("shortcut", "", 0)
	}
}

// handleMouse queues one event per button press and one per wheel step group.
func handleMouse(m *rawMouse) {
	f := m.ButtonFlags
	// v2.4.0: button holds. Down starts a hold, up ends it (drags keep one hold).
	for _, b := range [...]struct {
		down, up uint16
		id       string
	}{{rmbLeftDown, 0x0002, "left"}, {rmbRightDown, 0x0008, "right"}, {rmbMiddleDown, 0x0020, "middle"}, {rmbX1Down, 0x0080, "x1"}, {rmbX2Down, 0x0200, "x2"}} {
		if f&b.down != 0 {
			holdDown("m", b.id)
		}
		if f&b.up != 0 {
			holdUp("m", b.id)
		}
	}
	if f&rmbLeftDown != 0 {
		queueEvent("mouse", "left", 0)
	}
	if f&rmbRightDown != 0 {
		queueEvent("mouse", "right", 0)
	}
	if f&rmbMiddleDown != 0 {
		queueEvent("mouse", "middle", 0)
	}
	if f&rmbX1Down != 0 {
		queueEvent("mouse", "x1", 0)
	}
	if f&rmbX2Down != 0 {
		queueEvent("mouse", "x2", 0)
	}
	if f&rmbWheel != 0 {
		d := int16(m.ButtonData)
		id := ""
		switch {
		case d > 0:
			id = "up"
		case d < 0:
			id = "down"
		}
		if id != "" {
			queueEvent("scroll", id, uint64(abs(int(d))/120))
		}
	}
	if f&rmbHWheel != 0 {
		d := int16(m.ButtonData)
		id := ""
		switch {
		case d > 0:
			id = "right"
		case d < 0:
			id = "left"
		}
		if id != "" {
			queueEvent("scroll", id, uint64(abs(int(d))/120))
		}
	}
}

// rawBuf is reused for every WM_INPUT. Only the input thread touches it.
var rawBuf [256]byte

// handleRawInput decodes one WM_INPUT message (lParam is the HRAWINPUT handle).
func handleRawInput(lp uintptr) {
	size := uint32(len(rawBuf))
	n, _, _ := pGetRawInputData.Call(lp, ridInput, uintptr(unsafe.Pointer(&rawBuf[0])), uintptr(unsafe.Pointer(&size)), unsafe.Sizeof(rawHeader{}))
	if int32(n) <= 0 {
		return
	}
	p := unsafe.Pointer(&rawBuf[0])
	hdr := (*rawHeader)(p)
	data := unsafe.Add(p, unsafe.Sizeof(rawHeader{}))
	if isPaused() {
		return
	}
	switch hdr.Type {
	case rimTypeKeyboard:
		k := (*rawKeyboard)(data)
		if k.VKey == 0 || k.VKey == 0xFF {
			return
		}
		handleKey(uint32(k.VKey), uint32(k.MakeCode), k.Flags&riKeyBreak == 0, k.Flags&2 != 0) // 2 = RI_KEY_E0 (extended key)
	case rimTypeMouse:
		handleMouse((*rawMouse)(data))
	}
}

// rawProc is the window procedure of the hidden input window.
func rawProc(h, msg, wp, lp uintptr) uintptr {
	if msg == wmInput {
		handleRawInput(lp)
	}
	r, _, _ := pDefWindowProcW.Call(h, msg, wp, lp)
	return r
}

// registerRawInput asks Windows for keyboard and mouse input sent to target.
// RIDEV_INPUTSINK keeps delivering input while the window is in the background or the tray.
func registerRawInput(target uintptr) (bool, error) {
	devs := [2]rawDevice{
		{UsagePage: 1, Usage: usageKeyboard, Flags: ridevInputSink, Target: target},
		{UsagePage: 1, Usage: usageMouse, Flags: ridevInputSink, Target: target},
	}
	r, _, err := pRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&devs[0])), 2, unsafe.Sizeof(devs[0]))
	if r == 0 {
		return false, err
	}
	return true, nil
}

// unregisterRawInput stops keyboard and mouse delivery (RIDEV_REMOVE needs no window).
func unregisterRawInput() {
	devs := [2]rawDevice{
		{UsagePage: 1, Usage: usageKeyboard, Flags: ridevRemove},
		{UsagePage: 1, Usage: usageMouse, Flags: ridevRemove},
	}
	pRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&devs[0])), 2, unsafe.Sizeof(devs[0]))
}

// removeHooks stops input capture. Name kept from the hook version; it is called at shutdown.
func removeHooks() {
	unregisterRawInput()
	rawOK.Store(false)
}

// inputThread owns the input window: it creates it, registers for Raw Input, pumps its own
// message loop (so the UI thread cannot delay input) and re-registers every few seconds
// as a safety net. Its status is reported as "recording" by /api/status.
func inputThread() {
	runtime.LockOSThread()
	instance := appInstance
	className := utf16p("KeyClickRawInput")
	rawCallback = syscall.NewCallback(rawProc)
	wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: rawCallback, Instance: instance, ClassName: className}
	if r, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		logLine("Input setup warning: RegisterClassExW failed")
		return
	}
	hwndMessage := ^uintptr(2) // HWND_MESSAGE (-3): a message-only window, never visible
	inputWnd, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, instance, 0)
	if inputWnd == 0 {
		logLine("Input setup warning: CreateWindowExW failed")
		return
	}
	ok, regErr := registerRawInput(inputWnd)
	rawOK.Store(ok)
	if !ok {
		logLine(fmt.Sprintf("Input setup warning: RegisterRawInputDevices failed: %v", regErr))
	}
	pSetTimer.Call(0, 1, 3000, 0)
	var m winMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.Message == wmTimer {
			ok, _ := registerRawInput(inputWnd)
			rawOK.Store(ok)
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// ------------------------------------------------------------------ tray icon

func loadIcon(size int) uintptr {
	h, _, _ := pLoadImageW.Call(appInstance, iconResourceID, 1, uintptr(size), uintptr(size), 0)
	return h
}

func addTrayIcon() {
	if trayAdded {
		return
	}
	nid := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: hostHWND, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: loadIcon(16)}
	fixed(nid.Tip[:], appName+" · private input analytics")
	r, _, _ := pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	trayAdded = r != 0
}
func deleteTrayIcon() {
	if !trayAdded {
		return
	}
	nid := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: hostHWND, ID: 1}
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	trayAdded = false
}
func trayNotify(title, msg string) {
	if !trayAdded {
		return
	}
	nid := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: hostHWND, ID: 1, Flags: nifInfo, InfoFlags: niifInfo}
	fixed(nid.Info[:], msg)
	fixed(nid.InfoTitle[:], title)
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

// showNativeTrayMenu is the plain Windows menu. It is only used when the themed menu
// (traymenu.go) cannot be created.
func showNativeTrayMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	pAppendMenuW.Call(m, mfString, menuOpen, uintptr(unsafe.Pointer(utf16p("Open "+appName))))
	label := "Pause monitoring"
	if isPaused() {
		label = "Resume monitoring"
	}
	pAppendMenuW.Call(m, mfString, menuToggle, uintptr(unsafe.Pointer(utf16p(label))))
	storeMu.RLock()
	liveOn := store.Settings != nil && store.Settings.OverlayOn
	storeMu.RUnlock()
	liveFlags := uintptr(mfString)
	if liveOn {
		liveFlags |= 0x8 // MF_CHECKED
	}
	pAppendMenuW.Call(m, liveFlags, menuLive, uintptr(unsafe.Pointer(utf16p("Floating live box"))))
	pAppendMenuW.Call(m, mfSeparator, 0, 0)
	pAppendMenuW.Call(m, mfString, menuExit, uintptr(unsafe.Pointer(utf16p("Exit "+appName))))
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	pSetForegroundWindow.Call(hostHWND)
	cmd, _, _ := pTrackPopupMenu.Call(m, tpmRightButton|tpmReturnCmd, uintptr(p.X), uintptr(p.Y), 0, hostHWND, 0)
	pDestroyMenu.Call(m)
	switch cmd {
	case menuOpen:
		showMainWindow()
	case menuToggle:
		setPaused(!isPaused())
	case menuLive:
		toggleLiveBox()
	case menuExit:
		shutdown()
	}
}

// --------------------------------------------------------------- main window

func dpiOf(h uintptr) int {
	if d, _, _ := pGetDpiForWindow.Call(h); d != 0 {
		return int(d)
	}
	return 96
}

func applyRoundedRegion(h uintptr) {
	if h == 0 {
		return
	}
	if z, _, _ := pIsZoomed.Call(h); z != 0 {
		pSetWindowRgn.Call(h, 0, 1)
		return
	}
	var r rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	d := cornerRadiusDIP * 2 * dpiOf(h) / 96
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(r.Right-r.Left+1), uintptr(r.Bottom-r.Top+1), uintptr(d), uintptr(d))
	pSetWindowRgn.Call(h, rgn, 1) // the system owns the region after this call
}

func jsEval(js string) {
	if view != nil {
		view.Dispatch(func() { view.Eval(js) })
	}
}

func showMainWindow() {
	if mainHWND == 0 {
		return
	}
	if ic, _, _ := pIsWindowVisible.Call(mainHWND); ic == 0 {
		pShowWindow.Call(mainHWND, swShow)
	}
	pShowWindow.Call(mainHWND, swRestore)
	pSetForegroundWindow.Call(mainHWND)
	setUIVisible(true)
}

func mainProc(h, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case wmNCCalcSize:
		if wp != 0 {
			// Remove the system frame entirely. When maximized, Windows places the window
			// a few pixels past the screen edge, so pull the client area back in.
			if z, _, _ := pIsZoomed.Call(h); z != 0 {
				r := (*rect)(unsafe.Pointer(lp))
				dpi := uintptr(dpiOf(h))
				f, _, _ := pGetSystemMetricsForDp.Call(32, dpi)
				p, _, _ := pGetSystemMetricsForDp.Call(92, dpi)
				b := int32(f + p)
				r.Left += b
				r.Top += b
				r.Right -= b
				r.Bottom -= b
			}
			return 0
		}
	case wmGetMinMax:
		mm := (*minMaxInfo)(unsafe.Pointer(lp))
		s := int32(dpiOf(h))
		mm.MinTrack = point{X: 940 * s / 96, Y: 620 * s / 96}
	case wmClose:
		storeMu.RLock()
		toTray := store.CloseToTray
		storeMu.RUnlock()
		if toTray && trayAdded {
			pShowWindow.Call(h, swHide)
			setUIVisible(false) // v2.2.0: the page stops animating while hidden
			return 0
		}
		shutdown()
		return 0
	case wmSize:
		setUIVisible(wp != sizeMinimized) // v2.2.0: a minimised window does not need to animate
		r, _, _ := pCallWindowProcW.Call(oldMainProc, h, msg, wp, lp)
		if wp != sizeMinimized {
			applyRoundedRegion(h)
			if wp == sizeMaximized {
				jsEval("document.documentElement.classList.add('maxed')")
			} else {
				jsEval("document.documentElement.classList.remove('maxed')")
			}
		}
		return r
	}
	r, _, _ := pCallWindowProcW.Call(oldMainProc, h, msg, wp, lp)
	return r
}

func frameless(h uintptr) {
	style := uintptr(wsThickFrame | wsSysMenu | wsMinimizeBox | wsMaximizeBox | wsClipChildren | wsVisible)
	pSetWindowLongPtrW.Call(h, ^uintptr(-gwlStyle-1), style)
	mainCallback = syscall.NewCallback(mainProc)
	oldMainProc, _, _ = pSetWindowLongPtrW.Call(h, ^uintptr(-gwlpWndProc-1), mainCallback)
	dpi := dpiOf(h)
	w, hh := 1280*dpi/96, 820*dpi/96
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	if w > int(sw)-40 {
		w = int(sw) - 40
	}
	if hh > int(sh)-80 {
		hh = int(sh) - 80
	}
	pSetWindowPos.Call(h, 0, uintptr((int(sw)-w)/2), uintptr((int(sh)-hh)/2), uintptr(w), uintptr(hh), swpNoZOrder|swpFrameChanged)
	for _, kind := range []uintptr{0, 1} { // small + big icon
		sz := 16 + 16*kind
		pSendMessageW.Call(h, wmSetIcon, kind, loadIcon(int(sz)))
	}
	applyRoundedRegion(h)
}

func bindWindowControls() {
	post := func(f func()) { view.Dispatch(f) }
	_ = view.Bind("kcMinimize", func() { post(func() { pShowWindow.Call(mainHWND, swMinimize) }) })
	_ = view.Bind("kcMaximize", func() {
		post(func() {
			if z, _, _ := pIsZoomed.Call(mainHWND); z != 0 {
				pShowWindow.Call(mainHWND, swRestore)
			} else {
				pShowWindow.Call(mainHWND, swMaximize)
			}
		})
	})
	_ = view.Bind("kcClose", func() { post(func() { pPostMessageW.Call(mainHWND, wmClose, 0, 0) }) })
	_ = view.Bind("kcDrag", func() {
		post(func() {
			pReleaseCapture.Call()
			pPostMessageW.Call(mainHWND, wmNCLButton, 2, 0) // HTCAPTION
		})
	})
	edges := map[string]uintptr{"w": 10, "e": 11, "n": 12, "nw": 13, "ne": 14, "s": 15, "sw": 16, "se": 17}
	_ = view.Bind("kcResize", func(edge string) {
		if ht, ok := edges[edge]; ok {
			post(func() {
				pReleaseCapture.Call()
				pPostMessageW.Call(mainHWND, wmNCLButton, ht, 0)
			})
		}
	})
	_ = view.Bind("kcNotify", func(title, msg string) { trayNotify(title, msg) })
	_ = view.Bind("kcOpen", func(u string) { openContactLink(u) })
}

// ------------------------------------------------------------- host / lifecycle

func hostProc(h, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case wmTray:
		ev := uint32(lp & 0xFFFF)
		if ev == wmLBtnDbl || ev == wmTrayLBtnUp { // v2.0.0: a single left click opens the app
			showMainWindow()
			return 0
		}
		if ev == wmRButtonUp || ev == wmContextMen {
			showTrayMenu()
			return 0
		}
	case wmOpen:
		showMainWindow()
		return 0
	case wmExit:
		shutdown()
		return 0
	case wmEndSession:
		if wp != 0 {
			flushForExit() // Windows is logging off or shutting down: make sure counts are on disk
		}
		return 0
	case wmHotkey: // v2.0.0 global hotkeys (features.go)
		onHotkey(wp)
		return 0
	case wmPowerBroadcast: // v2.0.0: resume from sleep resets the idle anchor
		onPowerBroadcast(wp)
		return 1
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(h, msg, wp, lp)
	return r
}

func createHostWindow(instance uintptr) error {
	className := utf16p("KeyClickTrackerHost")
	cursor, _, _ := pLoadCursorW.Call(0, 32512)
	hostCallback = syscall.NewCallback(hostProc)
	wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: hostCallback, Instance: instance, Icon: loadIcon(32), IconSmall: loadIcon(16), Cursor: cursor, Background: 6, ClassName: className}
	if r, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassExW failed")
	}
	hostHWND, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(utf16p(appName+" background host"))), wsOverlapped, 0, 0, 100, 100, 0, 0, instance, 0)
	if hostHWND == 0 {
		return fmt.Errorf("CreateWindowExW failed")
	}
	return nil
}

func shutdown() {
	if exiting {
		return
	}
	exiting = true
	logLine("Shutting down " + appName)
	flushForExit()
	deleteTrayIcon()
	removeHooks()
	if mainHWND != 0 {
		pDestroyWindow.Call(mainHWND)
	}
	if hostHWND != 0 {
		pDestroyWindow.Call(hostHWND)
	}
	pPostQuitMessage.Call(0)
}

func msgBox(text, title string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16p(text))), uintptr(unsafe.Pointer(utf16p(title))), 0x10)
}

func main() {
	runtime.LockOSThread()
	if len(os.Args) > 1 && os.Args[1] == "--helper" { // v2.2.0: restart / icon helper (iconpatch.go)
		runHelper(os.Args[2:])
		return
	}
	for _, a := range os.Args[1:] {
		switch a {
		case "--background":
			startHidden = true
		case "--register-startup", "--unregister-startup": // used by the installer and uninstaller
			if err := setStartup(a == "--register-startup"); err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	// Only one instance may count input, otherwise every key would be counted twice.
	mutexName := `Local\KeyClickTrackerSingleton`
	if os.Getenv("KEYCLICK_DEV") == "1" && os.Getenv("KEYCLICK_DEV_MUTEX") != "" {
		mutexName += "-" + os.Getenv("KEYCLICK_DEV_MUTEX") // test only: lets a test copy run beside the installed app
	}
	name := utf16p(mutexName)
	// The error comes from this same call. A separate GetLastError call can read a value
	// that Go's runtime has already changed, which let a second copy run and double-count.
	_, _, mErr := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if errno, _ := mErr.(syscall.Errno); errno == 183 { // ERROR_ALREADY_EXISTS
		if h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(utf16p("KeyClickTrackerHost"))), 0); h != 0 {
			pPostMessageW.Call(h, wmOpen, 0, 0)
		}
		return
	}

	loadStore()
	bootStep("statistics file loaded")
	applyLiteEngine()         // v2.2.0: Lite mode's engine options, before any WebView2 window exists
	go cleanHelperCopies()    // v2.2.0: removes temporary files left by earlier restarts
	go appWatcher()    // v2.0.0 (F1): focused-program tracking (apps.go)
	go goalWatcher()   // v2.0.0 (F7): goals in force per day (features2.go)
	go backupWatcher() // v2.0.0 (F9): a backup copy every six hours (features2.go)
	go reportWatcher() // v2.0.0 (F3): scheduled summaries (api2.go)
	go strainWatcher() // v2.0.0 (F6): optional busy-stretch note (api2.go)
	logLine(appName + " " + appVersion + " starting")
	go ensureStartup() // keeps start-with-Windows pointing at this program
	storeMu.RLock()
	hide := startHidden
	storeMu.RUnlock()
	instance, _, _ := pGetModuleHandleW.Call(0)
	appInstance = instance
	logLine(fmt.Sprintf("icon resource handles: 16px=%t 32px=%t", loadIcon(16) != 0, loadIcon(32) != 0))
	if err := createHostWindow(instance); err != nil {
		logLine(err.Error())
		msgBox(err.Error(), appName+" startup error")
		return
	}
	onExitRequest = func() { pPostMessageW.Call(hostHWND, wmExit, 0, 0) }
	addTrayIcon()
	bootStep("tray icon added")
	registerHotkeys() // v2.0.0: Ctrl+Alt+K and Ctrl+Alt+L, unless turned off in Settings
	go recorder()
	go inputThread() // Raw Input lives on its own thread, independent of the UI
	bootStep("input capture started")
	ln, err := startServer()
	if err != nil {
		logLine("HTTP server failed: " + err.Error())
		msgBox(err.Error(), appName+" server error")
		shutdown()
		return
	}
	serverURL = "http://" + ln.Addr().String()
	bootStep("local page server listening")
	url := serverURL + "/?token=" + token
	logLine("Local server listening at " + serverURL)
	if os.Getenv("KEYCLICK_DEV") == "1" { // test hook: lets automated UI tests find the page
		_ = os.WriteFile(filepath.Join(dataDir, "devurl.txt"), []byte(url), 0600)
	}

	go func() {
		for range time.Tick(5 * time.Second) {
			if dirty.Swap(false) {
				if e := saveStore(); e != nil {
					logLine("save failed: " + e.Error())
					dirty.Store(true)
				}
			}
		}
	}()
	go breakWatcher()

	view = webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:         os.Getenv("KEYCLICK_DEV") == "1",
		AutoFocus:     true,
		DataPath:      filepath.Join(dataDir, "WebViewProfile"),
		WindowOptions: webview2.WindowOptions{Title: appName, Width: 1280, Height: 820, IconId: iconResourceID, Center: true},
	})
	if view == nil {
		logLine("WebView2 runtime unavailable; opening the default browser instead")
		pShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16p("open"))), uintptr(unsafe.Pointer(utf16p(url))), 0, 0, swNormal)
		var m winMsg
		for {
			r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	} else {
		mainHWND = uintptr(view.Window())
		frameless(mainHWND)
		pShowWindow.Call(mainHWND, swHide) // stay hidden until the page reports it has painted (no white flash)
		bindWindowControls()
		var shown atomic.Bool
		reveal := func() {
			if hide { // started in the background (sign-in): the page stays hidden and idle
				setUIVisible(false)
				return
			}
			if !shown.CompareAndSwap(false, true) {
				return
			}
			pShowWindow.Call(mainHWND, swShow)
			pSetForegroundWindow.Call(mainHWND)
		}
		_ = view.Bind("kcReady", func() { reveal() })
		onSettingsChanged = func() { view.Dispatch(syncOverlay); view.Dispatch(registerHotkeys); view.Dispatch(applyAppLogo) }
		view.Dispatch(applyAppLogo) // v2.1.0: the chosen logo for the tray and window icon
		view.Dispatch(syncOverlay) // show the live overlay at start-up if it was left enabled
		view.Init("window.__KC_NATIVE = true;")
		view.Navigate(url)
		go func() { time.Sleep(6 * time.Second); view.Dispatch(reveal) }() // safety net if the page never signals
		view.Run()
		view.Destroy()
	}
	removeHooks()
	flushForExit()
	logLine(appName + " exited cleanly")
}
