package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	appName    = "KeyClick Tracker"
	appVersion = "2.4.0"
	// The data folder keeps the original name on purpose: users upgrading from
	// KeyScope 1.x keep their existing activity.json without any migration.
	dataFolderName = "KeyScope"
	// storeVersion 4 (v2.0.0) only adds optional fields (idle_sec, idle_minutes,
	// hotkey_off). Version 3 files load unchanged; see V2.0.0-PLAN.md section 5.
	storeVersion = 4
)

// ---------------------------------------------------------------- data model

type HourBucket struct {
	Keys       map[string]uint64 `json:"keys"`
	Mouse      map[string]uint64 `json:"mouse"`
	Scroll     map[string]uint64 `json:"scroll"`
	ScrollUnit map[string]uint64 `json:"scroll_units"`
}

// DayRoll is a compact per-day summary of minute-level activity. It exists so
// WPM / CPM can be computed without keeping a record for every minute.
type DayRoll struct {
	Keys      uint64  `json:"keys"`
	Chars     uint64  `json:"chars"`
	Words     uint64  `json:"words"` // real word count (v1.2.0+); older days have 0
	Clicks    uint64  `json:"clicks"`
	Scrolls   uint64  `json:"scrolls"`
	ActiveMin uint32  `json:"active_min"`
	TypeMin   uint32  `json:"type_min"`
	ClickMin  uint32  `json:"click_min"`
	WPMSum    float64 `json:"wpm_sum"`
	CPMSum    float64 `json:"cpm_sum"`
	PeakWPM   float64 `json:"peak_wpm"`
	PeakCPM   float64 `json:"peak_cpm"`
	IdleSec   uint64  `json:"idle_sec,omitempty"` // v2.0.0: idle seconds (pauses longer than the idle threshold)
	// v2.0.0 (F1): counts per program file name. Names only, never titles or paths.
	Apps map[string]*AppCount `json:"apps,omitempty"`
	// v2.0.0 (F4): Ctrl/Alt/Win combinations and the Ctrl/Alt/Win presses themselves.
	Shortcuts uint64 `json:"shortcuts,omitempty"`
	ModKeys   uint64 `json:"mod_keys,omitempty"`
	// v2.0.0 (F6): the busiest 10 minutes of the day (keys + clicks + scrolls).
	Peak10 uint64 `json:"peak10,omitempty"`
	// v2.0.0 (F14): key presses per keyboard layout name.
	Layouts map[string]uint64 `json:"layouts,omitempty"`
	// v2.4.0: measured key and mouse-button holds (by key / button) and scroll sessions.
	KeyHold map[string]*HoldStat `json:"kh,omitempty"`
	MsHold  map[string]*HoldStat `json:"mh,omitempty"`
	Scr     *ScrollAgg           `json:"ss,omitempty"`
}

// PracticeScore is the result of one typing practice lesson (F8). Scores only: the typed text
// is never stored.
type PracticeScore struct {
	At     string  `json:"at"` // RFC 3339
	Lesson string  `json:"lesson"`
	WPM    float64 `json:"wpm"`
	Acc    float64 `json:"acc"` // percent 0..100
	Errors int     `json:"errors"`
	Secs   int     `json:"secs"`
}

// AppCount is what one program received on one day.
type AppCount struct {
	Keys   uint64 `json:"k,omitempty"`
	Clicks uint64 `json:"c,omitempty"`
	Scroll uint64 `json:"s,omitempty"`
	Secs   uint64 `json:"t,omitempty"` // seconds in the foreground while input was recent
	// v2.4.0: per-key and per-button counts and measured holds, scroll sessions, last input time.
	KeyIDs  map[string]uint64    `json:"ki,omitempty"`
	MsIDs   map[string]uint64    `json:"mi,omitempty"`
	KeyHold map[string]*HoldStat `json:"kh,omitempty"`
	MsHold  map[string]*HoldStat `json:"mh,omitempty"`
	Sess    uint64               `json:"ss,omitempty"`
	SessMs  uint64               `json:"sm,omitempty"`
	ScDir   map[string]uint64    `json:"sd,omitempty"`
	Last    int64                `json:"lt,omitempty"` // unix seconds of the last input
}

type Settings struct {
	Theme        string `json:"theme"`
	Font         string `json:"font"`
	FontScale    int    `json:"font_scale"` // percent, 85..130
	Animations   bool   `json:"animations"`
	LiveNumbers  bool   `json:"live_numbers"`
	GoalKeys     uint64 `json:"goal_keys"`
	GoalClicks   uint64 `json:"goal_clicks"`
	BreakMinutes int    `json:"break_minutes"` // 0 = off
	StartHidden  bool   `json:"start_hidden"`
	UserName     string `json:"user_name"`
	Clock24      bool   `json:"clock_24h"`
	TypingUnit   string `json:"typing_unit"` // wps | wpm | wph
	ClickUnit    string `json:"click_unit"`  // cps | cpm | cph
	OverlayOn    bool   `json:"overlay_on"`
	OverlaySet   bool   `json:"overlay_set"` // true once the user has positioned the overlay
	OverlayX     int    `json:"overlay_x"`
	OverlayY     int    `json:"overlay_y"`
	OverlayW     int    `json:"overlay_w"`
	OverlayH     int    `json:"overlay_h"`
	OverlayTheme string `json:"overlay_theme"` // colour theme of the floating live box, see ui/themes.js KC_OV_THEMES
	// v2.1.0 addition: file name of the chosen logo in ui/logos ("" = default). Optional.
	AppLogo string `json:"app_logo,omitempty"`
	// v2.2.0 addition: Lite mode lowers CPU and memory use (no animations or effects, a lighter
	// browser engine after the next start). Optional; default off.
	LiteMode bool `json:"lite_mode,omitempty"`
	// v2.0.0 additions. Optional: an old file simply reads zero values.
	IdleMinutes int  `json:"idle_minutes,omitempty"` // pause length that counts as idle; 0 = default 5 (range 1..60)
	HotkeyOff   bool `json:"hotkey_off,omitempty"`   // global hotkeys are on unless this is true (default on)
	// AppTrackingOff: per-program counting is on unless this is true (default on, F1).
	AppTrackingOff bool `json:"app_tracking_off,omitempty"`
	// F9 scheduled backups: how many daily copies to keep (7..90, default 14).
	BackupDays int `json:"backup_days,omitempty"`
	// F5 break reminders: "" or "simple" (BreakMinutes) or "cycle" (WorkMinutes then RestMinutes).
	ReminderMode string `json:"reminder_mode,omitempty"`
	WorkMinutes  int    `json:"work_minutes,omitempty"`
	RestMinutes  int    `json:"rest_minutes,omitempty"`
	// F7 typing-speed goal (words per minute, 0 = off).
	GoalWPM int `json:"goal_wpm,omitempty"`
	// F3 reports: "" (off), "daily" (18:00, today so far) or "weekly" (Monday 09:00, last week).
	ReportMode string `json:"report_mode,omitempty"`
	ReportFile bool   `json:"report_file,omitempty"` // also save the report as a .md file
	LastReport string `json:"last_report,omitempty"` // period key of the last report made
	// F13 live box presets: compact, detailed (default) or sparkline, with a size per preset.
	OverlayPreset string            `json:"overlay_preset,omitempty"`
	OverlaySizes  map[string][2]int `json:"overlay_sizes,omitempty"`
	// F6 strain signals (off by default) and their optional once-a-day tray note.
	StrainOn     bool `json:"strain_on,omitempty"`
	StrainNotify bool `json:"strain_notify,omitempty"`
	// A2 quiet hours: counting is paused between QuietFrom and QuietTo (not deleted).
	QuietOn   bool   `json:"quiet_on,omitempty"`
	QuietFrom string `json:"quiet_from,omitempty"`
	QuietTo   string `json:"quiet_to,omitempty"`
	// F14: counting by keyboard layout is on unless this is true.
	LayoutOff bool `json:"layout_off,omitempty"`
	// F6: the day the last strain note was shown (one tray note per day).
	LastStrainNote string `json:"last_strain_note,omitempty"`
}

func defaultSettings() *Settings {
	return &Settings{Theme: "classic", Font: "segoe", FontScale: 100, Animations: true, LiveNumbers: true, GoalKeys: 8000, GoalClicks: 1500,
		Clock24: false, TypingUnit: "wpm", ClickUnit: "cpm", OverlayW: 250, OverlayH: 92, OverlayTheme: "ink", IdleMinutes: 5}
}

type Store struct {
	Version     int                    `json:"version"`
	AppVersion  string                 `json:"app_version,omitempty"`
	CloseToTray bool                   `json:"close_to_tray"`
	Buckets     map[string]*HourBucket `json:"buckets"`
	Days        map[string]*DayRoll    `json:"days,omitempty"`
	Settings    *Settings              `json:"settings,omitempty"`
	// v2.0.0: goals in force per day with the met flags (F7), and practice scores (F8).
	GoalLog  map[string]*GoalDay `json:"goal_log,omitempty"`
	Practice []PracticeScore     `json:"practice,omitempty"`
	// v2.4.0: the newest interval records (key holds, button holds, scroll sessions) for History.
	Recent []Interval `json:"recent,omitempty"`
}

type minuteAcc struct {
	start                       time.Time
	keys, chars, clicks, scroll uint64
}

type ringSlot struct {
	sec                int64
	chars, clicks, key uint64
}

// pendingDelta is what has happened since the last live frame was pushed.
type pendingDelta struct {
	Keys   uint64            `json:"k"`
	Chars  uint64            `json:"ch"`
	Words  uint64            `json:"wd"`
	Clicks uint64            `json:"c"`
	Scroll uint64            `json:"s"`
	Units  uint64            `json:"u"`
	KeyIDs map[string]uint64 `json:"ki,omitempty"`
	MsIDs  map[string]uint64 `json:"mi,omitempty"`
	ScIDs  map[string]uint64 `json:"si,omitempty"`
}

func (p *pendingDelta) empty() bool { return p.Keys+p.Clicks+p.Scroll == 0 }

var (
	store      Store
	storeMu    sync.RWMutex
	dataDir    string
	dataPath   string
	logger     *log.Logger
	loadFailed bool
	dirty      atomic.Bool

	cur        minuteAcc
	ring       [16]ringSlot
	pend       pendingDelta
	sessStart  = time.Now()
	sessKeys   uint64
	sessChars  uint64
	sessClicks uint64
	sessScroll uint64
	sessActive uint32
	wordInside bool // word counter state: true while inside a word (no text is kept)
	// lastRecorded is the time of the previous counted event (guarded by storeMu).
	// Zero means "no anchor": the first event after start, pause or resume adds no idle time.
	lastRecorded time.Time
	// lastSaveUnix is the Unix time of the last successful save (atomic; read by /api/health).
	lastSaveUnix atomic.Int64
	signalLive   = make(chan struct{}, 1)
)

// resetIdleAnchor forgets the previous event time. It is called on resume from sleep and
// when counting restarts after a pause, so time away from the PC is not recorded as idle.
func resetIdleAnchor() {
	storeMu.Lock()
	lastRecorded = time.Time{}
	storeMu.Unlock()
}

func logLine(s string) {
	if logger != nil {
		logger.Println(s)
	}
}

func nowStamp() string { return time.Now().Format("20060102-150405") }

// ------------------------------------------------------------------ helpers

func isCharKey(id string) bool {
	if len(id) == 1 {
		return true
	}
	switch id {
	case "Space", "Semicolon", "Equal", "Comma", "Minus", "Period", "Slash", "Backquote", "BracketLeft", "Backslash", "BracketRight", "Quote", "IntlBackslash",
		"NumpadAdd", "NumpadSubtract", "NumpadMultiply", "NumpadDivide", "NumpadDecimal":
		return true
	}
	return strings.HasPrefix(id, "Numpad") && len(id) == 7
}

func newBucket() *HourBucket {
	return &HourBucket{Keys: map[string]uint64{}, Mouse: map[string]uint64{}, Scroll: map[string]uint64{}, ScrollUnit: map[string]uint64{}}
}

func (b *HourBucket) fix() {
	if b.Keys == nil {
		b.Keys = map[string]uint64{}
	}
	if b.Mouse == nil {
		b.Mouse = map[string]uint64{}
	}
	if b.Scroll == nil {
		b.Scroll = map[string]uint64{}
	}
	if b.ScrollUnit == nil {
		b.ScrollUnit = map[string]uint64{}
	}
}

func (s *Store) normalize() {
	if s.Buckets == nil {
		s.Buckets = map[string]*HourBucket{}
	}
	for k, b := range s.Buckets {
		if b == nil {
			delete(s.Buckets, k)
			continue
		}
		b.fix()
	}
	if s.Days == nil {
		s.Days = map[string]*DayRoll{}
	}
	if s.Settings == nil {
		s.Settings = defaultSettings()
	}
	st := s.Settings
	if st.Theme == "" {
		st.Theme = "classic"
	}
	if st.Font == "" {
		st.Font = "segoe"
	}
	if st.FontScale < 85 || st.FontScale > 130 {
		st.FontScale = 100
	}
	if st.BreakMinutes < 0 || st.BreakMinutes > 480 {
		st.BreakMinutes = 0
	}
	switch st.TypingUnit {
	case "wps", "wpm", "wph":
	default:
		st.TypingUnit = "wpm"
	}
	switch st.ClickUnit {
	case "cps", "cpm", "cph":
	default:
		st.ClickUnit = "cpm"
	}
	if r := []rune(strings.TrimSpace(st.UserName)); len(r) > 32 {
		st.UserName = string(r[:32])
	} else {
		st.UserName = strings.TrimSpace(st.UserName)
	}
	if st.OverlayTheme == "" || len(st.OverlayTheme) > 40 {
		st.OverlayTheme = "ink"
	}
	if !validLogo(st.AppLogo) {
		st.AppLogo = "" // an unknown logo name falls back to the default
	}
	if st.OverlayW < 150 || st.OverlayW > 900 {
		st.OverlayW = 250
	}
	if st.OverlayH < 56 || st.OverlayH > 500 {
		st.OverlayH = 92
	}
	// v2.0.0 (B14): goals were not range-checked before. Keep them at sane values.
	st.GoalKeys = min(st.GoalKeys, 10_000_000)
	st.GoalClicks = min(st.GoalClicks, 10_000_000)
	st.IdleMinutes = idleMinutesOrDefault(st.IdleMinutes)
	// v2.0.0 additions: every value is kept in range.
	st.BackupDays = clampInt(orDefault(st.BackupDays, 14), 7, 90)
	switch st.ReminderMode {
	case "", "simple", "cycle":
	default:
		st.ReminderMode = ""
	}
	st.WorkMinutes = clampInt(orDefault(st.WorkMinutes, 50), 10, 180)
	st.RestMinutes = clampInt(orDefault(st.RestMinutes, 10), 1, 60)
	st.GoalWPM = clampInt(st.GoalWPM, 0, 300)
	switch st.ReportMode {
	case "", "daily", "weekly":
	default:
		st.ReportMode = ""
	}
	if len(st.LastReport) > 40 {
		st.LastReport = ""
	}
	switch st.OverlayPreset {
	case "compact", "detailed", "sparkline":
	default:
		st.OverlayPreset = "detailed"
	}
	for k, sz := range st.OverlaySizes {
		if (k != "compact" && k != "detailed" && k != "sparkline") || sz[0] < 150 || sz[0] > 900 || sz[1] < 56 || sz[1] > 500 {
			delete(st.OverlaySizes, k)
		}
	}
	if _, ok := parseHM(st.QuietFrom); !ok {
		st.QuietFrom = "23:00"
	}
	if _, ok := parseHM(st.QuietTo); !ok {
		st.QuietTo = "07:00"
	}
	if s.GoalLog == nil {
		s.GoalLog = map[string]*GoalDay{}
	}
	if len(s.Practice) > 500 { // keep the latest 500 lesson scores
		s.Practice = s.Practice[len(s.Practice)-500:]
	}
}

// orDefault returns def when v is zero (an old file has no value for a new setting).
func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func dayKey(t time.Time) string { return t.In(time.Local).Format("2006-01-02") }

// localHourStart is the start of the local clock hour. time.Truncate(time.Hour) rounds to UTC
// hours, so in a time zone with a half-hour offset (India, UTC+5:30) it gave buckets that start
// at :30, and the hour-of-day charts were shifted by half an hour. Fixed in v2.0.0 (B17); older
// buckets are kept as they are, new activity uses the corrected key.
func localHourStart(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.Local)
}

// ------------------------------------------------------------ recording path

// record is called from the low-level hooks, so it must stay fast.
func record(kind, id string, units uint64) {
	if isPaused() {
		return
	}
	now := time.Now()
	storeMu.Lock()
	hourKey := localHourStart(now).Format(time.RFC3339)
	b := store.Buckets[hourKey]
	if b == nil {
		b = newBucket()
		store.Buckets[hourKey] = b
	}
	rollMinuteLocked(now, false)
	// v2.0.0 idle tracking: the pause since the previous event, beyond the threshold, is idle.
	if !lastRecorded.IsZero() {
		threshold := time.Duration(idleMinutesOrDefault(store.Settings.IdleMinutes)) * time.Minute
		for key, d := range idleSplit(lastRecorded, now, threshold) {
			if dr := store.Days[key]; dr != nil {
				dr.IdleSec += uint64(d / time.Second)
			} else {
				store.Days[key] = &DayRoll{IdleSec: uint64(d / time.Second)}
			}
		}
	}
	lastRecorded = now
	var chars, keys, clicks, scrolls uint64
	switch kind {
	case "key":
		b.Keys[id]++
		keys = 1
		if isCharKey(id) {
			chars = 1
		}
		pend.Keys++
		pend.Chars += chars
		if pend.KeyIDs == nil {
			pend.KeyIDs = map[string]uint64{}
		}
		pend.KeyIDs[id]++
		countWord(id)
	case "mouse":
		wordInside = false // a click ends the current word
		b.Mouse[id]++
		clicks = 1
		pend.Clicks++
		if pend.MsIDs == nil {
			pend.MsIDs = map[string]uint64{}
		}
		pend.MsIDs[id]++
	case "scroll":
		b.Scroll[id]++
		b.ScrollUnit[id] += units
		scrolls = 1
		pend.Scroll++
		pend.Units += units
		if pend.ScIDs == nil {
			pend.ScIDs = map[string]uint64{}
		}
		pend.ScIDs[id]++
	}
	addAppInputLocked(now, kind, id) // v2.0.0 (F1): credit the focused program (no-op when tracking is off)
	addLayoutInputLocked(now, kind) // v2.0.0 (F14): credit the keyboard layout in use
	if cur.keys+cur.clicks+cur.scroll == 0 {
		sessActive++
	}
	cur.keys += keys
	cur.chars += chars
	cur.clicks += clicks
	cur.scroll += scrolls
	sessKeys += keys
	sessChars += chars
	sessClicks += clicks
	sessScroll += scrolls
	sec := now.Unix()
	slot := &ring[sec%int64(len(ring))]
	if slot.sec != sec {
		*slot = ringSlot{sec: sec}
	}
	slot.chars += chars
	slot.clicks += clicks
	slot.key += keys
	dirty.Store(true)
	storeMu.Unlock()
	select {
	case signalLive <- struct{}{}:
	default:
	}
}

// wordNeutral keys do not end a word: modifiers, editing keys and the apostrophe.
var wordNeutral = map[string]bool{
	"ShiftLeft": true, "ShiftRight": true, "ControlLeft": true, "ControlRight": true, "AltLeft": true, "AltRight": true,
	"MetaLeft": true, "MetaRight": true, "CapsLock": true, "NumLock": true, "ScrollLock": true,
	"Backspace": true, "Delete": true, "Quote": true,
}

// isWordChar reports whether the key is a letter or digit (top row or numpad).
func isWordChar(id string) bool {
	if len(id) == 1 {
		c := id[0]
		return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
	}
	return len(id) == 7 && strings.HasPrefix(id, "Numpad") && id[6] >= '0' && id[6] <= '9'
}

// countWord is the hidden word engine. It keeps one flag and one counter; it never
// stores what was typed. A word starts at a letter or digit that follows a break
// (space, Enter, punctuation, arrows, Escape, a mouse click). Runs under storeMu.
func countWord(id string) {
	switch {
	case isWordChar(id):
		if wordInside {
			return
		}
		wordInside = true
		pend.Words++
		dayFor(time.Now()).Words++
	case wordNeutral[id]:
	default:
		wordInside = false
	}
}

// rollMinuteLocked folds the finished minute into its day summary. When force
// is true (shutdown) the running minute is folded regardless of the clock.
func rollMinuteLocked(now time.Time, force bool) {
	ms := now.Truncate(time.Minute)
	if !force && cur.start.Equal(ms) {
		return
	}
	if cur.keys+cur.clicks+cur.scroll > 0 && !cur.start.IsZero() {
		d := dayFor(cur.start)
		foldMinute(d, cur)
		trackBurst(d, cur) // F6: the day's busiest 10 minutes
	}
	cur = minuteAcc{start: ms}
}

func dayFor(t time.Time) *DayRoll {
	k := dayKey(t)
	d := store.Days[k]
	if d == nil {
		d = &DayRoll{}
		store.Days[k] = d
	}
	return d
}

func foldMinute(d *DayRoll, m minuteAcc) {
	d.Keys += m.keys
	d.Chars += m.chars
	d.Clicks += m.clicks
	d.Scrolls += m.scroll
	d.ActiveMin++
	if m.chars >= 5 {
		w := float64(m.chars) / 5
		d.TypeMin++
		d.WPMSum += w
		if w > d.PeakWPM {
			d.PeakWPM = w
		}
	}
	if m.clicks >= 1 {
		c := float64(m.clicks)
		d.ClickMin++
		d.CPMSum += c
		if c > d.PeakCPM {
			d.PeakCPM = c
		}
	}
}

// liveRates returns the current typing speed (words per minute) and click
// rate (clicks per minute) measured over a sliding window of up to 10 seconds.
func liveRates(now time.Time) (wpm, cpm, kpm float64) {
	storeMu.RLock()
	defer storeMu.RUnlock()
	sec := now.Unix()
	var chars, clicks, keys uint64
	oldest := sec
	for _, s := range ring {
		if s.sec > sec-10 && s.sec <= sec && (s.chars+s.clicks+s.key) > 0 {
			chars += s.chars
			clicks += s.clicks
			keys += s.key
			if s.sec < oldest {
				oldest = s.sec
			}
		}
	}
	span := float64(sec - oldest + 1)
	if span < 3 {
		span = 3
	}
	return float64(chars) * 60 / 5 / span, float64(clicks) * 60 / span, float64(keys) * 60 / span
}

// ---------------------------------------------------------------- persistence

func openDataDir() {
	dataDir = os.Getenv("KEYCLICK_DATA_DIR") // test override; never set in normal use
	if dataDir == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, _ = os.UserConfigDir()
		}
		dataDir = filepath.Join(base, dataFolderName)
	}
	_ = os.MkdirAll(dataDir, 0700)
	dataPath = filepath.Join(dataDir, "activity.json")
	if f, err := os.OpenFile(filepath.Join(dataDir, "keyscope.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
		logger = log.New(f, appName+" ", log.LstdFlags)
	}
}

func backupDir() string { d := filepath.Join(dataDir, "Backups"); _ = os.MkdirAll(d, 0700); return d }

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0600)
}

// backupNow copies the data file to a new, clearly named file. It never
// overwrites an existing backup.
func backupNow(label string) (string, error) {
	if _, err := os.Stat(dataPath); err != nil {
		return "", err
	}
	name := fmt.Sprintf("activity-%s-%s.json", label, nowStamp())
	dst := filepath.Join(backupDir(), name)
	for i := 2; ; i++ {
		if _, err := os.Stat(dst); err != nil {
			break
		}
		dst = filepath.Join(backupDir(), fmt.Sprintf("activity-%s-%s-%d.json", label, nowStamp(), i))
	}
	return dst, copyFile(dataPath, dst)
}

func latestBackup() string {
	es, err := os.ReadDir(backupDir())
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range es {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for i := len(names) - 1; i >= 0; i-- {
		b, err := os.ReadFile(filepath.Join(backupDir(), names[i]))
		var probe Store
		if err == nil && json.Unmarshal(b, &probe) == nil && probe.Buckets != nil {
			return filepath.Join(backupDir(), names[i])
		}
	}
	return ""
}

func loadStore() {
	openDataDir()
	store = Store{Version: storeVersion, CloseToTray: true}
	b, err := os.ReadFile(dataPath)
	switch {
	case err == nil:
		var tmp Store
		if e := json.Unmarshal(b, &tmp); e != nil {
			// Corrupt file: keep it for inspection, never overwrite it silently.
			aside := dataPath + ".corrupt-" + nowStamp()
			_ = os.WriteFile(aside, b, 0600)
			logLine("activity.json unreadable (" + e.Error() + "); preserved as " + aside)
			if lb := latestBackup(); lb != "" {
				if bb, e2 := os.ReadFile(lb); e2 == nil && json.Unmarshal(bb, &tmp) == nil {
					logLine("restored from backup " + lb)
					store = tmp
				}
			}
		} else {
			store = tmp
		}
	case os.IsNotExist(err):
		logLine("no existing data file; starting fresh")
	default:
		loadFailed = true // do not risk overwriting a file we could not read
		logLine("could not read activity.json: " + err.Error())
	}
	prevVersion, prevApp := store.Version, store.AppVersion
	store.normalize()
	if !loadFailed {
		if _, e := os.Stat(dataPath); e == nil && (prevApp != appVersion || prevVersion < storeVersion) {
			if p, e := backupNow("before-v" + appVersion); e == nil {
				logLine("pre-update backup: " + p)
			}
		}
		dailyBackup()
	}
	store.Version = storeVersion
	store.AppVersion = appVersion
	cur = minuteAcc{start: time.Now().Truncate(time.Minute)}
}

func dailyBackup() {
	if _, err := os.Stat(dataPath); err != nil {
		return
	}
	dst := filepath.Join(backupDir(), "activity-daily-"+time.Now().Format("2006-01-02")+".json")
	if _, err := os.Stat(dst); err == nil {
		return
	}
	_ = copyFile(dataPath, dst)
	es, _ := os.ReadDir(backupDir())
	var daily []string
	for _, e := range es {
		if strings.HasPrefix(e.Name(), "activity-daily-") {
			daily = append(daily, e.Name())
		}
	}
	// v2.0.0 (F9): the number of daily snapshots kept is a setting (7 to 90, default 14).
	// Only activity-daily-* files are ever pruned; every other backup is kept.
	keep := 14
	if store.Settings != nil {
		keep = clampInt(orDefault(store.Settings.BackupDays, 14), 7, 90)
	}
	for _, n := range namesToPrune(daily, keep) {
		_ = os.Remove(filepath.Join(backupDir(), n))
	}
}

func saveStore() error {
	if loadFailed {
		return fmt.Errorf("data file was unreadable at start; not overwriting it")
	}
	storeMu.Lock()
	rollMinuteLocked(time.Now(), false)
	b, err := json.Marshal(&store)
	storeMu.Unlock()
	if err != nil {
		return err
	}
	tmp := dataPath + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, dataPath); err != nil { // atomic replace on Windows
		_ = os.Remove(tmp)
		return fmt.Errorf("replace statistics file: %v", err)
	}
	lastSaveUnix.Store(time.Now().Unix())
	return nil
}

func flushForExit() {
	storeMu.Lock()
	rollMinuteLocked(time.Now(), true)
	storeMu.Unlock()
	if err := saveStore(); err != nil { // B2: the exit save used to fail silently
		logLine("save at exit failed: " + err.Error())
	}
}
