package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestStore points the app at a temporary data folder with an empty store.
// No window or hook is created; record() only touches the store.
func setupTestStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	dataDir = dir
	dataPath = filepath.Join(dir, "activity.json")
	storeMu.Lock()
	store = Store{Version: storeVersion, Buckets: map[string]*HourBucket{}, Days: map[string]*DayRoll{}, Settings: defaultSettings()}
	store.normalize()
	lastRecorded = time.Time{}
	cur = minuteAcc{start: time.Now().Truncate(time.Minute)} // no counts left over from an earlier test
	currentApp = ""
	for i := range ring { // the live-speed ring is global: clear it too
		ring[i] = ringSlot{}
	}
	storeMu.Unlock()
	lastEvent.Store(0)
}

// TestRecordAddsIdleAfterPause checks the real record() path: a 20 minute pause with the
// default 5 minute threshold must add about 15 minutes (900 s) of idle time for today.
func TestRecordAddsIdleAfterPause(t *testing.T) {
	setupTestStore(t)
	storeMu.Lock()
	lastRecorded = time.Now().Add(-20 * time.Minute)
	storeMu.Unlock()
	record("key", "KeyA", 0)
	got := store.Days[dayKey(time.Now())].IdleSec
	if got < 898 || got > 901 {
		t.Fatalf("idle after 20 min pause = %d s, want about 900", got)
	}
}

// TestRecordNoIdleWithoutAnchor: the first event after start adds no idle time (C1.4 rule).
func TestRecordNoIdleWithoutAnchor(t *testing.T) {
	setupTestStore(t)
	record("key", "KeyA", 0)
	if d := store.Days[dayKey(time.Now())]; d != nil && d.IdleSec != 0 {
		t.Fatalf("idle with no anchor = %d, want 0", d.IdleSec)
	}
}

// TestResetIdleAnchorAfterResume: after resetIdleAnchor (resume from sleep or pause), a long
// gap must not be counted.
func TestResetIdleAnchorAfterResume(t *testing.T) {
	setupTestStore(t)
	storeMu.Lock()
	lastRecorded = time.Now().Add(-3 * time.Hour)
	storeMu.Unlock()
	resetIdleAnchor()
	record("mouse", "left", 0)
	if d := store.Days[dayKey(time.Now())]; d != nil && d.IdleSec != 0 {
		t.Fatalf("idle after resume = %d, want 0", d.IdleSec)
	}
}

// TestIdleStatsRange checks GET /api/idle output for "today" and that the setting is reported.
func TestIdleStatsRange(t *testing.T) {
	setupTestStore(t)
	storeMu.Lock()
	lastRecorded = time.Now().Add(-30 * time.Minute)
	store.Settings.IdleMinutes = 10
	storeMu.Unlock()
	record("key", "KeyB", 0)
	out := idleStats(map[string][]string{"range": {"today"}})
	idle, _ := out["idle_sec"].(uint64)
	if idle < 1198 || idle > 1201 { // 30 min minus 10 min threshold = 1200 s
		t.Fatalf("idle_sec = %v, want about 1200", out["idle_sec"])
	}
	if out["idle_minutes_setting"] != 10 {
		t.Fatalf("idle_minutes_setting = %v, want 10", out["idle_minutes_setting"])
	}
}

// TestHealthStatesValidAndBroken: a readable file gives ok; an unreadable file gives red.
func TestHealthStatesValidAndBroken(t *testing.T) {
	setupTestStore(t)
	b, _ := json.Marshal(&store)
	if err := os.WriteFile(dataPath, b, 0600); err != nil {
		t.Fatal(err)
	}
	// The "input" check is red in a test process, because no raw input is registered here.
	// That is correct behaviour, so the file checks are judged on their own.
	fileLevel := func(h map[string]any, id string) string {
		for _, c := range h["checks"].([]map[string]string) {
			if c["id"] == id {
				return c["level"]
			}
		}
		return "missing"
	}
	h := healthStats()
	if l := fileLevel(h, "file_parses"); l != levelOK {
		t.Fatalf("valid file: file_parses = %s, want ok", l)
	}
	if l := fileLevel(h, "file_size"); l != levelOK {
		t.Fatalf("valid file: file_size = %s, want ok", l)
	}
	if err := os.WriteFile(dataPath, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	h = healthStats()
	if l := fileLevel(h, "file_parses"); l != levelRed {
		t.Fatalf("broken file: file_parses = %s, want red", l)
	}
	if h["status"] != levelRed {
		t.Fatalf("broken file: overall status = %v, want red", h["status"])
	}
	// The check must never change the file (C10.5): the broken file is still exactly what was written.
	back, _ := os.ReadFile(dataPath)
	if string(back) != "{not json" {
		t.Fatalf("health check changed the statistics file")
	}
}

// TestHotkeysJSONShape: a registered main hotkey and an in-use live-box hotkey.
func TestHotkeysJSONShape(t *testing.T) {
	setupTestStore(t)
	setHotkeyStatus("registered", "in use by another program")
	out := hotkeysJSON()
	main := out["main_window"].(map[string]any)
	live := out["live_box"].(map[string]any)
	if main["registered"] != true || main["combo"] != defaultMainCombo {
		t.Fatalf("main_window = %v", main)
	}
	if live["registered"] != false || live["reason"] != "in use by another program" {
		t.Fatalf("live_box = %v", live)
	}
}

// TestNormalizeClampsGoalsAndIdle is the B14 regression test: goals and idle minutes stay in range.
func TestNormalizeClampsGoalsAndIdle(t *testing.T) {
	s := Store{Settings: &Settings{GoalKeys: 1 << 40, GoalClicks: 99_000_000, IdleMinutes: 9999}}
	s.normalize()
	if s.Settings.GoalKeys != 10_000_000 || s.Settings.GoalClicks != 10_000_000 {
		t.Fatalf("goals not clamped: %d %d", s.Settings.GoalKeys, s.Settings.GoalClicks)
	}
	if s.Settings.IdleMinutes != 60 {
		t.Fatalf("idle minutes not clamped: %d", s.Settings.IdleMinutes)
	}
}
