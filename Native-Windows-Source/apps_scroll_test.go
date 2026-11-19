package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// v2.3.0: scrolls per program in "By application".

// Programs with only keys, only clicks, only scrolls, all three, and zero scrolls.
func TestPerAppScrollVariety(t *testing.T) {
	setupTestStore(t)
	setCurrentApp("keys.exe")
	record("key", "KeyA", 0)
	setCurrentApp("clicks.exe")
	record("mouse", "left", 0)
	setCurrentApp("scrolls.exe")
	record("scroll", "down", 3) // one scroll event of 3 wheel steps
	record("scroll", "up", 1)
	setCurrentApp("all.exe")
	record("key", "KeyB", 0)
	record("mouse", "right", 0)
	record("scroll", "down", 1)
	out := todayApps(t)
	want := map[string][3]uint64{"keys.exe": {1, 0, 0}, "clicks.exe": {0, 1, 0}, "scrolls.exe": {0, 0, 2}, "all.exe": {1, 1, 1}}
	for name, w := range want {
		r := rowFor(out, name)
		if r == nil {
			t.Fatalf("%s missing: %v", name, out)
		}
		if r["keys"] != w[0] || r["clicks"] != w[1] || r["scroll"] != w[2] {
			t.Errorf("%s = keys %v clicks %v scroll %v, want %v", name, r["keys"], r["clicks"], r["scroll"], w)
		}
	}
	// the global scroll total is not double counted by the per-program entries
	if out["total_scroll"] != uint64(3) {
		t.Fatalf("total_scroll = %v, want 3 (2 + 1, each event once)", out["total_scroll"])
	}
}

// Scroll attribution follows focus; the numbers survive a save and reload.
func TestPerAppScrollSurvivesReload(t *testing.T) {
	setupTestStore(t)
	setCurrentApp("chrome.exe")
	record("scroll", "down", 1)
	record("scroll", "down", 1)
	setCurrentApp("telegram.exe")
	record("scroll", "up", 1)
	if err := saveStore(); err != nil {
		t.Fatal(err)
	}
	// Reload from the saved file the way loadStore does (read, unmarshal, normalize), without
	// opening the data folder's log, so the test never touches the real data folder.
	b, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	var fresh Store
	if err := json.Unmarshal(b, &fresh); err != nil {
		t.Fatal(err)
	}
	fresh.normalize()
	storeMu.Lock()
	store = fresh
	storeMu.Unlock()
	out := todayApps(t)
	if r := rowFor(out, "chrome.exe"); r == nil || r["scroll"] != uint64(2) {
		t.Fatalf("chrome after reload: %v", r)
	}
	if r := rowFor(out, "telegram.exe"); r == nil || r["scroll"] != uint64(1) {
		t.Fatalf("telegram after reload: %v", r)
	}
	if out["total_scroll"] != uint64(3) {
		t.Fatalf("total_scroll after reload = %v", out["total_scroll"])
	}
}

// Old data (program entries written without scrolls) shows zero scrolls, never a guess.
func TestOldProgramEntriesHaveZeroScrolls(t *testing.T) {
	setupTestStore(t)
	storeMu.Lock()
	d := dayFor(time.Now())
	d.Apps = map[string]*AppCount{"old.exe": {Keys: 5, Clicks: 2}}
	storeMu.Unlock()
	r := rowFor(todayApps(t), "old.exe")
	if r == nil || r["scroll"] != uint64(0) || r["keys"] != uint64(5) {
		t.Fatalf("old entry: %v", r)
	}
}
