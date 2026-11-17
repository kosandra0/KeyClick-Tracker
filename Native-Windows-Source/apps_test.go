package main

import (
	"testing"
	"time"
)

func todayApps(t *testing.T) map[string]any {
	t.Helper()
	return appsJSON(map[string][]string{"range": {"today"}})
}

// rowFor finds one program's row in the appsJSON output.
func rowFor(out map[string]any, name string) map[string]any {
	for _, r := range out["apps"].([]map[string]any) {
		if r["name"] == name {
			return r
		}
	}
	return nil
}

// TestPerAppCounts: input goes to the program that has focus (C2.x, F1).
func TestPerAppCounts(t *testing.T) {
	setupTestStore(t)
	setCurrentApp("notepad.exe")
	for i := 0; i < 3; i++ {
		record("key", "KeyA", 0)
	}
	record("mouse", "left", 0)
	record("mouse", "left", 0)
	record("scroll", "up", 1)
	out := todayApps(t)
	r := rowFor(out, "notepad.exe")
	if r == nil {
		t.Fatalf("notepad.exe missing: %v", out)
	}
	if r["keys"] != uint64(3) || r["clicks"] != uint64(2) || r["scroll"] != uint64(1) {
		t.Fatalf("notepad counts = %v", r)
	}
	if out["other_keys"] != uint64(0) || out["total_keys"] != uint64(3) {
		t.Fatalf("sum rule broken: %v", out)
	}
}

// TestUnattributedKeysGoToOtherKeys: keys with no known program still count overall (C2.7).
func TestUnattributedKeysGoToOtherKeys(t *testing.T) {
	setupTestStore(t)
	setCurrentApp("")
	record("key", "KeyB", 0)
	setCurrentApp("chrome.exe")
	record("key", "KeyC", 0)
	out := todayApps(t)
	if out["total_keys"] != uint64(2) || out["other_keys"] != uint64(1) {
		t.Fatalf("other_keys sum rule: %v", out)
	}
}

// TestTrackingOffAddsNoProgramEntries (C2.6): totals still count, program entries do not.
func TestTrackingOffAddsNoProgramEntries(t *testing.T) {
	setupTestStore(t)
	storeMu.Lock()
	store.Settings.AppTrackingOff = true
	storeMu.Unlock()
	setCurrentApp("notepad.exe")
	record("key", "KeyA", 0)
	out := todayApps(t)
	if len(out["apps"].([]map[string]any)) != 0 {
		t.Fatalf("program entries added while tracking off: %v", out)
	}
	if out["total_keys"] != uint64(1) || out["tracking"] != false {
		t.Fatalf("totals or tracking flag wrong: %v", out)
	}
}

// TestSecondsNeedRecentInput: an idle program earns no foreground time.
func TestSecondsNeedRecentInput(t *testing.T) {
	setupTestStore(t)
	setCurrentApp("code.exe")
	lastEvent.Store(0) // no input ever
	addAppSecond()
	if r := rowFor(todayApps(t), "code.exe"); r != nil && r["secs"] != uint64(0) {
		t.Fatalf("time credited without input: %v", r)
	}
	lastEvent.Store(time.Now().UnixNano())
	addAppSecond()
	addAppSecond()
	if r := rowFor(todayApps(t), "code.exe"); r == nil || r["secs"] != uint64(2) {
		t.Fatalf("time not credited with recent input: %v", r)
	}
}

// TestRowsSortedAndLimited: highest activity first; limit respected.
func TestRowsSortedAndLimited(t *testing.T) {
	setupTestStore(t)
	setCurrentApp("a.exe")
	record("key", "KeyA", 0)
	setCurrentApp("b.exe")
	for i := 0; i < 3; i++ {
		record("key", "KeyB", 0)
	}
	out := appsJSON(map[string][]string{"range": {"today"}, "limit": {"1"}})
	rows := out["apps"].([]map[string]any)
	if len(rows) != 1 || rows[0]["name"] != "b.exe" {
		t.Fatalf("sort/limit wrong: %v", rows)
	}
}
