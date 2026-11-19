package main

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------------------
// v2.3.0: Windows-key and Alt combinations.
//
// Measured with a Raw Input probe (harmless keys only): with Win or Alt held, Windows handles
// many shortcut keys itself and Raw Input delivers only the key-UP of the shortcut key, never
// the key-down. With Ctrl or Shift held both arrive. These tests script exactly those streams.

const (
	vkTab, vkShift, vkCtrl, vkAlt, vkLWin, vkRWin = 0x09, 0x10, 0x11, 0x12, 0x5B, 0x5C
	vk2, vk5, vkT, vkE, vkA, vkK                  = 0x32, 0x35, 0x54, 0x45, 0x41, 0x4B
)

func dn(vk uint32) keyStep    { return keyStep{vk: vk, down: true} }
func up(vk uint32) keyStep    { return keyStep{vk: vk, down: false} }
func dnE(vk uint32) keyStep   { return keyStep{vk: vk, down: true, ext: true} }
func upE(vk uint32) keyStep   { return keyStep{vk: vk, down: false, ext: true} }
func tap(vk uint32) []keyStep { return []keyStep{dn(vk), up(vk)} }

// consumed is a key whose key-down Windows swallowed: only the key-up arrives.
func consumed(vk uint32) keyStep { return up(vk) }

func seq(parts ...any) []keyStep {
	var out []keyStep
	for _, p := range parts {
		switch v := p.(type) {
		case keyStep:
			out = append(out, v)
		case []keyStep:
			out = append(out, v...)
		}
	}
	return out
}

// counts returns how many "key:<id>" events and how many "shortcut:" events were queued.
func counts(got []string) (keys map[string]int, shortcuts int) {
	keys = map[string]int{}
	for _, g := range got {
		switch {
		case strings.HasPrefix(g, "key:"):
			keys[strings.TrimPrefix(g, "key:")]++
		case g == "shortcut:":
			shortcuts++
		}
	}
	return
}

func wantKeys(t *testing.T, got []string, want map[string]int, wantShortcuts int) {
	t.Helper()
	keys, sc := counts(got)
	for k, n := range want {
		if keys[k] != n {
			t.Errorf("key %q counted %d times, want %d (events: %v)", k, keys[k], n, got)
		}
	}
	for k, n := range keys {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected key %q counted %d times (events: %v)", k, n, got)
		}
	}
	if sc != wantShortcuts {
		t.Errorf("shortcut events = %d, want %d (events: %v)", sc, wantShortcuts, got)
	}
}

// Win+2 where Windows swallows the key-down of "2": only its key-up arrives.
func TestWinPlusNumberKeyDownConsumed(t *testing.T) {
	got := feedKeys(t, seq(dn(vkLWin), consumed(vk2), up(vkLWin)))
	wantKeys(t, got, map[string]int{"MetaLeft": 1, "2": 1}, 1)
}

// Same combination when both halves do arrive: still exactly one count (no duplicates).
func TestWinPlusNumberFullPairNotDoubleCounted(t *testing.T) {
	got := feedKeys(t, seq(dn(vkLWin), tap(vk2), up(vkLWin)))
	wantKeys(t, got, map[string]int{"MetaLeft": 1, "2": 1}, 1)
}

// Win held, several different keys, some consumed and some not.
func TestWinHeldWithSeveralKeys(t *testing.T) {
	got := feedKeys(t, seq(dn(vkLWin), consumed(vkT), consumed(vk5), tap(vkE), consumed(vk2), up(vkLWin)))
	wantKeys(t, got, map[string]int{"MetaLeft": 1, "T": 1, "5": 1, "E": 1, "2": 1}, 4)
}

// Win+T, then 5 while Win is still held.
func TestWinTThenFive(t *testing.T) {
	got := feedKeys(t, seq(dn(vkLWin), consumed(vkT), consumed(vk5), up(vkLWin)))
	wantKeys(t, got, map[string]int{"MetaLeft": 1, "T": 1, "5": 1}, 2)
}

// Alt+Tab where the Tab key-down is consumed by the task switcher.
func TestAltTab(t *testing.T) {
	got := feedKeys(t, seq(dn(vkAlt), consumed(vkTab), up(vkAlt)))
	wantKeys(t, got, map[string]int{"AltLeft": 1, "Tab": 1}, 1)
}

// Alt held, Tab pressed three times: every Tab counts, Alt counts once.
func TestAltHeldTabRepeatedThreeTimes(t *testing.T) {
	got := feedKeys(t, seq(dn(vkAlt), consumed(vkTab), consumed(vkTab), consumed(vkTab), up(vkAlt)))
	wantKeys(t, got, map[string]int{"AltLeft": 1, "Tab": 3}, 3)
}

// The same when the app does get the downs: still three, never six.
func TestAltTabThreeWithDownsNotDoubleCounted(t *testing.T) {
	got := feedKeys(t, seq(dn(vkAlt), tap(vkTab), tap(vkTab), tap(vkTab), up(vkAlt)))
	wantKeys(t, got, map[string]int{"AltLeft": 1, "Tab": 3}, 3)
}

// Several Alt combinations in a row, Alt released between them.
func TestSeveralAltCombinationsInSuccession(t *testing.T) {
	got := feedKeys(t, seq(
		dn(vkAlt), consumed(vkTab), up(vkAlt),
		dn(vkAlt), consumed(vkTab), up(vkAlt),
		dn(vkAlt), consumed(vkA), up(vkAlt)))
	wantKeys(t, got, map[string]int{"AltLeft": 3, "Tab": 2, "A": 1}, 3)
}

// Left and right Alt. Right Alt is reported with the extended-key flag. Right Alt alone is not
// counted as a "shortcut" (AltGr layouts send Ctrl+Alt), but its keys are counted.
func TestLeftAndRightAlt(t *testing.T) {
	got := feedKeys(t, seq(
		dn(vkAlt), consumed(vkTab), up(vkAlt),
		dnE(vkAlt), consumed(vkTab), upE(vkAlt)))
	wantKeys(t, got, map[string]int{"AltLeft": 1, "AltRight": 1, "Tab": 2}, 1)
}

// Left and right Windows keys.
func TestLeftAndRightWindows(t *testing.T) {
	got := feedKeys(t, seq(
		dn(vkLWin), consumed(vk2), up(vkLWin),
		dn(vkRWin), consumed(vk2), up(vkRWin)))
	wantKeys(t, got, map[string]int{"MetaLeft": 1, "MetaRight": 1, "2": 2}, 2)
}

// After the modifier is released, a lone key-up is not a press, and the next ordinary key
// is counted once, as before.
func TestModifierReleaseThenOrdinaryKeys(t *testing.T) {
	got := feedKeys(t, seq(
		dn(vkAlt), consumed(vkTab), up(vkAlt),
		up(vkA), // stray release with no modifier held: ignored
		tap(vkA)))
	wantKeys(t, got, map[string]int{"AltLeft": 1, "Tab": 1, "A": 1}, 1)
}

// Rapid combinations: 50 quick Alt+Tab presses, then 50 quick Win+2 presses.
func TestRapidCombinations(t *testing.T) {
	steps := []keyStep{dn(vkAlt)}
	for i := 0; i < 50; i++ {
		steps = append(steps, consumed(vkTab))
	}
	steps = append(steps, up(vkAlt), dn(vkLWin))
	for i := 0; i < 50; i++ {
		steps = append(steps, consumed(vk2))
	}
	steps = append(steps, up(vkLWin))
	got := feedKeys(t, steps)
	wantKeys(t, got, map[string]int{"AltLeft": 1, "Tab": 50, "MetaLeft": 1, "2": 50}, 100)
}

// Ctrl regression: unchanged. A key-up with no counted press while only Ctrl is held is NOT a press.
func TestCtrlRegression(t *testing.T) {
	got := feedKeys(t, seq(dn(vkCtrl), tap(vkA), tap(vkA), up(vkCtrl)))
	wantKeys(t, got, map[string]int{"ControlLeft": 1, "A": 2}, 2)
	got = feedKeys(t, seq(dn(vkCtrl), consumed(vkA), up(vkCtrl)))
	wantKeys(t, got, map[string]int{"ControlLeft": 1}, 0) // Ctrl never loses key-downs: no guessing
}

// Shift regression: unchanged.
func TestShiftRegression(t *testing.T) {
	got := feedKeys(t, seq(dn(vkShift), tap(vkA), tap(vkA), up(vkShift)))
	wantKeys(t, got, map[string]int{"ShiftLeft": 1, "A": 2}, 0)
	got = feedKeys(t, seq(dn(vkShift), consumed(vkA), up(vkShift)))
	wantKeys(t, got, map[string]int{"ShiftLeft": 1}, 0)
}

// Ctrl+Alt (the app's own hotkeys, which Windows consumes): the key now counts, as a shortcut.
func TestCtrlAltHotkeyKeyIsCounted(t *testing.T) {
	got := feedKeys(t, seq(dn(vkCtrl), dn(vkAlt), consumed(vkK), up(vkAlt), up(vkCtrl)))
	wantKeys(t, got, map[string]int{"ControlLeft": 1, "AltLeft": 1, "K": 1}, 1)
}

// Auto-repeat of a held key while Alt is held must not multiply the count.
func TestAltHeldAutoRepeatNotMultiplied(t *testing.T) {
	got := feedKeys(t, seq(dn(vkAlt), dn(vkA), dn(vkA), dn(vkA), up(vkA), up(vkAlt)))
	wantKeys(t, got, map[string]int{"AltLeft": 1, "A": 1}, 1)
}

// Interrupted sequences: a modifier whose release never arrives (for example Win+L) expires
// after 30 s, so later stray releases are not counted.
func TestStaleModifierExpires(t *testing.T) {
	var m modState
	t0 := tm(10, 0)
	m.apply(vkLWin, false, true, t0)
	if !m.winOrAltHeld() {
		t.Fatal("Win should be held")
	}
	m.apply(vkA, false, false, t0.Add(31*time.Second)) // any later event after 30 s of silence
	if m.winOrAltHeld() {
		t.Fatal("stale Win state must expire")
	}
}

// Aggregation: the events these streams queue end up as the right per-key counts in the
// statistics, through the real record() path.
func TestComboKeysAggregateInStatistics(t *testing.T) {
	setupTestStore(t)
	for len(hookEvents) > 0 {
		<-hookEvents
	}
	pressedAt = map[uint32]int64{}
	mods = modState{}
	for _, s := range seq(dn(vkAlt), consumed(vkTab), consumed(vkTab), consumed(vkTab), up(vkAlt), dn(vkLWin), consumed(vk2), up(vkLWin)) {
		handleKey(s.vk, s.scan, s.down, s.ext)
	}
	for len(hookEvents) > 0 {
		ev := <-hookEvents
		if ev.kind == "key" {
			record(ev.kind, ev.id, ev.units)
		}
	}
	storeMu.RLock()
	defer storeMu.RUnlock()
	tot := map[string]uint64{}
	for _, b := range store.Buckets {
		for k, v := range b.Keys {
			tot[k] += v
		}
	}
	if tot["Tab"] != 3 || tot["AltLeft"] != 1 || tot["2"] != 1 || tot["MetaLeft"] != 1 {
		t.Fatalf("aggregated key counts wrong: %v", tot)
	}
}
