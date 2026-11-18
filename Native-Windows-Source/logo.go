package main

// v2.1.0: the user can choose the app logo from the 20 logos in ui/logos (embedded in the
// program). The choice changes the tray icon, the taskbar and window icon and the logos
// inside the app. Icons are made from the chosen PNG at run time, so the default
// resource icon is kept as the fallback.

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"regexp"
	"sync"
	"unsafe"
)

var (
	pCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
)

const logoIconFormat = 0x00030000 // dwVer for PNG icon data (Windows Vista and later)

var logoIDPattern = regexp.MustCompile(`^\d{2}_[A-Za-z0-9_]+\.png$`)

// validLogo reports whether id names one of the logos embedded in the program. "" means the
// default logo (ui/logo.png), which is valid as a stored value too.
func validLogo(id string) bool {
	if id == "" {
		return true
	}
	if !logoIDPattern.MatchString(id) {
		return false
	}
	_, err := uiFiles.ReadFile("ui/logos/" + id)
	return err == nil
}

// logoPNG returns the PNG bytes for id, or the default logo when id is empty or unknown.
func logoPNG(id string) []byte {
	if id != "" && validLogo(id) {
		if b, err := uiFiles.ReadFile("ui/logos/" + id); err == nil {
			return b
		}
	}
	b, _ := uiFiles.ReadFile("ui/logo.png")
	return b
}

// scaleLogo shrinks a PNG to size x size with a box filter (average of each block).
func scaleLogo(pngBytes []byte, size int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, src, b.Min, draw.Src)
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/size, b.Min.Y+(y+1)*b.Dy()/size
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < size; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/size, b.Min.X+(x+1)*b.Dx()/size
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := rgba.RGBAAt(xx, yy)
					r += uint32(c.R)
					g += uint32(c.G)
					bl += uint32(c.B)
					a += uint32(c.A)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// iconFromPNG makes a Windows icon of the given size from PNG data.
func iconFromPNG(pngBytes []byte, size int) uintptr {
	data, err := scaleLogo(pngBytes, size)
	if err != nil || len(data) == 0 {
		return 0
	}
	h, _, _ := pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, logoIconFormat, uintptr(size), uintptr(size), 0)
	return h
}

var (
	logoMu      sync.Mutex
	logoIcons   = map[string][2]uintptr{} // id -> small (16) and big (32) icon handles
	appliedLogo = "\x00none"              // logo last applied to the tray and window
)

// logoIconsFor returns the small and big icons for id, creating them once.
func logoIconsFor(id string) (uintptr, uintptr) {
	logoMu.Lock()
	defer logoMu.Unlock()
	if ic, ok := logoIcons[id]; ok && ic[0] != 0 {
		return ic[0], ic[1]
	}
	png := logoPNG(id)
	ic := [2]uintptr{iconFromPNG(png, 16), iconFromPNG(png, 32)}
	if ic[0] == 0 || ic[1] == 0 {
		return 0, 0 // the caller keeps the resource icon
	}
	logoIcons[id] = ic
	return ic[0], ic[1]
}

// applyAppLogo updates the tray icon and the window icon to the chosen logo. It runs on
// the UI thread (through view.Dispatch) when the setting changes, and once at start.
func applyAppLogo() {
	storeMu.RLock()
	id := ""
	if store.Settings != nil {
		id = store.Settings.AppLogo
	}
	storeMu.RUnlock()
	if id == appliedLogo {
		return
	}
	small, big := logoIconsFor(id)
	if small == 0 || big == 0 {
		return
	}
	appliedLogo = id
	if trayAdded {
		nid := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: hostHWND, ID: 1, Flags: nifIcon, Icon: small}
		pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	}
	if view != nil {
		if hwnd := uintptr(view.Window()); hwnd != 0 {
			pSendMessageW.Call(hwnd, wmSetIcon, 0, small) // ICON_SMALL
			pSendMessageW.Call(hwnd, wmSetIcon, 1, big)   // ICON_BIG
		}
	}
}
