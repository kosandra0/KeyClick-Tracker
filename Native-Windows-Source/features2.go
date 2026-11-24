package main

// v2.0.0 features that need the store or Windows: quiet hours (A2), shortcut counters (F4),
// burst tracking (F6), keyboard layout names (F14), goal log (F7), break reminders (F5),
// scheduled backups (F9) and the display list (F15).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	shcore                = syscall.NewLazyDLL("shcore.dll")
	pMonitorFromWindow    = user32.NewProc("MonitorFromWindow")
	pGetClassNameW        = user32.NewProc("GetClassNameW")
	pEnumDisplayMonitors  = user32.NewProc("EnumDisplayMonitors")
	pGetDpiForMonitor     = shcore.NewProc("GetDpiForMonitor")
	pGetKeyboardLayout    = user32.NewProc("GetKeyboardLayout")
	pGetLocaleInfoW       = kernel32.NewProc("GetLocaleInfoW")
	pGetWindowThreadProcE = user32.NewProc("GetWindowThreadProcessId")
)

// ------------------------------------------------------------------ A2 quiet hours, F4 counters

// quietNow is true while counting is paused by quiet hours.
func quietNow() bool {
	storeMu.RLock()
	defer storeMu.RUnlock()
	s := store.Settings
	return s != nil && s.QuietOn && inQuietHours(time.Now(), s.QuietFrom, s.QuietTo)
}

// recordCombo adds one shortcut or one Ctrl/Alt/Win press to today (F4).
func recordCombo(kind string) {
	storeMu.Lock()
	d := dayFor(time.Now())
	if kind == "shortcut" {
		d.Shortcuts++
	} else {
		d.ModKeys++
	}
	dirty.Store(true)
	storeMu.Unlock()
}

// ------------------------------------------------------------------ F6 burst tracking

var burst burstTracker // guarded by storeMu

// trackBurst updates the day's busiest-10-minutes figure when a minute is folded. storeMu held.
func trackBurst(d *DayRoll, m minuteAcc) {
	if sum := burst.add(m.start.Unix()/60, m.keys+m.clicks+m.scroll); sum > d.Peak10 {
		d.Peak10 = sum
	}
}

// ------------------------------------------------------------------ F14 keyboard layout

var (
	currentLayout string // guarded by storeMu; "" = unknown
	layoutCache   sync.Map
)

// layoutName returns an English name such as "English (United States)" for a language id.
func layoutName(lang uint32) string {
	if v, ok := layoutCache.Load(lang); ok {
		return v.(string)
	}
	get := func(typ uintptr) string {
		buf := make([]uint16, 128)
		n, _, _ := pGetLocaleInfoW.Call(uintptr(lang), typ, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n == 0 {
			return ""
		}
		return syscall.UTF16ToString(buf)
	}
	name := get(0x1001)            // LOCALE_SENGLISHLANGUAGENAME
	if c := get(0x1002); c != "" { // LOCALE_SENGLISHCOUNTRYNAME
		name += " (" + c + ")"
	}
	if name == "" {
		name = "Layout " + strings.ToUpper(strconvHex(lang))
	}
	layoutCache.Store(lang, name)
	return name
}

func strconvHex(v uint32) string {
	const d = "0123456789abcdef"
	out := []byte{}
	for i := 3; i >= 0; i-- {
		out = append(out, d[(v>>(uint(i)*4))&0xF])
	}
	return string(out)
}

// updateLayout reads the keyboard layout of the window that has focus.
func updateLayout(h uintptr) {
	if h == 0 {
		return
	}
	tid, _, _ := pGetWindowThreadProcE.Call(h, 0)
	if tid == 0 {
		return
	}
	hkl, _, _ := pGetKeyboardLayout.Call(tid)
	name := layoutName(uint32(hkl) & 0xFFFF)
	storeMu.Lock()
	currentLayout = name
	storeMu.Unlock()
}

// addLayoutInputLocked credits one key press to the current layout. storeMu held.
func addLayoutInputLocked(now time.Time, kind string) {
	if kind != "key" || currentLayout == "" || store.Settings == nil || store.Settings.LayoutOff {
		return
	}
	d := dayFor(now)
	if d.Layouts == nil {
		d.Layouts = map[string]uint64{}
	}
	d.Layouts[currentLayout]++
}

// ------------------------------------------------------------------ F7 goal log

// goalTick records today's goals in force and whether each is met. Past days are not touched.
func goalTick(now time.Time) {
	storeMu.Lock()
	defer storeMu.Unlock()
	st := store.Settings
	if st == nil {
		return
	}
	key := dayKey(now)
	g := store.GoalLog[key]
	if g == nil {
		g = &GoalDay{}
		store.GoalLog[key] = g
	}
	before := *g
	g.GK, g.GC, g.GW = st.GoalKeys, st.GoalClicks, uint64(st.GoalWPM)
	var keys, clicks uint64
	var avg float64
	if d := store.Days[key]; d != nil {
		keys, clicks = d.Keys, d.Clicks
		if d.TypeMin > 0 {
			avg = d.WPMSum / float64(d.TypeMin)
		}
	}
	if cur.start.Format("2006-01-02") == key {
		keys += cur.keys
		clicks += cur.clicks
	}
	g.evaluate(keys, clicks, avg)
	if *g != before {
		dirty.Store(true)
	}
}

func goalWatcher() {
	goalTick(time.Now())
	for range time.Tick(time.Minute) {
		goalTick(time.Now())
	}
}

// ------------------------------------------------------------------ F5 reminders

var (
	reminderMu sync.Mutex
	reminder   reminderState
)

func foregroundFullscreen() bool {
	h, _, _ := pForegroundWnd.Call()
	if h == 0 {
		return false
	}
	cls := make([]uint16, 64)
	pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&cls[0])), uintptr(len(cls)))
	switch syscall.UTF16ToString(cls) {
	case "Progman", "WorkerW", "Shell_TrayWnd": // the desktop and taskbar are not "full screen"
		return false
	}
	var wr rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr)))
	mon, _, _ := pMonitorFromWindow.Call(h, 2)
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return false
	}
	return rectCoversMonitor(wr, mi.Monitor)
}

func init() {
	snoozeAvailable = func() bool {
		reminderMu.Lock()
		defer reminderMu.Unlock()
		return reminder.canSnooze(time.Now())
	}
	snoozeReminder = func() {
		reminderMu.Lock()
		reminder.snooze(time.Now(), 5)
		reminderMu.Unlock()
	}
}

// breakWatcher raises a tray reminder after continuous activity (simple mode) or a work/rest
// cycle (cycle mode). It replaces the v1 watcher that used a fixed five-minute gap.
func breakWatcher() {
	for range time.Tick(20 * time.Second) {
		storeMu.RLock()
		st := store.Settings
		cfg := reminderCfg{Mode: st.ReminderMode, BreakMin: st.BreakMinutes, WorkMin: st.WorkMinutes, RestMin: st.RestMinutes, IdleMin: st.IdleMinutes}
		storeMu.RUnlock()
		if isPaused() || quietNow() {
			continue
		}
		last := time.Unix(0, lastEvent.Load())
		if lastEvent.Load() == 0 {
			last = time.Time{}
		}
		fs := (cfg.Mode == "cycle" || cfg.BreakMin > 0) && foregroundFullscreen()
		reminderMu.Lock()
		fire, msg := reminder.step(time.Now(), last, cfg, fs)
		reminderMu.Unlock()
		if fire {
			trayNotify("Time for a short break", msg)
		}
	}
}

// ------------------------------------------------------------------ F9 scheduled backups

const autoBackupEvery = 6 * time.Hour

// autoBackupIfDue takes a copy every six hours while the app runs and keeps the latest eight.
// Daily copies keep their own prefix and are pruned by dailyBackup; no other file is touched.
func autoBackupIfDue() {
	names := backupNamesWithPrefix("activity-auto-")
	if len(names) > 0 {
		newest := filepath.Join(backupDir(), names[len(names)-1])
		if fi, err := os.Stat(newest); err == nil && time.Since(fi.ModTime()) < autoBackupEvery {
			return
		}
	}
	if err := saveStore(); err != nil {
		return // a failed load or save is never backed up as if it were good data
	}
	if p, err := backupNow("auto"); err == nil {
		logLine("auto backup: " + p)
	}
	for _, n := range namesToPrune(backupNamesWithPrefix("activity-auto-"), 8) {
		_ = os.Remove(filepath.Join(backupDir(), n))
	}
}

// backupNamesWithPrefix lists backup file names that start with prefix, oldest first.
func backupNamesWithPrefix(prefix string) []string {
	es, _ := os.ReadDir(backupDir())
	var out []string
	for _, e := range es {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func backupWatcher() {
	time.Sleep(45 * time.Second)
	autoBackupIfDue()
	for range time.Tick(10 * time.Minute) {
		autoBackupIfDue()
	}
}

// ------------------------------------------------------------------ F15 displays

var monitorEnumCB uintptr
var monitorsOut []map[string]any

func displaysJSON() map[string]any {
	monitorsOut = nil
	if monitorEnumCB == 0 {
		monitorEnumCB = syscall.NewCallback(func(hm, hdc, rc, lp uintptr) uintptr {
			var mi monitorInfo
			mi.Size = uint32(unsafe.Sizeof(mi))
			if r, _, _ := pGetMonitorInfoW.Call(hm, uintptr(unsafe.Pointer(&mi))); r == 0 {
				return 1
			}
			var dx, dy uint32 = 96, 96
			pGetDpiForMonitor.Call(hm, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))) // MDT_EFFECTIVE_DPI
			monitorsOut = append(monitorsOut, map[string]any{
				"x": mi.Monitor.Left, "y": mi.Monitor.Top, "w": mi.Monitor.Right - mi.Monitor.Left, "h": mi.Monitor.Bottom - mi.Monitor.Top,
				"dpi": dx, "scale_percent": int(dx) * 100 / 96, "primary": mi.Flags&1 != 0,
			})
			return 1
		})
	}
	pEnumDisplayMonitors.Call(0, 0, monitorEnumCB, 0)
	out := map[string]any{"monitors": monitorsOut, "count": len(monitorsOut)}
	storeMu.RLock()
	st := store.Settings
	ov := map[string]any{"on": st.OverlayOn, "x": st.OverlayX, "y": st.OverlayY, "w": st.OverlayW, "h": st.OverlayH}
	set := st.OverlaySet
	storeMu.RUnlock()
	if set {
		ov["on_a_screen"] = onScreen(rect{Left: int32(ov["x"].(int)), Top: int32(ov["y"].(int)), Right: int32(ov["x"].(int) + ov["w"].(int)), Bottom: int32(ov["y"].(int) + ov["h"].(int))})
	} else {
		ov["on_a_screen"] = true // not placed by the user yet: the default position is on the primary screen
	}
	out["live_box"] = ov
	return out
}

// ------------------------------------------------------------------ F13 live box presets

// presetDefaults are the first sizes of each preset (width, height in dip).
var presetDefaults = map[string][2]int{"compact": {220, 72}, "detailed": {330, 92}, "sparkline": {260, 120}}

// applyOverlayPreset runs when settings are saved. When the preset changes, the size the box
// has now is remembered for the old preset and the new preset's own size is applied. Position
// is shared, so the box does not jump. storeMu is held by the caller.
func applyOverlayPreset(old, next *Settings) {
	if old == nil || next.OverlayPreset == old.OverlayPreset {
		return
	}
	switch next.OverlayPreset {
	case "compact", "detailed", "sparkline":
	default:
		return // normalize() resets an unknown preset
	}
	sizes := map[string][2]int{}
	for k, v := range old.OverlaySizes {
		sizes[k] = v
	}
	prev := old.OverlayPreset
	if prev == "" {
		prev = "detailed"
	}
	sizes[prev] = [2]int{old.OverlayW, old.OverlayH}
	next.OverlaySizes = sizes
	if sz, ok := sizes[next.OverlayPreset]; ok {
		next.OverlayW, next.OverlayH = sz[0], sz[1]
	} else {
		d := presetDefaults[next.OverlayPreset]
		next.OverlayW, next.OverlayH = d[0], d[1]
	}
}

// ------------------------------------------------------------------ A7 start-up diagnostics

var (
	bootMu    sync.Mutex
	bootLog   []map[string]string
	bootStart = time.Now()
)

// bootStep records one start-up step with its time, for the Data page and the log.
func bootStep(step string) {
	bootMu.Lock()
	bootLog = append(bootLog, map[string]string{"step": step, "at": "+" + time.Since(bootStart).Round(time.Millisecond).String()})
	bootMu.Unlock()
	logLine("start-up: " + step)
}

func bootSteps() []map[string]string {
	bootMu.Lock()
	defer bootMu.Unlock()
	return append([]map[string]string(nil), bootLog...)
}
