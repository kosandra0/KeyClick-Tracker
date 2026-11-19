package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// v2.4.0: KeyClick Tracker is listed in the application list like any other program.
func TestSelfAppNameIsTheProgramFileName(t *testing.T) {
	n := selfAppName()
	exe, _ := os.Executable()
	if n == "" || n == unknownAppKey || !strings.HasSuffix(n, ".exe") {
		t.Fatalf("selfAppName = %q", n)
	}
	if n != appNameFromPath(exe) {
		t.Fatalf("selfAppName = %q, want the file name of %q", n, exe)
	}
	if strings.ContainsAny(n, `\/:`) {
		t.Fatalf("a path leaked into the name: %q", n)
	}
	// its path is remembered in memory (for the icon) and nowhere else
	if _, ok := appPaths.Load(n); !ok {
		t.Fatal("the path was not remembered for the icon")
	}
}

// Input typed into this app's own window is credited to this app, with its keys and holds.
func TestOwnWindowInputIsCountedUnderTheApp(t *testing.T) {
	resetInput(t)
	self := selfAppName()
	setCurrentApp(self)
	record("key", "KeyA", 0)
	record("mouse", "left", 0)
	recordHold(hookEvent{kind: "hold", id: "k:KeyA", units: 250, app: self, t: time.Now()})
	r := rowFor(todayApps(t), self)
	if r == nil || r["keys"] != uint64(1) || r["clicks"] != uint64(1) {
		t.Fatalf("own program row = %v", r)
	}
	if out := todayApps(t); out["other_keys"] != uint64(0) {
		t.Fatalf("the key was left unattributed: %v", out["other_keys"])
	}
}

// A hold that just ended is reported for 1.5 s so that even a quick tap shows up live.
func TestReleasedHoldIsReportedBriefly(t *testing.T) {
	resetInput(t)
	recordHold(hookEvent{kind: "hold", id: "k:A", units: 40, app: "x.exe", t: time.Now().Add(-100 * time.Millisecond)})
	recordHold(hookEvent{kind: "hold", id: "m:left", units: 30, app: "x.exe", t: time.Now().Add(-5 * time.Second)}) // old: not reported
	rel := heldJSON()["released"].([]map[string]any)
	if len(rel) != 1 || rel[0]["id"] != "A" || rel[0]["k"] != "key" || rel[0]["ms"].(uint64) != 40 {
		t.Fatalf("released = %v", rel)
	}
	if rel[0]["total_ms"].(uint64) != 40 || rel[0]["holds"].(uint64) != 1 {
		t.Fatalf("totals in released row = %v", rel[0])
	}
	// an interrupted hold never shows as released
	resetInput(t)
	recordHold(hookEvent{kind: "hold", id: "k:Q", sub: "intr", t: time.Now()})
	if rel := heldJSON()["released"].([]map[string]any); len(rel) != 0 {
		t.Fatalf("an interrupted hold was reported: %v", rel)
	}
}
