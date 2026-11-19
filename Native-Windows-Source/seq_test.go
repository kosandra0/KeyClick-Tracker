package main

import (
	"testing"
	"time"
)

// Every recorded hold gets the next sequence number, and every total says which one it contains up
// to. The page adds a just-released hold only while its number is above the total's, so a total
// never counts a hold twice and never misses one in between.
func TestSequenceNumbersMakeLiveTotalsExact(t *testing.T) {
	resetInput(t)
	recordHold(hookEvent{kind: "hold", id: "k:A", units: 100, app: "x.exe", t: time.Now().Add(-200 * time.Millisecond)})
	snap := durationJSON(map[string][]string{"range": {"today"}})
	seq1 := snap["seq"].(uint64)
	if seq1 != 1 {
		t.Fatalf("seq after one hold = %d, want 1", seq1)
	}
	rel := heldJSON()["released"].([]map[string]any)
	if len(rel) != 1 || rel[0]["seq"].(uint64) != 1 {
		t.Fatalf("released = %v", rel)
	}
	// a second hold is recorded after the snapshot: its number is above the snapshot's
	recordHold(hookEvent{kind: "hold", id: "k:B", units: 50, app: "x.exe", t: time.Now().Add(-100 * time.Millisecond)})
	rel = heldJSON()["released"].([]map[string]any)
	var newer int
	for _, r := range rel {
		if r["seq"].(uint64) > seq1 {
			newer++
			if r["id"] != "B" {
				t.Fatalf("the newer hold should be B: %v", r)
			}
		}
	}
	if newer != 1 {
		t.Fatalf("holds newer than the snapshot = %d, want 1", newer)
	}
	// the snapshot taken now contains both, and says so
	if s2 := durationJSON(map[string][]string{"range": {"today"}})["seq"].(uint64); s2 != 2 {
		t.Fatalf("seq in a later snapshot = %d, want 2", s2)
	}
	if a := apps2JSON(map[string][]string{"range": {"today"}})["seq"].(uint64); a != 2 {
		t.Fatalf("apps seq = %d, want 2", a)
	}
	// an interrupted hold is not a recorded hold: no number
	recordHold(hookEvent{kind: "hold", id: "k:Q", sub: "intr", t: time.Now()})
	if durationJSON(map[string][]string{"range": {"today"}})["seq"].(uint64) != 2 {
		t.Fatal("an interrupted hold took a sequence number")
	}
}

// A closed scroll session is numbered too and reported with the live state.
func TestClosedScrollSessionIsNumbered(t *testing.T) {
	resetInput(t)
	t0 := time.Now()
	scrollEvent(scr("a.exe", "down", t0, 0))
	scrollEvent(scr("a.exe", "down", t0, 120))
	closeScroll()
	closed := heldJSON()["scroll"].(map[string]any)["closed"].(map[string]any)
	if closed["seq"].(uint64) != 1 || closed["span_ms"].(uint64) != 120 || closed["events"].(uint64) != 2 {
		t.Fatalf("closed session = %v", closed)
	}
	if durationJSON(map[string][]string{"range": {"today"}})["seq"].(uint64) != 1 {
		t.Fatal("the totals do not contain the closed session")
	}
}

// The live answer says which program has focus, so the page can glow the keys of that program.
func TestHeldReportsTheForegroundProgram(t *testing.T) {
	resetInput(t)
	setCurrentApp("game.exe")
	if fg := heldJSON()["fg"]; fg != "game.exe" {
		t.Fatalf("fg = %v", fg)
	}
	setCurrentApp("")
	if fg := heldJSON()["fg"]; fg != "" {
		t.Fatalf("fg = %v, want empty", fg)
	}
}

// The sequence is never written to the data file or to an export.
func TestSequenceIsNotSaved(t *testing.T) {
	resetInput(t)
	recordHold(hookEvent{kind: "hold", id: "k:A", units: 10, t: time.Now()})
	storeMu.RLock()
	b := j(t, store.Recent)
	storeMu.RUnlock()
	if len(b) == 0 || containsSeq(b) {
		t.Fatalf("history carries a sequence: %s", b)
	}
}

func containsSeq(s string) bool {
	for i := 0; i+5 <= len(s); i++ {
		if s[i:i+5] == `"seq"` || s[i:i+5] == `"Seq"` {
			return true
		}
	}
	return false
}
