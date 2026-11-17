//go:build windows

package main

import (
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
)

// The live overlay is a small, always-on-top, frameless, semi-transparent window
// that shows the current WPM / CPM. It is a second WebView2 window owned by this
// process, so it keeps working while the main window is hidden in the tray.

var (
	pSetLayered       = user32.NewProc("SetLayeredWindowAttributes")
	pMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	pIsIconic         = user32.NewProc("IsIconic")

	ovView     webview2.WebView
	ovHWND     uintptr
	ovOldProc  uintptr
	ovCallback uintptr
	ovAlpha    atomic.Int32
	ovGen      atomic.Int32
	ovWatching atomic.Bool
)

const (
	ovIdleAlpha  = 175 // a little see-through while the pointer is elsewhere
	ovHoverAlpha = 255
	wsPopup      = 0x80000000
	wsExTopmost  = 0x00000008
	wsExTool     = 0x00000080
	wsExLayered  = 0x00080000
	wsExNoActive = 0x08000000
	gwlExStyle   = -20
	lwaAlpha     = 2
	swpNoActive  = 0x10
	swShowNoAct  = 4
)

// onScreen reports whether the centre of the rectangle lies on a monitor that
// is currently connected. A saved position from a disconnected monitor fails this.
func onScreen(r rect) bool {
	cx := int64(r.Left+r.Right) / 2
	cy := int64(r.Top+r.Bottom) / 2
	// POINT is passed by value as one 64-bit argument on x64
	pt := uintptr(uint32(int32(cx))) | uintptr(uint32(int32(cy)))<<32
	h, _, _ := pMonitorFromPoint.Call(pt, 0) // MONITOR_DEFAULTTONULL
	return h != 0
}

// defaultOverlayXY is the top-right corner of the primary monitor.
func defaultOverlayXY(w, _ int, dpi int) (int, int) {
	pw, _, _ := pGetSystemMetrics.Call(0) // SM_CXSCREEN: primary monitor width (origin 0,0)
	return int(pw) - w - 24*dpi/96, 60 * dpi / 96
}

// ensureOverlayVisible repairs the overlay if it has been hidden, minimized,
// pushed off every monitor, or pushed behind other topmost windows. UI thread only.
func ensureOverlayVisible() {
	storeMu.RLock()
	on := store.Settings.OverlayOn
	storeMu.RUnlock()
	if !on || ovHWND == 0 {
		return
	}
	var r rect
	pGetWindowRect.Call(ovHWND, uintptr(unsafe.Pointer(&r)))
	iconic, _, _ := pIsIconic.Call(ovHWND)
	if !onScreen(r) || r.Right <= r.Left {
		dpi := dpiOf(ovHWND)
		w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
		if w <= 0 || h <= 0 {
			storeMu.RLock()
			w, h = store.Settings.OverlayW*dpi/96, store.Settings.OverlayH*dpi/96
			storeMu.RUnlock()
		}
		x, y := defaultOverlayXY(w, h, dpi)
		pSetWindowPos.Call(ovHWND, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActive)
		logLine("overlay was off-screen; moved it to the primary monitor")
	}
	if iconic != 0 {
		pShowWindow.Call(ovHWND, swShowNoAct)
	}
	if vis, _, _ := pIsWindowVisible.Call(ovHWND); vis == 0 {
		pShowWindow.Call(ovHWND, swShowNoAct)
	}
	// re-assert "always on top" without activating the window
	pSetWindowPos.Call(ovHWND, ^uintptr(0), 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActive)
}

func setOverlayAlpha(a int32) {
	ovAlpha.Store(a)
	if ovHWND != 0 {
		pSetLayered.Call(ovHWND, 0, uintptr(a), lwaAlpha)
	}
}

// fadeOverlay animates the window opacity so hover feels like a soft glow, not a jump.
func fadeOverlay(target int32) {
	gen := ovGen.Add(1)
	go func() {
		from := ovAlpha.Load()
		const steps = 9
		for i := 1; i <= steps; i++ {
			if ovGen.Load() != gen {
				return
			}
			setOverlayAlpha(from + (target-from)*int32(i)/steps)
			time.Sleep(16 * time.Millisecond)
		}
	}()
}

func overlayRegion(h uintptr) {
	var r rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	d := 2 * 18 * dpiOf(h) / 96
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(r.Right-r.Left+1), uintptr(r.Bottom-r.Top+1), uintptr(d), uintptr(d))
	pSetWindowRgn.Call(h, rgn, 1)
}

func overlayProc(h, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case wmNCCalcSize:
		if wp != 0 {
			return 0
		}
	case wmGetMinMax:
		mm := (*minMaxInfo)(unsafe.Pointer(lp))
		s := int32(dpiOf(h))
		mm.MinTrack = point{X: 150 * s / 96, Y: 56 * s / 96}
		mm.MaxTrack = point{X: 900 * s / 96, Y: 500 * s / 96}
	case wmClose:
		hideOverlay(true)
		return 0
	case wmSize:
		r, _, _ := pCallWindowProcW.Call(ovOldProc, h, msg, wp, lp)
		overlayRegion(h)
		return r
	}
	r, _, _ := pCallWindowProcW.Call(ovOldProc, h, msg, wp, lp)
	return r
}

func hideOverlay(persist bool) {
	if ovHWND != 0 {
		pShowWindow.Call(ovHWND, swHide)
	}
	if persist {
		storeMu.Lock()
		store.Settings.OverlayOn = false
		storeMu.Unlock()
		dirty.Store(true)
		_ = saveStore()
		jsEval("window.KC&&KC.overlayClosed&&KC.overlayClosed()") // keep the Settings switch in step
	}
}

// syncOverlay shows or hides the overlay to match the saved setting. UI thread only.
func syncOverlay() {
	storeMu.RLock()
	on := store.Settings.OverlayOn
	storeMu.RUnlock()
	if !on {
		if ovHWND != 0 {
			hideOverlay(false)
		}
		return
	}
	if ovHWND == 0 {
		createOverlay()
		return
	}
	pShowWindow.Call(ovHWND, 8) // SW_SHOWNA: show without taking focus
	applyOverlayPresetSize()    // v2.0.0 (F13): a changed style brings its own size
	if ovView != nil {          // apply theme / font / unit changes straight away
		ovView.Eval("window.kcOvReload&&kcOvReload()")
	}
}

// ovPresetApplied is the live box style whose size is currently applied to the window.
var ovPresetApplied string

// applyOverlayPresetSize resizes the box when the style changed. The size comes from the
// settings (applyOverlayPreset already stored each style's own size there). A size you dragged
// to is not touched by other setting changes, because this runs only when the style changes.
func applyOverlayPresetSize() {
	storeMu.RLock()
	preset, wDip, hDip := store.Settings.OverlayPreset, store.Settings.OverlayW, store.Settings.OverlayH
	storeMu.RUnlock()
	if preset == ovPresetApplied || ovHWND == 0 {
		return
	}
	ovPresetApplied = preset
	dpi := dpiOf(ovHWND)
	w, h := wDip*dpi/96, hDip*dpi/96
	var r rect
	pGetWindowRect.Call(ovHWND, uintptr(unsafe.Pointer(&r)))
	if abs(int(r.Right-r.Left)-w) > 1 || abs(int(r.Bottom-r.Top)-h) > 1 {
		pSetWindowPos.Call(ovHWND, 0, 0, 0, uintptr(w), uintptr(h), 0x0002|0x0004|0x0010) // SWP_NOMOVE | SWP_NOZORDER | SWP_NOACTIVATE
		overlayRegion(ovHWND)
	}
}

func createOverlay() {
	storeMu.RLock()
	st := *store.Settings
	storeMu.RUnlock()
	ovPresetApplied = st.OverlayPreset // the window is created at this style's size
	v := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:      filepath.Join(dataDir, "WebViewProfile"),
		WindowOptions: webview2.WindowOptions{Title: appName + " Live", Width: uint(st.OverlayW), Height: uint(st.OverlayH), IconId: iconResourceID},
	})
	if v == nil {
		logLine("overlay: WebView2 window could not be created")
		return
	}
	ovView = v
	ovHWND = uintptr(v.Window())
	pSetWindowLongPtrW.Call(ovHWND, ^uintptr(-gwlStyle-1), uintptr(wsPopup|wsThickFrame|wsClipChildren))
	pSetWindowLongPtrW.Call(ovHWND, ^uintptr(-gwlExStyle-1), uintptr(wsExTopmost|wsExTool|wsExLayered|wsExNoActive))
	ovCallback = syscall.NewCallback(overlayProc)
	ovOldProc, _, _ = pSetWindowLongPtrW.Call(ovHWND, ^uintptr(-gwlpWndProc-1), ovCallback)
	setOverlayAlpha(ovIdleAlpha)

	dpi := dpiOf(ovHWND)
	w, h := st.OverlayW*dpi/96, st.OverlayH*dpi/96
	x, y := defaultOverlayXY(w, h, dpi) // first run: top-right corner
	if st.OverlaySet {
		x, y = st.OverlayX, st.OverlayY
	}
	// Use the saved spot only if its centre is on a monitor that is connected now
	// (a monitor may have been unplugged since the position was saved).
	if !onScreen(rect{Left: int32(x), Top: int32(y), Right: int32(x + w), Bottom: int32(y + h)}) {
		x, y = defaultOverlayXY(w, h, dpi)
		logLine("saved overlay position is no longer on a connected screen; reset to the top-right corner")
	}
	pSetWindowPos.Call(ovHWND, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpFrameChanged|swpNoActive|swpShowWindow)
	overlayRegion(ovHWND)

	_ = v.Bind("kcOvDrag", func() {
		pReleaseCapture.Call()
		pPostMessageW.Call(ovHWND, wmNCLButton, 2, 0)
	})
	_ = v.Bind("kcOvResize", func() {
		pReleaseCapture.Call()
		pPostMessageW.Call(ovHWND, wmNCLButton, 17, 0) // HTBOTTOMRIGHT
	})
	_ = v.Bind("kcOvClose", func() { pPostMessageW.Call(ovHWND, wmClose, 0, 0) })
	_ = v.Bind("kcOvHover", func(on bool) {
		if on {
			fadeOverlay(ovHoverAlpha)
		} else {
			fadeOverlay(ovIdleAlpha)
		}
	})
	v.Navigate(serverURL + "/ui/overlay.html?token=" + token)
	logLine("overlay window created")

	if ovWatching.CompareAndSwap(false, true) {
		go watchOverlayRect()
	}
}

// watchOverlayRect remembers where the user dropped and how big they made the overlay.
func watchOverlayRect() {
	var last rect
	for range time.Tick(1500 * time.Millisecond) {
		if ovHWND == 0 || ovView == nil {
			continue
		}
		// Dispatch on the MAIN view: go-webview2 runs only the dispatch queue of the
		// view whose Run loop is active, so ovView.Dispatch never ran and the
		// overlay was never re-raised to the top after other windows took focus.
		view.Dispatch(ensureOverlayVisible) // repair on the UI thread
		if vis, _, _ := pIsWindowVisible.Call(ovHWND); vis == 0 {
			continue
		}
		if iconic, _, _ := pIsIconic.Call(ovHWND); iconic != 0 {
			continue // minimized windows report off-screen coordinates; never save those
		}
		var r rect
		pGetWindowRect.Call(ovHWND, uintptr(unsafe.Pointer(&r)))
		if r == last || r.Right <= r.Left || !onScreen(r) {
			continue
		}
		last = r
		dpi := dpiOf(ovHWND)
		storeMu.Lock()
		s := store.Settings
		s.OverlayX, s.OverlayY, s.OverlaySet = int(r.Left), int(r.Top), true
		s.OverlayW, s.OverlayH = int(r.Right-r.Left)*96/dpi, int(r.Bottom-r.Top)*96/dpi
		storeMu.Unlock()
		dirty.Store(true)
	}
}
