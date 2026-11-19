package main

// v2.0.0 features: global hotkeys (F12), idle time API (F2) and the data health
// check (F10). Everything here is read-only except the hotkey actions, which change
// only the live-box setting, the same way the Settings page does.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

const (
	wmHotkey         = 0x0312
	wmPowerBroadcast = 0x0218
	pbtResumeSuspend = 0x0007
	pbtResumeAuto    = 0x0012
	hotkeyMainID     = 1
	hotkeyLiveID     = 2
	modNoRepeat      = 0x4000
	defaultMainCombo = "ctrl+alt+k" // show or hide the main window
	defaultLiveCombo = "ctrl+alt+l" // turn the floating live box on or off
	dirSizeCacheFor  = 10 * time.Minute
)

var (
	pRegisterHotKey   = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	pGetDiskFreeSpace = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// hotkeyStatus is what the Settings page shows: "registered", "off", "in use by another program",
// or "invalid: ...". It is written only on the UI thread (registerHotkeys) and read by the API.
var hotkeyStatus struct {
	mu         sync.Mutex
	main, live string
}

func setHotkeyStatus(main, live string) {
	hotkeyStatus.mu.Lock()
	hotkeyStatus.main, hotkeyStatus.live = main, live
	hotkeyStatus.mu.Unlock()
}

func getHotkeyStatus() (string, string) {
	hotkeyStatus.mu.Lock()
	defer hotkeyStatus.mu.Unlock()
	return hotkeyStatus.main, hotkeyStatus.live
}

// tryRegister registers one combination on the host window. It never falls back to another
// combination: the result is reported as it is.
func tryRegister(id uintptr, combo string) string {
	mods, vk, err := parseCombo(combo)
	if err != nil {
		return "invalid: " + err.Error()
	}
	if r, _, _ := pRegisterHotKey.Call(hostHWND, id, uintptr(mods|modNoRepeat), uintptr(vk)); r == 0 {
		return "in use by another program"
	}
	return "registered"
}

// registerHotkeys (re)registers both hotkeys to match the setting. UI thread only.
func registerHotkeys() {
	if hostHWND == 0 {
		setHotkeyStatus("not started", "not started")
		return
	}
	pUnregisterHotKey.Call(hostHWND, hotkeyMainID)
	pUnregisterHotKey.Call(hostHWND, hotkeyLiveID)
	storeMu.RLock()
	off := store.Settings.HotkeyOff
	storeMu.RUnlock()
	if off {
		setHotkeyStatus("off", "off")
		logLine("global hotkeys are off")
		return
	}
	m := tryRegister(hotkeyMainID, defaultMainCombo)
	l := tryRegister(hotkeyLiveID, defaultLiveCombo)
	setHotkeyStatus(m, l)
	logLine("hotkeys: main " + defaultMainCombo + " " + m + "; live box " + defaultLiveCombo + " " + l)
}

// onHotkey runs on the UI thread when Windows delivers WM_HOTKEY.
func onHotkey(id uintptr) {
	switch id {
	case hotkeyMainID:
		if vis, _, _ := pIsWindowVisible.Call(mainHWND); vis != 0 {
			pShowWindow.Call(mainHWND, swHide)
		} else {
			showMainWindow()
		}
	case hotkeyLiveID:
		toggleLiveBox() // shared with the tray menu (traymenu.go)
	}
}

// onPowerBroadcast resets the idle anchor when the PC wakes, so sleep is not counted as idle.
func onPowerBroadcast(wp uintptr) {
	if wp == pbtResumeAuto || wp == pbtResumeSuspend {
		clearHeld() // v2.4.0: a hold open across sleep is dropped, not counted as hours
		resetIdleAnchor()
	}
}

// hotkeysJSON is the body of GET /api/hotkeys.
func hotkeysJSON() map[string]any {
	storeMu.RLock()
	off := store.Settings.HotkeyOff
	storeMu.RUnlock()
	m, l := getHotkeyStatus()
	part := func(combo, status string) map[string]any {
		out := map[string]any{"on": !off, "combo": combo, "status": status, "registered": status == "registered"}
		if status != "registered" && status != "off" {
			out["reason"] = status
		}
		return out
	}
	return map[string]any{"main_window": part(defaultMainCombo, m), "live_box": part(defaultLiveCombo, l)}
}

// idleStats sums idle and active time over the chosen range (see V2.0.0-PLAN.md, A.2).
// active_sec is the minutes that had input, multiplied by 60; idle_sec is the idle time
// measured by the idle threshold.
func idleStats(q map[string][]string) map[string]any {
	start, end, label := rangeBounds(q)
	var idle, active, withData uint64
	storeMu.RLock()
	for k, d := range store.Days {
		t, err := time.ParseInLocation("2006-01-02", k, time.Local)
		if err != nil || t.Before(start) || !t.Before(end) {
			continue
		}
		idle += d.IdleSec
		active += uint64(d.ActiveMin) * 60
		if d.IdleSec > 0 || d.ActiveMin > 0 {
			withData++
		}
	}
	setting := store.Settings.IdleMinutes
	storeMu.RUnlock()
	return map[string]any{
		"range":                label,
		"idle_minutes_setting": idleMinutesOrDefault(setting),
		"active_sec":           active,
		"idle_sec":             idle,
		"days_with_data":       withData,
	}
}

// dirSize adds up the file sizes under root. A missing folder counts as 0.
func dirSize(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, e := d.Info(); e == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

var profileCache struct {
	mu    sync.Mutex
	bytes int64
	at    time.Time
}

// profileBytes returns the WebView2 profile size, cached for dirSizeCacheFor because the walk is slow.
func profileBytes() (int64, time.Duration) {
	profileCache.mu.Lock()
	defer profileCache.mu.Unlock()
	if profileCache.at.IsZero() || time.Since(profileCache.at) > dirSizeCacheFor {
		profileCache.bytes = dirSize(filepath.Join(dataDir, "WebViewProfile"))
		profileCache.at = time.Now()
	}
	return profileCache.bytes, time.Since(profileCache.at)
}

// diskFree returns the free bytes available on the drive that holds the data folder.
func diskFree() int64 {
	p := utf16p(dataDir)
	var avail, total, totalFree uint64
	r, _, _ := pGetDiskFreeSpace.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
	if r == 0 {
		return -1
	}
	return int64(avail)
}

// healthStats is the body of GET /api/health. It never writes or repairs anything.
func healthStats() map[string]any {
	var checks []map[string]string
	var levels []string
	add := func(id, level, text string) {
		checks = append(checks, map[string]string{"id": id, "level": level, "text": text})
		levels = append(levels, level)
	}

	var size int64
	exists := false
	parses := false
	storeVer := 0
	if fi, err := os.Stat(dataPath); err == nil {
		exists = true
		size = fi.Size()
		if b, e := os.ReadFile(dataPath); e == nil {
			var tmp Store
			if json.Unmarshal(b, &tmp) == nil {
				parses = true
				storeVer = tmp.Version
			}
		}
	}
	if !exists {
		add("file_parses", levelOK, "No statistics file yet. It is created after the first activity.")
	} else if parses {
		add("file_parses", levelOK, "Statistics file reads correctly.")
	} else {
		add("file_parses", levelRed, "Statistics file cannot be read. Open the data folder to check it.")
	}
	if exists {
		lvl := fileSizeLevel(size)
		add("file_size", lvl, "Statistics file is "+humanBytes(size)+".")
	}

	if ts := lastSaveUnix.Load(); ts == 0 {
		add("last_save", levelOK, "Not saved yet in this session.")
	} else {
		ago := time.Since(time.Unix(ts, 0)).Round(time.Second)
		add("last_save", levelOK, "Saved "+ago.String()+" ago.")
	}

	pb, _ := profileBytes()
	add("profile_size", profileLevel(pb), "Browser cache is "+humanBytes(pb)+". Clearing it is safe, but not required.")

	free := diskFree()
	if free < 0 {
		add("disk", levelAmber, "Free space on the data drive could not be read.")
	} else {
		add("disk", diskLevel(free), humanBytes(free)+" free on the data drive.")
	}

	if hooksOK() {
		add("input", levelOK, "Input counting is running.")
	} else {
		add("input", levelRed, "Input counting is not running.")
	}
	if loadFailed {
		add("load", levelRed, "The statistics file could not be read at start, so it is not being overwritten.")
	}

	backups, total, newest := backupSummary()
	bt := "Backups: " + strconv.Itoa(backups) + " files, " + humanBytes(total) + "."
	add("backups", levelOK, bt)

	return map[string]any{
		"status": worstLevel(levels...),
		"data_file": map[string]any{
			"exists": exists, "size_bytes": size, "parses": parses, "store_version": storeVer, "app_version": appVersion,
		},
		"last_save":       lastSaveTime(),
		"backups":         map[string]any{"count": backups, "total_bytes": total, "newest": newest},
		"webview_profile": map[string]any{"bytes": pb},
		"disk_free_bytes": free,
		"hooks_ok":        hooksOK(),
		"load_failed":     loadFailed,
		"boot":            bootSteps(),
		"checks":          checks,
	}
}

func lastSaveTime() string {
	ts := lastSaveUnix.Load()
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).Format(time.RFC3339)
}

// backupSummary counts the files in the Backups folder without creating it.
func backupSummary() (count int, total int64, newest string) {
	dir := filepath.Join(dataDir, "Backups")
	es, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, ""
	}
	for _, e := range es {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		count++
		total += fi.Size()
		if e.Name() > newest {
			newest = e.Name() // file names carry a time stamp, so the largest name is the newest
		}
	}
	return count, total, newest
}

func humanBytes(n int64) string {
	switch {
	case n < 0:
		return "unknown size"
	case n < 1<<10:
		return strconv.FormatInt(n, 10) + " bytes"
	case n < 1<<20:
		return f1(float64(n)/(1<<10)) + " KB"
	case n < 1<<30:
		return f1(float64(n)/(1<<20)) + " MB"
	}
	return f1(float64(n)/(1<<30)) + " GB"
}
