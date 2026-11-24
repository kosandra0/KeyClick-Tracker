package main

import (
	"strings"
	"testing"
	"time"
)

func tm(h, m int) time.Time { return time.Date(2026, 11, 16, h, m, 0, 0, time.Local) }

// ---------------------------------------------------------------- F4 shortcuts

func TestModStateShortcuts(t *testing.T) {
	now := tm(10, 0)
	var m modState
	// Ctrl down, C down -> shortcut
	m.apply(0x11, false, true, now)
	if !m.isShortcut() {
		t.Fatal("Ctrl held: want shortcut")
	}
	m.apply(0x11, false, false, now)
	if m.isShortcut() {
		t.Fatal("Ctrl released: want no shortcut")
	}
	// Shift alone is not a shortcut
	m.apply(0x10, false, true, now)
	if m.isShortcut() {
		t.Fatal("Shift only: want no shortcut")
	}
	m.apply(0x10, false, false, now)
	// left Alt (F4) and Win (E) are shortcuts
	m.apply(0x12, false, true, now)
	if !m.isShortcut() {
		t.Fatal("left Alt: want shortcut")
	}
	m.apply(0x12, false, false, now)
	m.apply(0x5B, false, true, now)
	if !m.isShortcut() {
		t.Fatal("Win: want shortcut")
	}
	m.apply(0x5B, false, false, now)
	// AltGr (Ctrl fake + right Alt) is the documented limit: not counted
	m.apply(0x11, false, true, now)
	m.apply(0x12, true, true, now)
	if m.isShortcut() {
		t.Fatal("AltGr (Ctrl + AltRight): want no shortcut (known limit)")
	}
}

func TestModStateStaleReset(t *testing.T) {
	var m modState
	m.apply(0x5B, false, true, tm(10, 0)) // Win down, key-up never arrives (Win+L)
	if !m.isShortcut() {
		t.Fatal("setup")
	}
	if m.apply(0x41, false, true, tm(10, 5)) { // a letter 5 minutes later
		t.Fatal("A is not a modifier")
	}
	if m.isShortcut() {
		t.Fatal("stale Win state was not cleared after 30 s without events")
	}
}

func TestCountedModifier(t *testing.T) {
	for vk, want := range map[uint32]bool{0x11: true, 0xA2: true, 0x12: true, 0x5B: true, 0x10: false, 0x41: false} {
		if countedModifier(vk) != want {
			t.Errorf("countedModifier(%#x) = %v, want %v", vk, !want, want)
		}
	}
}

// ---------------------------------------------------------------- F9, A2

func TestNamesToPrune(t *testing.T) {
	names := []string{"activity-daily-2026-11-03.json", "activity-daily-2026-11-01.json", "activity-daily-2026-11-02.json", "activity-daily-2026-11-04.json"}
	got := namesToPrune(names, 2)
	if len(got) != 2 || got[0] != "activity-daily-2026-11-01.json" || got[1] != "activity-daily-2026-11-02.json" {
		t.Fatalf("prune = %v, want the two oldest", got)
	}
	if len(namesToPrune(names, 4)) != 0 || len(namesToPrune(names, 10)) != 0 {
		t.Fatal("nothing should be pruned when count <= keep")
	}
	if len(namesToPrune(names, 0)) != 4 {
		t.Fatal("keep 0 prunes all")
	}
}

func TestInQuietHours(t *testing.T) {
	cases := []struct {
		now      time.Time
		from, to string
		want     bool
	}{
		{tm(23, 30), "23:00", "07:00", true},
		{tm(3, 0), "23:00", "07:00", true},
		{tm(7, 0), "23:00", "07:00", false},
		{tm(12, 0), "23:00", "07:00", false},
		{tm(9, 0), "09:00", "17:00", true},
		{tm(17, 0), "09:00", "17:00", false},
		{tm(10, 0), "10:00", "10:00", false},
		{tm(10, 0), "bad", "07:00", false},
	}
	for i, c := range cases {
		if got := inQuietHours(c.now, c.from, c.to); got != c.want {
			t.Errorf("case %d: inQuietHours(%v, %s, %s) = %v, want %v", i, c.now.Format("15:04"), c.from, c.to, got, c.want)
		}
	}
}

// ---------------------------------------------------------------- F5 reminders

func TestReminderSimpleFiresOnceThenRepeats(t *testing.T) {
	var s reminderState
	cfg := reminderCfg{BreakMin: 30, IdleMin: 5}
	start := tm(9, 0)
	step := func(at time.Time) bool { f, _ := s.step(at, at, cfg, false); return f } // input right now
	if step(start) {
		t.Fatal("fired at the start")
	}
	if step(start.Add(29 * time.Minute)) {
		t.Fatal("fired before the break time")
	}
	if !step(start.Add(31 * time.Minute)) {
		t.Fatal("did not fire after 30 minutes of continuous activity")
	}
	if step(start.Add(32 * time.Minute)) {
		t.Fatal("fired twice in a row")
	}
	if !step(start.Add(62 * time.Minute)) {
		t.Fatal("did not remind again after another 30 minutes of continuous activity")
	}
}

func TestReminderNotDuringIdleAndRestResets(t *testing.T) {
	var s reminderState
	cfg := reminderCfg{BreakMin: 30, IdleMin: 5}
	t0 := tm(9, 0)
	s.step(t0, t0, cfg, false)
	// last input at 9:25, now 9:40: a 15 minute gap is a rest (>= idle 5), so the cycle restarts
	now := t0.Add(40 * time.Minute)
	if f, _ := s.step(now, t0.Add(25*time.Minute), cfg, false); f {
		t.Fatal("fired after a long gap")
	}
	// work resumes at 9:41; 29 minutes later it must not fire (the timer restarted)
	r := t0.Add(41 * time.Minute)
	s.step(r, r, cfg, false)
	if f, _ := s.step(r.Add(29*time.Minute), r.Add(29*time.Minute), cfg, false); f {
		t.Fatal("fired too early after a rest")
	}
	if f, _ := s.step(r.Add(31*time.Minute), r.Add(31*time.Minute), cfg, false); !f {
		t.Fatal("did not fire 30 minutes after resuming")
	}
}

func TestReminderCycleModeRestLength(t *testing.T) {
	var s reminderState
	cfg := reminderCfg{Mode: "cycle", WorkMin: 50, RestMin: 10, IdleMin: 5}
	t0 := tm(9, 0)
	s.step(t0, t0, cfg, false)
	// A 7 minute pause: idle (>= 5) but not a rest (< 10): the work timer is kept, nothing fires
	if f, _ := s.step(t0.Add(30*time.Minute), t0.Add(23*time.Minute), cfg, false); f {
		t.Fatal("fired during idle time")
	}
	// back at work at 9:31, still counted from 9:00: fires at 9:50 + a bit
	if f, msg := s.step(t0.Add(51*time.Minute), t0.Add(51*time.Minute), cfg, false); !f || !strings.Contains(msg, "rest of 10 minutes") {
		t.Fatalf("cycle reminder missing: %v %q", f, msg)
	}
	// a real rest (12 minutes without input) restarts the cycle
	if f, _ := s.step(t0.Add(70*time.Minute), t0.Add(58*time.Minute), cfg, false); f {
		t.Fatal("fired during a rest")
	}
	if !s.firedAt.IsZero() || !s.workStart.IsZero() {
		t.Fatal("cycle was not reset after a rest of RestMin")
	}
}

func TestReminderSnoozeAndFullscreen(t *testing.T) {
	var s reminderState
	cfg := reminderCfg{BreakMin: 10, IdleMin: 5}
	t0 := tm(9, 0)
	s.step(t0, t0, cfg, false)
	at := t0.Add(11 * time.Minute)
	if f, _ := s.step(at, at, cfg, true); f {
		t.Fatal("fired while a full-screen program was in front")
	}
	if f, _ := s.step(at.Add(20*time.Second), at.Add(20*time.Second), cfg, false); !f {
		t.Fatal("did not fire once the full-screen program was gone")
	}
	if !s.canSnooze(at.Add(time.Minute)) {
		t.Fatal("snooze should be available right after a reminder")
	}
	now := at.Add(2 * time.Minute)
	s.snooze(now, 5)
	if f, _ := s.step(now.Add(4*time.Minute), now.Add(4*time.Minute), cfg, false); f {
		t.Fatal("fired during the snooze")
	}
	if f, _ := s.step(now.Add(6*time.Minute), now.Add(6*time.Minute), cfg, false); !f {
		t.Fatal("did not fire again after the snooze ended")
	}
}

func TestRectCoversMonitor(t *testing.T) {
	mon := rect{0, 0, 1920, 1080}
	if !rectCoversMonitor(rect{0, 0, 1920, 1080}, mon) || !rectCoversMonitor(rect{-5, -5, 1925, 1085}, mon) {
		t.Fatal("a full-screen window must cover the monitor")
	}
	if rectCoversMonitor(rect{0, 0, 1920, 1040}, mon) {
		t.Fatal("a maximized window above the taskbar is not full screen")
	}
}

// ---------------------------------------------------------------- F7 goals

func TestGoalDayStates(t *testing.T) {
	g := &GoalDay{GK: 1000, GC: 100}
	g.evaluate(1500, 50, 0)
	if g.state() != "partly" {
		t.Fatalf("state = %s, want partly", g.state())
	}
	g.evaluate(1500, 150, 0)
	if g.state() != "met" {
		t.Fatalf("state = %s, want met", g.state())
	}
	g.evaluate(10, 10, 0)
	if g.state() != "missed" {
		t.Fatalf("state = %s, want missed", g.state())
	}
	w := &GoalDay{GW: 40}
	w.evaluate(0, 0, 41.5)
	if w.state() != "met" {
		t.Fatal("WPM goal not met at 41.5 >= 40")
	}
	if (&GoalDay{}).state() != "none" {
		t.Fatal("no active goals must be none")
	}
}

func TestGoalStreakAcrossMissingDay(t *testing.T) {
	today := tm(12, 0)
	d := func(back int) string { return midnight(today).AddDate(0, 0, -back).Format("2006-01-02") }
	met := &GoalDay{GK: 1, MK: true}
	log := map[string]*GoalDay{d(0): met, d(1): met, d(2): met, d(4): met} // day 3 missing
	if n := goalStreak(log, today); n != 3 {
		t.Fatalf("streak = %d, want 3 (the missing day breaks it)", n)
	}
	// today not met yet: the streak ends yesterday and is not broken by an unfinished day
	log[d(0)] = &GoalDay{GK: 1000}
	if n := goalStreak(log, today); n != 2 {
		t.Fatalf("streak with today in progress = %d, want 2", n)
	}
}

func TestGoalHistoryNotRewrittenByGoalChange(t *testing.T) {
	setupTestStore(t)
	yesterday := midnight(time.Now()).AddDate(0, 0, -1).Format("2006-01-02")
	storeMu.Lock()
	store.GoalLog[yesterday] = &GoalDay{GK: 8000, MK: true}
	store.Settings.GoalKeys = 99999
	storeMu.Unlock()
	goalTick(time.Now())
	storeMu.RLock()
	g := store.GoalLog[yesterday]
	today := store.GoalLog[dayKey(time.Now())]
	storeMu.RUnlock()
	if g.GK != 8000 || !g.MK {
		t.Fatalf("past day was rewritten: %+v", g)
	}
	if today == nil || today.GK != 99999 {
		t.Fatalf("today must use the goal in force now: %+v", today)
	}
}

// ---------------------------------------------------------------- F6 strain

func TestBurstTrackerWindow(t *testing.T) {
	var b burstTracker
	b.add(100, 50)
	b.add(101, 20)
	if s := b.add(105, 30); s != 100 {
		t.Fatalf("10 minute sum = %d, want 100", s)
	}
	if s := b.add(111, 5); s != 35 { // minutes 100 and 101 have left the window; 105 and 111 remain
		t.Fatalf("sum after the window moved = %d, want 35", s)
	}
}

func TestStrainFlag(t *testing.T) {
	prior := make([]uint64, 20)
	for i := range prior {
		prior[i] = uint64(300 + i*10)
	}
	if f, _ := strainFlag(2000, prior[:10]); f {
		t.Fatal("flagged with fewer than 14 days of history")
	}
	if f, _ := strainFlag(200, prior); f {
		t.Fatal("flagged below the absolute floor")
	}
	if f, p := strainFlag(2000, prior); !f || p == 0 {
		t.Fatalf("not flagged at 2000 vs p90 %d", p)
	}
	if f, _ := strainFlag(310, prior); f {
		t.Fatal("flagged a normal day")
	}
}

func TestPercentile(t *testing.T) {
	v := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if p := percentile(v, 90); p != 9 {
		t.Fatalf("p90 = %d, want 9", p)
	}
	if percentile(nil, 90) != 0 {
		t.Fatal("empty input must give 0")
	}
}

// ---------------------------------------------------------------- F11, F3 helpers

func TestCSVRows(t *testing.T) {
	days := map[string]*DayRoll{
		"2026-11-02": {Keys: 5, Clicks: 2, Scrolls: 1, Words: 1, ActiveMin: 3, IdleSec: 125, Shortcuts: 4},
		"2026-11-01": {Keys: 9},
		"2026-10-01": {Keys: 1},
	}
	rows := csvDayRows(days, "2026-11-01", "2026-11-30")
	if len(rows) != 3 || rows[0][0] != "date" || rows[1][0] != "2026-11-01" || rows[2][0] != "2026-11-02" {
		t.Fatalf("rows wrong: %v", rows)
	}
	if rows[2][6] != "2" || rows[2][7] != "4" { // 125 s idle = 2 whole minutes; 4 shortcuts
		t.Fatalf("idle/shortcut columns wrong: %v", rows[2])
	}
}

func TestPercentChange(t *testing.T) {
	if percentChange(120, 100) != "+20% compared with the period before" {
		t.Fatal(percentChange(120, 100))
	}
	if percentChange(80, 100) != "-20% compared with the period before" {
		t.Fatal(percentChange(80, 100))
	}
	if percentChange(5, 0) != "no earlier data" {
		t.Fatal("zero previous must say no earlier data")
	}
}

func TestReportDue(t *testing.T) {
	monday := time.Date(2026, 11, 16, 8, 59, 0, 0, time.Local) // 2026-11-16 is a Monday
	if _, due := reportDue("weekly", "", monday); due {
		t.Fatal("weekly report due before Monday 09:00")
	}
	if p, due := reportDue("weekly", "", monday.Add(2*time.Minute)); !due || p != "week" {
		t.Fatal("weekly report not due at Monday 09:01")
	}
	tuesday := monday.AddDate(0, 0, 1).Add(5 * time.Hour) // started late in the week: still due once
	if _, due := reportDue("weekly", "", tuesday); !due {
		t.Fatal("weekly report must still fire when the app starts on Tuesday")
	}
	_, key, _ := func() (string, string, string) {
		a, b, c := buildReport("week", monday.Add(2*time.Minute))
		return a, c, b
	}()
	if _, due := reportDue("weekly", key, monday.Add(2*time.Minute)); due {
		t.Fatalf("weekly report fired twice for key %q", key)
	}
	d := time.Date(2026, 11, 16, 17, 59, 0, 0, time.Local)
	if _, due := reportDue("daily", "", d); due {
		t.Fatal("daily report due before 18:00")
	}
	if _, due := reportDue("daily", "", d.Add(2*time.Minute)); !due {
		t.Fatal("daily report not due after 18:00")
	}
	if _, due := reportDue("daily", "daily:2026-11-16", d.Add(2*time.Minute)); due {
		t.Fatal("daily report fired twice")
	}
	if _, due := reportDue("", "", d.Add(2*time.Minute)); due {
		t.Fatal("reports are off by default")
	}
}

// ---------------------------------------------------------------- F13 presets, settings ranges

func TestOverlayPresetKeepsPerPresetSize(t *testing.T) {
	old := &Settings{OverlayPreset: "detailed", OverlayW: 300, OverlayH: 110}
	next := *old
	next.OverlayPreset = "compact"
	applyOverlayPreset(old, &next)
	if next.OverlayW != 220 || next.OverlayH != 72 { // v2.5.1: compact default raised from 60 to 72 dip
		t.Fatalf("compact first size = %dx%d, want 220x72", next.OverlayW, next.OverlayH)
	}
	if got := next.OverlaySizes["detailed"]; got != [2]int{300, 110} {
		t.Fatalf("detailed size not remembered: %v", got)
	}
	// switch back: the remembered size returns
	back := next
	back.OverlayPreset = "detailed"
	applyOverlayPreset(&next, &back)
	if back.OverlayW != 300 || back.OverlayH != 110 {
		t.Fatalf("detailed size after switching back = %dx%d, want 300x110", back.OverlayW, back.OverlayH)
	}
}

func TestNormalizeNewSettingsRanges(t *testing.T) {
	s := Store{Settings: &Settings{BackupDays: 5000, WorkMinutes: 1, RestMinutes: 999, GoalWPM: 9999, ReminderMode: "weird", ReportMode: "hourly", OverlayPreset: "huge", QuietFrom: "25:99", QuietTo: "x"}}
	s.normalize()
	st := s.Settings
	if st.BackupDays != 90 || st.WorkMinutes != 10 || st.RestMinutes != 60 || st.GoalWPM != 300 {
		t.Fatalf("numbers not clamped: %+v", st)
	}
	if st.ReminderMode != "" || st.ReportMode != "" || st.OverlayPreset != "detailed" || st.QuietFrom != "23:00" || st.QuietTo != "07:00" {
		t.Fatalf("choices not reset: %+v", st)
	}
	// an old file (all zero) gets the defaults
	old := Store{Settings: &Settings{}}
	old.normalize()
	if old.Settings.BackupDays != 14 || old.Settings.WorkMinutes != 50 || old.Settings.RestMinutes != 10 {
		t.Fatalf("defaults missing for an old file: %+v", old.Settings)
	}
}
