package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestBuildIconGroup(t *testing.T) {
	g := buildIconGroup([]int{16, 256}, []int{100, 5000})
	if len(g) != 6+2*14 {
		t.Fatalf("length %d, want %d", len(g), 6+2*14)
	}
	if binary.LittleEndian.Uint16(g[2:]) != 1 || binary.LittleEndian.Uint16(g[4:]) != 2 {
		t.Fatal("bad header: type must be 1 and count 2")
	}
	if g[6] != 16 || g[7] != 16 {
		t.Fatalf("first entry size %dx%d, want 16x16", g[6], g[7])
	}
	if g[6+14] != 0 || g[7+14] != 0 {
		t.Fatal("a 256 pixel image must be written as 0 (an icon directory stores 256 as 0)")
	}
	if binary.LittleEndian.Uint32(g[6+8:]) != 100 || binary.LittleEndian.Uint32(g[6+14+8:]) != 5000 {
		t.Fatal("image byte counts were not written")
	}
	if binary.LittleEndian.Uint16(g[6+12:]) != iconImageBase || binary.LittleEndian.Uint16(g[6+14+12:]) != iconImageBase+1 {
		t.Fatal("image resource ids must count up from iconImageBase")
	}
}

func TestMarkerRoundTrip(t *testing.T) {
	m := exeIconMarker{logo: "02_Emerald_Mint.png", size: 15083520, mtime: 1763430000123456789}
	got, ok := parseMarker(formatMarker(m))
	if !ok || got != m {
		t.Fatalf("round trip gave %+v ok=%v", got, ok)
	}
	for _, bad := range []string{"", "a|b|c", "x|1", "x|1|2|3"} {
		if _, ok := parseMarker(bad); ok {
			t.Fatalf("%q must not parse", bad)
		}
	}
	// the default logo is stored as an empty name
	if got, ok := parseMarker(formatMarker(exeIconMarker{"", 5, 6})); !ok || got.logo != "" {
		t.Fatal("an empty logo name must round trip")
	}
}

func TestIconPending(t *testing.T) {
	m := exeIconMarker{logo: "02_Emerald_Mint.png"}
	cases := []struct {
		name                 string
		want                 string
		m                    exeIconMarker
		haveMarker, sameFile bool
		pending              bool
	}{
		{"custom logo never applied", "02_Emerald_Mint.png", exeIconMarker{}, false, false, true},
		{"custom logo already in this file", "02_Emerald_Mint.png", m, true, true, false},
		{"file replaced by an update", "02_Emerald_Mint.png", m, true, false, true},
		{"different logo in the file", "03_Neon_Violet_Magenta.png", m, true, true, true},
		{"default logo, fresh file", "", exeIconMarker{}, false, false, false},
		{"default logo, file replaced by an update", "", m, true, false, false},
		{"default logo, file still has a custom one", "", m, true, true, true},
		{"default logo, file already reset", "", exeIconMarker{logo: ""}, true, true, false},
	}
	for _, c := range cases {
		if got := iconPending(c.want, c.m, c.haveMarker, c.sameFile); got != c.pending {
			t.Errorf("%s: pending=%v, want %v", c.name, got, c.pending)
		}
	}
}

// Patches a copy of the test program (which carries the app icon from rsrc_windows_amd64.syso)
// and reads the icon directory back: it must now describe the six new images.
func TestPatchExeIconOnCopy(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path")
	}
	cp := filepath.Join(t.TempDir(), "copy.exe")
	if err := copyFile(exe, cp); err != nil {
		t.Fatal(err)
	}
	if err := patchExeIcon(cp, logoPNG("02_Emerald_Mint.png")); err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	h, _, _ := pLoadLibraryExW.Call(uintptr(unsafe.Pointer(utf16p(cp))), 0, 0x2)
	if h == 0 {
		t.Fatal("cannot load the patched copy")
	}
	defer pFreeLibrary.Call(h)
	find := kernel32.NewProc("FindResourceW")
	load := kernel32.NewProc("LoadResource")
	lock := kernel32.NewProc("LockResource")
	size := kernel32.NewProc("SizeofResource")
	res, _, _ := find.Call(h, iconGroupID, rtGroupIcon)
	if res == 0 {
		t.Fatal("the patched copy has no icon group")
	}
	n, _, _ := size.Call(h, res)
	lr, _, _ := load.Call(h, res)
	p, _, _ := lock.Call(lr)
	data := unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))
	if got := binary.LittleEndian.Uint16(data[4:]); int(got) != len(iconSizes) {
		t.Fatalf("icon group has %d images, want %d", got, len(iconSizes))
	}
	if !strings.HasSuffix(cp, ".exe") {
		t.Fatal("unexpected copy name")
	}
}
