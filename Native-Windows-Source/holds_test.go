package main

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"testing"
	"time"
)

// resetInput clears every piece of shared input state so a test starts clean.
func resetInput(t *testing.T) {
	t.Helper()
	setupTestStore(t)
	storeMu.Lock()
	holdSeq = 0
	lastScroll.Seq, lastScroll.SpanMs, lastScroll.Events = 0, 0, 0
	storeMu.Unlock()
	for len(hookEvents) > 0 {
		<-hookEvents
	}
	pressedAt = map[uint32]int64{}
	mods = modState{}
	clearHeld()
	sess = nil
	setPaused(false)
}

func drain() []hookEvent {
	var out []hookEvent
	for len(hookEvents) > 0 {
		out = append(out, <-hookEvents)
	}
	return out
}

func holds(evs []hookEvent) map[string]hookEvent {
	m := map[string]hookEvent{}
	for _, e := range evs {
		if e.kind == "hold" {
			m[e.id] = e
		}
	}
	return m
}

// A held key is one interval whose length is the real time between key-down and key-up.
func TestKeyHoldIsMeasured(t *testing.T) {
	resetInput(t)
	handleKey(vkA, 0x1E, true, false)
	time.Sleep(150 * time.Millisecond)
	handleKey(vkA, 0x1E, false, false)
	h := holds(drain())["k:A"]
	if h.units < 140 || h.units > 600 {
		t.Fatalf("hold of A = %d ms, want about 150", h.units)
	}
	recordHold(h)
	storeMu.RLock()
	hs := dayFor(time.Now()).KeyHold["A"]
	storeMu.RUnlock()
	if hs == nil || hs.N != 1 || hs.Ms != h.units || hs.Max != h.units {
		t.Fatalf("stored hold = %+v", hs)
	}
}

// OS auto-repeat key-downs must not start new holds or cut the first one short.
func TestAutoRepeatDoesNotRestartHold(t *testing.T) {
	resetInput(t)
	handleKey(vkA, 0x1E, true, false)
	time.Sleep(60 * time.Millisecond)
	handleKey(vkA, 0x1E, true, false) // repeat
	time.Sleep(60 * time.Millisecond)
	handleKey(vkA, 0x1E, true, false) // repeat
	time.Sleep(60 * time.Millisecond)
	handleKey(vkA, 0x1E, false, false)
	evs := drain()
	n := 0
	for _, e := range evs {
		if e.kind == "hold" {
			n++
			if e.units < 170 {
				t.Fatalf("hold = %d ms, the repeats restarted it", e.units)
			}
		}
	}
	if n != 1 {
		t.Fatalf("hold events = %d, want 1", n)
	}
}

// Ctrl+Shift+A is three independent concurrent holds.
func TestConcurrentHoldsAreIndependent(t *testing.T) {
	resetInput(t)
	handleKey(vkCtrl, 0x1D, true, false)
	time.Sleep(40 * time.Millisecond)
	handleKey(vkShift, 0x2A, true, false)
	time.Sleep(40 * time.Millisecond)
	handleKey(vkA, 0x1E, true, false)
	heldMu.Lock()
	open := len(heldNow)
	heldMu.Unlock()
	if open != 3 {
		t.Fatalf("open holds = %d, want 3", open)
	}
	time.Sleep(40 * time.Millisecond)
	handleKey(vkA, 0x1E, false, false)
	handleKey(vkShift, 0x2A, false, false)
	handleKey(vkCtrl, 0x1D, false, false)
	h := holds(drain())
	if len(h) != 3 || !(h["k:ControlLeft"].units > h["k:ShiftLeft"].units && h["k:ShiftLeft"].units > h["k:A"].units) {
		t.Fatalf("holds not nested as expected: ctrl=%d shift=%d a=%d (%d holds)", h["k:ControlLeft"].units, h["k:ShiftLeft"].units, h["k:A"].units, len(h))
	}
}

// Left and right Shift are separate identities when Windows reports the scan code.
func TestLeftRightShiftHoldsDiffer(t *testing.T) {
	resetInput(t)
	handleKey(vkShift, 0x2A, true, false)
	handleKey(vkShift, 0x2A, false, false)
	handleKey(vkShift, 0x36, true, false)
	handleKey(vkShift, 0x36, false, false)
	h := holds(drain())
	if _, ok := h["k:ShiftLeft"]; !ok {
		t.Fatal("no left Shift hold")
	}
	if _, ok := h["k:ShiftRight"]; !ok {
		t.Fatal("no right Shift hold")
	}
}

// A press whose key-down Windows consumed (Win+2) is counted but has no measurable duration.
func TestConsumedKeyDownHasNoDuration(t *testing.T) {
	resetInput(t)
	handleKey(vkLWin, 0x5B, true, true)
	handleKey(vk2, 0x03, false, false)
	handleKey(vkLWin, 0x5B, false, true)
	evs := drain()
	h := holds(evs)
	if _, ok := h["k:2"]; ok {
		t.Fatal("a hold was invented for a key whose key-down was never seen")
	}
	if _, ok := h["k:MetaLeft"]; !ok {
		t.Fatal("the Windows key hold is missing")
	}
	counted := false
	for _, e := range evs {
		if e.kind == "key" && e.id == "2" {
			counted = true
		}
	}
	if !counted {
		t.Fatal("the consumed press was no longer counted")
	}
}

// Mouse button holds: drag keeps one hold; each button has its own identity.
func TestMouseButtonHold(t *testing.T) {
	resetInput(t)
	handleMouse(&rawMouse{ButtonFlags: rmbLeftDown})
	handleMouse(&rawMouse{ButtonFlags: rmbX1Down})
	time.Sleep(120 * time.Millisecond)
	handleMouse(&rawMouse{ButtonFlags: 0}) // cursor movement alone: nothing
	handleMouse(&rawMouse{ButtonFlags: 0x0002})
	handleMouse(&rawMouse{ButtonFlags: 0x0080})
	h := holds(drain())
	if h["m:left"].units < 110 || h["m:x1"].units < 110 {
		t.Fatalf("mouse holds = left %d, x1 %d ms", h["m:left"].units, h["m:x1"].units)
	}
	if len(h) != 2 {
		t.Fatalf("holds = %d, want 2", len(h))
	}
}

// The live endpoint shows what is held now and nothing after release.
func TestLiveHeldState(t *testing.T) {
	resetInput(t)
	handleKey(vkA, 0x1E, true, false)
	time.Sleep(30 * time.Millisecond)
	j := heldJSON()
	if keys := j["keys"].([]map[string]any); len(keys) != 1 || keys[0]["id"] != "A" {
		t.Fatalf("live keys = %v", j["keys"])
	}
	// total held today = finished holds + the hold in progress
	recordHold(hookEvent{kind: "hold", id: "k:A", units: 500, t: time.Now()})
	row := heldJSON()["keys"].([]map[string]any)[0]
	if tot := row["total_ms"].(uint64); tot < 530 || tot > 5000 || row["holds"].(uint64) != 2 {
		t.Fatalf("total held = %v ms over %v holds, want about 500 + current", row["total_ms"], row["holds"])
	}
	handleKey(vkA, 0x1E, false, false)
	if keys := heldJSON()["keys"].([]map[string]any); len(keys) != 0 {
		t.Fatalf("still held after release: %v", keys)
	}
}

// A hold older than maxHold (a lost key-up) is dropped as interrupted and adds no time.
func TestStaleHoldIsInterruptedNotCounted(t *testing.T) {
	resetInput(t)
	heldMu.Lock()
	heldNow["k:Q"] = &heldItem{start: time.Now().Add(-20 * time.Minute), kind: "k", id: "Q"}
	heldMu.Unlock()
	sweepHolds(time.Now())
	evs := drain()
	h := holds(evs)["k:Q"]
	if h.sub != "intr" {
		t.Fatalf("stale hold event = %+v", h)
	}
	recordHold(h)
	storeMu.RLock()
	defer storeMu.RUnlock()
	if d := dayFor(time.Now()); d.KeyHold["Q"] != nil {
		t.Fatalf("an interrupted hold was added to the totals: %+v", d.KeyHold["Q"])
	}
	if n := len(store.Recent); n != 1 || store.Recent[0].Sts != "intr" {
		t.Fatalf("history = %+v", store.Recent)
	}
}

// While paused, open holds are forgotten and no interval is recorded.
func TestPauseDropsOpenHolds(t *testing.T) {
	resetInput(t)
	handleKey(vkA, 0x1E, true, false)
	setPaused(true)
	sweepHolds(time.Now())
	setPaused(false)
	heldMu.Lock()
	n := len(heldNow)
	heldMu.Unlock()
	if n != 0 || len(drain()) > 1 { // the press event itself may be queued, no hold
		t.Fatalf("open holds after pause = %d", n)
	}
}

// ---------------------------------------------------------------- scroll sessions

func scr(app, dir string, t0 time.Time, ms int) hookEvent {
	return hookEvent{kind: "scroll", id: dir, units: 1, app: app, t: t0.Add(time.Duration(ms) * time.Millisecond)}
}

func scrollDay(t *testing.T) ScrollAgg {
	t.Helper()
	storeMu.RLock()
	defer storeMu.RUnlock()
	d := dayFor(time.Now())
	if d.Scr == nil {
		return ScrollAgg{}
	}
	return *d.Scr
}

func TestScrollSingleEventHasNoSpan(t *testing.T) {
	resetInput(t)
	t0 := time.Now()
	scrollEvent(scr("a.exe", "down", t0, 0))
	closeScroll()
	a := scrollDay(t)
	if a.Sessions != 1 || a.Single != 1 || a.SpanMs != 0 {
		t.Fatalf("single event session = %+v", a)
	}
}

func TestScrollBurstIsOneSessionWithSpan(t *testing.T) {
	resetInput(t)
	t0 := time.Now()
	for _, ms := range []int{0, 100, 220, 330} {
		scrollEvent(scr("a.exe", "down", t0, ms))
	}
	closeScroll()
	a := scrollDay(t)
	if a.Sessions != 1 || a.Events != 4 || a.SpanMs != 330 || a.MaxMs != 330 || a.Single != 0 {
		t.Fatalf("burst = %+v", a)
	}
}

func TestScrollGapStartsNewSession(t *testing.T) {
	resetInput(t)
	t0 := time.Now()
	scrollEvent(scr("a.exe", "down", t0, 0))
	scrollEvent(scr("a.exe", "down", t0, 100))
	scrollEvent(scr("a.exe", "up", t0, 100+500)) // gap above 400 ms
	closeScroll()
	a := scrollDay(t)
	if a.Sessions != 2 || a.Events != 3 {
		t.Fatalf("gap = %+v", a)
	}
	if a.Dir["down"] != 2 || a.Dir["up"] != 1 {
		t.Fatalf("directions = %v", a.Dir)
	}
}

func TestScrollSessionSplitsWhenProgramChanges(t *testing.T) {
	resetInput(t)
	setCurrentApp("a.exe")
	t0 := time.Now()
	scrollEvent(scr("a.exe", "down", t0, 0))
	scrollEvent(scr("a.exe", "down", t0, 50))
	scrollEvent(scr("b.exe", "down", t0, 100)) // focus moved, gap is short
	closeScroll()
	storeMu.RLock()
	d := dayFor(time.Now())
	a, b := d.Apps["a.exe"], d.Apps["b.exe"]
	storeMu.RUnlock()
	if a == nil || b == nil || a.Sess != 1 || b.Sess != 1 || a.SessMs != 50 || b.SessMs != 0 {
		t.Fatalf("per-program sessions: a=%+v b=%+v", a, b)
	}
}

// The scroll event totals of the existing counters are untouched by session tracking.
func TestScrollSessionsDoNotChangeScrollCounts(t *testing.T) {
	resetInput(t)
	for i := 0; i < 3; i++ {
		processEvent(hookEvent{kind: "scroll", id: "down", units: 1, t: time.Now()})
	}
	closeScroll()
	out := todayApps(t)
	if out["total_scroll"] != uint64(0) { // counted on the minute roll, not here
		t.Logf("total_scroll before roll = %v", out["total_scroll"])
	}
	storeMu.RLock()
	defer storeMu.RUnlock()
	if sc := store.Buckets[localHourStart(time.Now()).Format(time.RFC3339)].Scroll["down"]; sc != 3 {
		t.Fatalf("scroll events counted = %d, want 3", sc)
	}
}

// ---------------------------------------------------------------- per-program detail and range

func TestApps2Detail(t *testing.T) {
	resetInput(t)
	setCurrentApp("game.exe")
	record("key", "KeyW", 0)
	record("key", "KeyW", 0)
	record("mouse", "left", 0)
	recordHold(hookEvent{kind: "hold", id: "k:KeyW", units: 400, app: "game.exe", t: time.Now()})
	recordHold(hookEvent{kind: "hold", id: "k:KeyW", units: 600, app: "game.exe", t: time.Now()})
	recordHold(hookEvent{kind: "hold", id: "m:left", units: 90, app: "game.exe", t: time.Now()})
	b, _ := json.Marshal(apps2JSON(map[string][]string{"range": {"today"}}))
	var out struct {
		Apps []struct {
			Name   string                       `json:"name"`
			Keys   uint64                       `json:"keys"`
			KI     map[string]uint64            `json:"ki"`
			MI     map[string]uint64            `json:"mi"`
			KH     map[string]map[string]uint64 `json:"kh"`
			MH     map[string]map[string]uint64 `json:"mh"`
			Clicks uint64                       `json:"clicks"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(b, &out); err != nil || len(out.Apps) != 1 {
		t.Fatalf("apps2 = %s (%v)", b, err)
	}
	a := out.Apps[0]
	if a.Name != "game.exe" || a.Keys != 2 || a.KI["KeyW"] != 2 || a.MI["left"] != 1 {
		t.Fatalf("counts: %+v", a)
	}
	if kh := a.KH["KeyW"]; kh["n"] != 2 || kh["ms"] != 1000 || kh["mx"] != 600 {
		t.Fatalf("key hold: %v", a.KH)
	}
	if mh := a.MH["left"]; mh["n"] != 1 || mh["ms"] != 90 {
		t.Fatalf("button hold: %v", a.MH)
	}
}

// Data written by v2.3.0 and earlier (no hold or session fields) still loads and shows zero.
func TestOldDataLoadsWithoutDurations(t *testing.T) {
	old := `{"version":4,"buckets":{},"days":{"` + dayKey(time.Now()) + `":{"keys":10,"clicks":2,"apps":{"old.exe":{"k":10,"c":2}}}}}`
	var s Store
	if err := json.Unmarshal([]byte(old), &s); err != nil {
		t.Fatal(err)
	}
	d := s.Days[dayKey(time.Now())]
	if d.KeyHold != nil || d.Scr != nil || d.Apps["old.exe"].KeyIDs != nil {
		t.Fatal("legacy data must have no durations")
	}
	if d.Apps["old.exe"].Keys != 10 {
		t.Fatal("legacy counts lost")
	}
}

// The history ledger is capped and newest-first by range.
func TestRecentLedgerCapped(t *testing.T) {
	resetInput(t)
	for i := 0; i < recentLimit+50; i++ {
		recordHold(hookEvent{kind: "hold", id: "k:A", units: uint64(i + 1), t: time.Now()})
	}
	storeMu.RLock()
	n := len(store.Recent)
	last := store.Recent[n-1].Ms
	storeMu.RUnlock()
	if n != recentLimit || last != uint64(recentLimit+50) {
		t.Fatalf("ledger size %d, newest %d", n, last)
	}
	rows := recentJSON(map[string][]string{"range": {"today"}})["rows"].([]Interval)
	if len(rows) != recentLimit || rows[0].Ms != uint64(recentLimit+50) {
		t.Fatalf("recent rows = %d, first %d", len(rows), rows[0].Ms)
	}
}

// ---------------------------------------------------------------- native icons

func TestExtractRealIcon(t *testing.T) {
	p := `C:\Windows\System32\notepad.exe`
	if _, err := os.Stat(p); err != nil {
		t.Skip("notepad.exe not found")
	}
	b, err := extractIconPNG(p)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if bd := img.Bounds(); bd.Dx() < 16 || bd.Dy() < 16 {
		t.Fatalf("icon size %v", bd)
	}
	visible := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				visible++
			}
		}
	}
	if visible < 50 {
		t.Fatalf("icon is blank (%d visible pixels)", visible)
	}
}
