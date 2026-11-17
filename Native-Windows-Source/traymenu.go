package main

// v2.0.0 tray menu. The old Windows menu cannot have rounded corners or the app's colours,
// so the menu is a small popup window drawn here with GDI. Colours come from the theme the
// user chose in the app (ui/themes.js is the single source of the palettes). If the window
// cannot be created, the old native menu is used instead.

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	pBeginPaint          = user32.NewProc("BeginPaint")
	pEndPaint            = user32.NewProc("EndPaint")
	pGetClientRect       = user32.NewProc("GetClientRect")
	pInvalidateRect      = user32.NewProc("InvalidateRect")
	pDrawTextW           = user32.NewProc("DrawTextW")
	pGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	pTrackMouseEvent     = user32.NewProc("TrackMouseEvent")
	pGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	pSetFocus            = user32.NewProc("SetFocus")
	pCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	pCreatePen           = gdi32.NewProc("CreatePen")
	pDeleteObject        = gdi32.NewProc("DeleteObject")
	pSelectObject        = gdi32.NewProc("SelectObject")
	pGetStockObject      = gdi32.NewProc("GetStockObject")
	pSetBkMode           = gdi32.NewProc("SetBkMode")
	pSetTextColor        = gdi32.NewProc("SetTextColor")
	pCreateFontW         = gdi32.NewProc("CreateFontW")
	pRoundRect           = gdi32.NewProc("RoundRect")
	pEllipse             = gdi32.NewProc("Ellipse")
	pCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBmp = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt              = gdi32.NewProc("BitBlt")
	pDeleteDC            = gdi32.NewProc("DeleteDC")
	pMoveToEx            = gdi32.NewProc("MoveToEx")
	pLineTo              = gdi32.NewProc("LineTo")
)

const (
	menuLive   = 504 // floating live box on/off
	menuSnooze = 505 // remind me again in 5 minutes (break reminders)

	trayMenuClass = "KeyClickTrayMenu"

	wmPaint        = 0x000F
	wmEraseBkgnd   = 0x0014
	wmActivate     = 0x0006
	wmKeyDown      = 0x0100
	wmMouseMove    = 0x0200
	wmLButtonUp    = 0x0202
	wmMouseLeave   = 0x02A3
	wmTrayLBtnUp   = 0x0202
	vkEscape       = 0x1B
	vkReturn       = 0x0D
	vkUp           = 0x26
	vkDown         = 0x28
	trayItemNormal = 0
	trayItemSep    = 1
	trayItemToggle = 2
)

type paintStruct struct {
	HDC       uintptr
	FErase    int32
	RcPaint   rect
	FRestore  int32
	FIncUpdat int32
	Reserved  [32]byte
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type trackMouse struct {
	Size      uint32
	Flags     uint32
	HWndTrack uintptr
	HoverTime uint32
}

type trayItem struct {
	id      int
	label   string
	kind    int
	on      bool
	enabled bool
	top     int32 // pixel rows, filled in by layout
	bottom  int32
}

type trayPalette struct {
	bg, hover, ink, muted, line, accent uint32 // COLORREF (0x00BBGGRR)
}

var tray struct {
	hwnd    uintptr
	items   []trayItem
	hover   int
	wasFG   bool // the menu has been the foreground window at least once
	pal     trayPalette
	scale   float64
	font    uintptr
	cls     bool
	tracked bool
}

var trayMenuCallback uintptr

// ---------------------------------------------------------------- palette (from ui/themes.js)

var (
	themeOnce sync.Once
	themeMap  map[string][]string
	hexRE     = regexp.MustCompile(`#[0-9a-fA-F]{6}`)
	idRE      = regexp.MustCompile(`^\s*\['([a-z0-9]+)'`)
)

func loadThemes() {
	themeMap = map[string][]string{}
	b, err := uiFiles.ReadFile("ui/themes.js")
	if err != nil {
		return
	}
	for _, ln := range strings.Split(string(b), "\n") {
		m := idRE.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		hx := hexRE.FindAllString(ln, -1)
		if len(hx) >= 9 { // bg sidebar panel panel2 ink muted line accent accent2
			themeMap[m[1]] = hx
		}
	}
}

func colorRef(hex string) uint32 {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return 0
	}
	r, g, b := uint32(v>>16)&0xFF, uint32(v>>8)&0xFF, uint32(v)&0xFF
	return b<<16 | g<<8 | r
}

// paletteFor returns the menu colours for a theme id (unknown ids use the first theme).
func paletteFor(id string) trayPalette {
	themeOnce.Do(loadThemes)
	hx := themeMap[id]
	if hx == nil {
		hx = themeMap["classic"]
	}
	if hx == nil { // themes.js unreadable: a safe dark palette
		return trayPalette{bg: colorRef("#1f2026"), hover: colorRef("#2c2e36"), ink: colorRef("#f0f0f1"), muted: colorRef("#92949e"), line: colorRef("#2e3038"), accent: colorRef("#91a8e6")}
	}
	// hx indexes: 0 bg, 1 sidebar, 2 panel, 3 panel2, 4 ink, 5 muted, 6 line, 7 accent, 8 accent2
	return trayPalette{bg: colorRef(hx[2]), hover: colorRef(hx[3]), ink: colorRef(hx[4]), muted: colorRef(hx[5]), line: colorRef(hx[6]), accent: colorRef(hx[7])}
}

// ---------------------------------------------------------------- items

func buildTrayItems() []trayItem {
	storeMu.RLock()
	liveOn := store.Settings != nil && store.Settings.OverlayOn
	storeMu.RUnlock()
	pause := "Pause monitoring"
	if isPaused() {
		pause = "Resume monitoring"
	}
	items := []trayItem{
		{id: menuOpen, label: "Open " + appName, enabled: true},
		{id: menuToggle, label: pause, enabled: true},
		{id: menuLive, label: "Floating live box", kind: trayItemToggle, on: liveOn, enabled: true},
	}
	if snoozeAvailable != nil && snoozeAvailable() {
		items = append(items, trayItem{id: menuSnooze, label: "Remind me in 5 minutes", enabled: true})
	}
	items = append(items, trayItem{kind: trayItemSep}, trayItem{id: menuExit, label: "Exit " + appName, enabled: true})
	return items
}

// snoozeAvailable is set by the break-reminder code (features2.go) when a reminder is waiting.
var snoozeAvailable func() bool

// toggleLiveBox turns the floating live box on or off (tray menu, hotkey and Settings share this).
func toggleLiveBox() {
	storeMu.Lock()
	store.Settings.OverlayOn = !store.Settings.OverlayOn
	storeMu.Unlock()
	dirty.Store(true)
	_ = saveStore()
	if onSettingsChanged != nil {
		onSettingsChanged() // shows or hides the box on the UI thread
	}
}

// ---------------------------------------------------------------- show / hide

func px(dip int) int32 { return int32(float64(dip)*tray.scale + 0.5) }

func layoutTrayItems() (w, h int32) {
	const itemH, sepH, pad, width = 36, 9, 8, 256
	y := px(pad)
	for i := range tray.items {
		it := &tray.items[i]
		it.top = y
		if it.kind == trayItemSep {
			y += px(sepH)
		} else {
			y += px(itemH)
		}
		it.bottom = y
	}
	return px(width), y + px(pad)
}

func closeTrayMenu() {
	if tray.hwnd != 0 {
		h := tray.hwnd
		tray.hwnd = 0
		pDestroyWindow.Call(h)
	}
}

func showTrayMenu() {
	closeTrayMenu()
	if !openCustomTrayMenu() {
		showNativeTrayMenu() // fallback: the plain Windows menu
	}
}

func openCustomTrayMenu() bool {
	instance := appInstance
	if !tray.cls {
		className := utf16p(trayMenuClass)
		cursor, _, _ := pLoadCursorW.Call(0, 32512)
		trayMenuCallback = syscall.NewCallback(trayMenuProc)
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: trayMenuCallback, Instance: instance, Cursor: cursor, ClassName: className}
		if r, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			logLine("tray menu: RegisterClassExW failed; using the Windows menu")
			return false
		}
		tray.cls = true
	}
	storeMu.RLock()
	themeID := "classic"
	if store.Settings != nil && store.Settings.Theme != "" {
		themeID = store.Settings.Theme
	}
	storeMu.RUnlock()
	tray.pal = paletteFor(themeID)
	tray.items = buildTrayItems()
	tray.hover = -1
	tray.tracked = false
	tray.wasFG = false

	var cp point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cp)))
	// MonitorFromPoint takes the POINT by value: both coordinates packed in one 64-bit argument.
	packed := uintptr(uint64(uint32(cp.Y))<<32 | uint64(uint32(cp.X)))
	mon, _, _ := pMonitorFromPoint.Call(packed, 2) // MONITOR_DEFAULTTONEAREST
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))

	// Create small first, so the window's own DPI is known, then size it.
	hwnd, _, _ := pCreateWindowExW.Call(wsExTopmost|wsExTool, uintptr(unsafe.Pointer(utf16p(trayMenuClass))), 0, wsPopup, uintptr(cp.X), uintptr(cp.Y), 10, 10, 0, 0, instance, 0)
	if hwnd == 0 {
		logLine("tray menu: CreateWindowExW failed; using the Windows menu")
		return false
	}
	tray.hwnd = hwnd
	tray.scale = float64(dpiOf(hwnd)) / 96
	if tray.font != 0 {
		pDeleteObject.Call(tray.font)
	}
	tray.font, _, _ = pCreateFontW.Call(uintptr(^uint(px(15)-1)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(utf16p("Segoe UI"))))

	w, h := layoutTrayItems()
	x, y := cp.X, cp.Y-h // the menu opens upwards from the tray, like the Windows menu
	if mi.Work.Right != 0 {
		if x+w > mi.Work.Right {
			x = mi.Work.Right - w
		}
		if x < mi.Work.Left {
			x = mi.Work.Left
		}
		if y < mi.Work.Top {
			y = mi.Work.Top
		}
		if y+h > mi.Work.Bottom {
			y = mi.Work.Bottom - h
		}
	}
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0040) // HWND_TOPMOST, SWP_SHOWWINDOW
	d := px(28)                                                                                   // 14 dip radius, as a diameter
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(d), uintptr(d))
	pSetWindowRgn.Call(hwnd, rgn, 1)
	pSetForegroundWindow.Call(hwnd)
	pSetFocus.Call(hwnd)
	pSetTimer.Call(hwnd, 1, 120, 0) // watches for a click outside or a switch to another window
	pInvalidateRect.Call(hwnd, 0, 0)
	return true
}

// ---------------------------------------------------------------- window procedure

func hitTrayItem(y int32) int {
	for i, it := range tray.items {
		if it.kind != trayItemSep && it.enabled && y >= it.top && y < it.bottom {
			return i
		}
	}
	return -1
}

func moveTrayHover(step int) {
	n := len(tray.items)
	i := tray.hover
	for k := 0; k < n; k++ {
		i = (i + step + n) % n
		if tray.items[i].kind != trayItemSep && tray.items[i].enabled {
			tray.hover = i
			pInvalidateRect.Call(tray.hwnd, 0, 0)
			return
		}
	}
}

func activateTrayItem(i int) {
	if i < 0 || i >= len(tray.items) {
		return
	}
	id := tray.items[i].id
	closeTrayMenu()
	switch id {
	case menuOpen:
		showMainWindow()
	case menuToggle:
		setPaused(!isPaused())
	case menuLive:
		toggleLiveBox()
	case menuSnooze:
		if snoozeReminder != nil {
			snoozeReminder()
		}
	case menuExit:
		shutdown()
	}
}

// snoozeReminder is set by the break-reminder code (features2.go).
var snoozeReminder func()

func trayMenuProc(h, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintTrayMenu(h)
		return 0
	case wmMouseMove:
		y := int32(int16(uint16(lp >> 16)))
		if i := hitTrayItem(y); i != tray.hover {
			tray.hover = i
			pInvalidateRect.Call(h, 0, 0)
		}
		if !tray.tracked {
			tme := trackMouse{Size: uint32(unsafe.Sizeof(trackMouse{})), Flags: 2, HWndTrack: h}
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			tray.tracked = true
		}
		return 0
	case wmMouseLeave:
		tray.tracked = false
		if tray.hover != -1 {
			tray.hover = -1
			pInvalidateRect.Call(h, 0, 0)
		}
		return 0
	case wmLButtonUp:
		y := int32(int16(uint16(lp >> 16)))
		activateTrayItem(hitTrayItem(y))
		return 0
	case wmKeyDown:
		switch wp {
		case vkEscape:
			closeTrayMenu()
		case vkDown:
			moveTrayHover(1)
		case vkUp:
			moveTrayHover(-1)
		case vkReturn:
			activateTrayItem(tray.hover)
		}
		return 0
	case wmTimer:
		if wp == 1 && tray.hwnd == h {
			var cp point
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&cp)))
			var r rect
			pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
			outside := cp.X < r.Left || cp.X >= r.Right || cp.Y < r.Top || cp.Y >= r.Bottom
			down := false
			for _, vk := range []uintptr{0x01, 0x02, 0x04} { // left, right, middle mouse button
				if s, _, _ := pGetAsyncKeyState.Call(vk); s&0x8000 != 0 {
					down = true
				}
			}
			fg, _, _ := pForegroundWnd.Call()
			if fg == h {
				tray.wasFG = true
			}
			if (outside && down) || (tray.wasFG && fg != h) { // clicked elsewhere, or focus moved away
				closeTrayMenu()
			}
		}
		return 0
	case wmActivate:
		if uint16(wp&0xFFFF) == 0 && tray.hwnd == h { // lost focus: close like a normal menu
			closeTrayMenu()
		}
		return 0
	case wmDestroy:
		if tray.hwnd == h {
			tray.hwnd = 0
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(h, msg, wp, lp)
	return r
}

// ---------------------------------------------------------------- drawing

func paintTrayMenu(h uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer pEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	w, hh := rc.Right, rc.Bottom
	mem, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBmp.Call(hdc, uintptr(w), uintptr(hh))
	old, _, _ := pSelectObject.Call(mem, bmp)
	drawTrayMenu(mem, w, hh)
	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(hh), mem, 0, 0, 0x00CC0020) // SRCCOPY
	pSelectObject.Call(mem, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
}

func drawTrayMenu(dc uintptr, w, h int32) {
	pal := tray.pal
	nullPen, _, _ := pGetStockObject.Call(8) // NULL_PEN
	bg, _, _ := pCreateSolidBrush.Call(uintptr(pal.bg))
	hov, _, _ := pCreateSolidBrush.Call(uintptr(pal.hover))
	acc, _, _ := pCreateSolidBrush.Call(uintptr(pal.accent))
	trk, _, _ := pCreateSolidBrush.Call(uintptr(pal.line))
	knob, _, _ := pCreateSolidBrush.Call(uintptr(pal.ink))
	border, _, _ := pCreatePen.Call(0, uintptr(px(1)), uintptr(pal.line))
	sep, _, _ := pCreatePen.Call(0, uintptr(px(1)), uintptr(pal.line))
	defer func() {
		// An object cannot be deleted while it is selected into a device context.
		whitePen, _, _ := pGetStockObject.Call(6)
		whiteBrush, _, _ := pGetStockObject.Call(0)
		pSelectObject.Call(dc, whitePen)
		pSelectObject.Call(dc, whiteBrush)
		for _, o := range []uintptr{bg, hov, acc, trk, knob, border, sep} {
			pDeleteObject.Call(o)
		}
	}()

	// Panel with a thin border and rounded corners.
	pSelectObject.Call(dc, bg)
	pSelectObject.Call(dc, border)
	d := uintptr(px(28))
	pRoundRect.Call(dc, 0, 0, uintptr(w), uintptr(h), d, d)

	pSetBkMode.Call(dc, 1) // TRANSPARENT
	pSelectObject.Call(dc, tray.font)
	inset := px(6)
	for i, it := range tray.items {
		if it.kind == trayItemSep {
			mid := (it.top + it.bottom) / 2
			pSelectObject.Call(dc, sep)
			pMoveToEx.Call(dc, uintptr(px(14)), uintptr(mid), 0)
			pLineTo.Call(dc, uintptr(w-px(14)), uintptr(mid))
			continue
		}
		if i == tray.hover {
			pSelectObject.Call(dc, hov)
			pSelectObject.Call(dc, nullPen)
			pRoundRect.Call(dc, uintptr(inset), uintptr(it.top), uintptr(w-inset), uintptr(it.bottom), uintptr(px(16)), uintptr(px(16)))
		}
		col := pal.ink
		if !it.enabled {
			col = pal.muted
		}
		pSetTextColor.Call(dc, uintptr(col))
		tr := rect{Left: px(18), Top: it.top, Right: w - px(18), Bottom: it.bottom}
		if it.kind == trayItemToggle {
			tr.Right = w - px(18) - px(44)
		}
		lbl := utf16p(it.label)
		pDrawTextW.Call(dc, uintptr(unsafe.Pointer(lbl)), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), 0x824) // LEFT|VCENTER|SINGLELINE|NOPREFIX
		if it.kind == trayItemToggle {
			pw, ph := px(34), px(18)
			x1 := w - px(18) - pw
			y1 := (it.top+it.bottom)/2 - ph/2
			if it.on {
				pSelectObject.Call(dc, acc)
			} else {
				pSelectObject.Call(dc, trk)
			}
			pSelectObject.Call(dc, nullPen)
			pRoundRect.Call(dc, uintptr(x1), uintptr(y1), uintptr(x1+pw), uintptr(y1+ph), uintptr(ph), uintptr(ph))
			kd := ph - px(4)
			kx := x1 + px(2)
			if it.on {
				kx = x1 + pw - px(2) - kd
			}
			pSelectObject.Call(dc, knob)
			pEllipse.Call(dc, uintptr(kx), uintptr(y1+px(2)), uintptr(kx+kd), uintptr(y1+px(2)+kd))
		}
	}
}
