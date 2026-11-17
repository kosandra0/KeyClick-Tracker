package main

// F1 per-application tracking (v2.0.0). A background watcher checks the foreground window
// every 500 ms and remembers the executable file name of the program that owns it. Counts
// are then added to that day under the same lock as every other count. Only the file name
// is stored: never a window title, command line or full path.

import (
	"os"
	"sort"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

var (
	pForegroundWnd    = user32.NewProc("GetForegroundWindow")
	pWndThreadPID     = user32.NewProc("GetWindowThreadProcessId")
	pOpenProc         = kernel32.NewProc("OpenProcess")
	pCloseHdl         = kernel32.NewProc("CloseHandle")
	pQueryImageName   = kernel32.NewProc("QueryFullProcessImageNameW")
	processQueryLimit = uintptr(0x1000) // PROCESS_QUERY_LIMITED_INFORMATION
)

// currentApp is the name counted for new input ("" = not attributed, e.g. our own windows).
// Guarded by storeMu.
var currentApp string

func setCurrentApp(name string) {
	storeMu.Lock()
	currentApp = name
	storeMu.Unlock()
}

// processNameForWindow returns the file name of the program that owns window h.
// "" means the window belongs to this app (not counted). "(protected)" means the program
// cannot be opened, for example an elevated system process.
func processNameForWindow(h uintptr) string {
	if h == 0 {
		return ""
	}
	var pid uint32
	pWndThreadPID.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	if int(pid) == os.Getpid() {
		return ""
	}
	hp, _, _ := pOpenProc.Call(processQueryLimit, 0, uintptr(pid))
	if hp == 0 {
		return appNameFromPath("")
	}
	defer pCloseHdl.Call(hp)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := pQueryImageName.Call(hp, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return appNameFromPath("")
	}
	return appNameFromPath(utf16ToString(buf[:size]))
}

func utf16ToString(u []uint16) string { return syscall.UTF16ToString(u) }

// appWatcher runs for the life of the process. It updates currentApp when focus changes,
// and once a second credits the focused program with time while input was recent.
func appWatcher() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastH uintptr
	var lastName string
	tick := 0
	for range ticker.C {
		h, _, _ := pForegroundWnd.Call()
		updateLayout(h) // F14: keyboard layout of the focused window
		if h != lastH {
			lastH = h
			lastName = processNameForWindow(h)
			setCurrentApp(lastName)
		}
		tick++
		if tick%2 == 0 {
			addAppSecond()
		}
	}
}

// addAppSecond credits one second to the focused program when input happened within the
// idle threshold. An idle program earns no time.
func addAppSecond() {
	if isPaused() {
		return
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	if currentApp == "" || store.Settings == nil || store.Settings.AppTrackingOff {
		return
	}
	threshold := time.Duration(idleMinutesOrDefault(store.Settings.IdleMinutes)) * time.Minute
	if time.Since(time.Unix(0, lastEvent.Load())) > threshold {
		return
	}
	ac := appEntryLocked(time.Now(), currentApp)
	ac.Secs++
}

// appEntryLocked returns the counter for name on today's day record. storeMu must be held.
func appEntryLocked(now time.Time, name string) *AppCount {
	d := dayFor(now)
	if d.Apps == nil {
		d.Apps = map[string]*AppCount{}
	}
	ac := d.Apps[name]
	if ac == nil {
		ac = &AppCount{}
		d.Apps[name] = ac
	}
	return ac
}

// addAppInputLocked credits one input event to the focused program. kind is key, mouse or scroll.
// storeMu must be held (record() calls this).
func addAppInputLocked(now time.Time, kind string) {
	if currentApp == "" || store.Settings == nil || store.Settings.AppTrackingOff {
		return
	}
	ac := appEntryLocked(now, currentApp)
	switch kind {
	case "key":
		ac.Keys++
	case "mouse":
		ac.Clicks++
	case "scroll":
		ac.Scroll++
	}
}

// appsJSON is the body of GET /api/apps: per-program totals for the chosen range.
func appsJSON(q map[string][]string) map[string]any {
	start, end, label := rangeBounds(q)
	limit := 20
	if v := first(q, "limit", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = clampInt(n, 1, 100)
		}
	}
	type row struct {
		name                 string
		keys, clicks, scroll uint64
		secs                 uint64
	}
	agg := map[string]*row{}
	var totalKeys, totalClicks, totalScroll uint64
	storeMu.RLock()
	for k, d := range store.Days {
		t, err := time.ParseInLocation("2006-01-02", k, time.Local)
		if err != nil || t.Before(start) || !t.Before(end) {
			continue
		}
		totalKeys += d.Keys
		totalClicks += d.Clicks
		totalScroll += d.Scrolls
		for name, ac := range d.Apps {
			r := agg[name]
			if r == nil {
				r = &row{name: name}
				agg[name] = r
			}
			r.keys += ac.Keys
			r.clicks += ac.Clicks
			r.scroll += ac.Scroll
			r.secs += ac.Secs
		}
	}
	// The running minute is not in the day totals yet (it is folded when the minute ends),
	// but the per-program counts already include it. Add it so the two sides agree.
	if !cur.start.IsZero() && !cur.start.Before(start) && cur.start.Before(end) {
		totalKeys += cur.keys
		totalClicks += cur.clicks
		totalScroll += cur.scroll
	}
	tracking := !store.Settings.AppTrackingOff
	storeMu.RUnlock()

	rows := make([]*row, 0, len(agg))
	var appKeys uint64
	for _, r := range agg {
		rows = append(rows, r)
		appKeys += r.keys
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].keys+rows[i].clicks+rows[i].scroll, rows[j].keys+rows[j].clicks+rows[j].scroll
		if a != b {
			return a > b
		}
		return rows[i].name < rows[j].name
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	apps := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		apps = append(apps, map[string]any{"name": r.name, "keys": r.keys, "clicks": r.clicks, "scroll": r.scroll, "secs": r.secs})
	}
	var other uint64
	if totalKeys > appKeys {
		other = totalKeys - appKeys
	}
	return map[string]any{
		"range": label, "tracking": tracking,
		"total_keys": totalKeys, "total_clicks": totalClicks, "total_scroll": totalScroll,
		"apps": apps, "other_keys": other,
	}
}
