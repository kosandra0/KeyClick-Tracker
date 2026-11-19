package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fillSampleStore puts exact, hand-picked numbers into the global store: hourly counts, two days
// with measured holds, scroll sessions and per-program detail, and interval history.
func fillSampleStore(t *testing.T) (today, yesterday string) {
	t.Helper()
	now := time.Now()
	today, yesterday = dayKey(now), dayKey(now.AddDate(0, 0, -1))
	h1 := localHourStart(now).Format(time.RFC3339)
	h2 := localHourStart(now.Add(-time.Hour)).Format(time.RFC3339)
	storeMu.Lock()
	defer storeMu.Unlock()
	store.Buckets[h1] = &HourBucket{Keys: map[string]uint64{"A": 120, "Space": 33}, Mouse: map[string]uint64{"left": 14, "right": 2}, Scroll: map[string]uint64{"down": 9}, ScrollUnit: map[string]uint64{"down": 27}}
	store.Buckets[h2] = &HourBucket{Keys: map[string]uint64{"A": 80}, Mouse: map[string]uint64{"left": 5}, Scroll: map[string]uint64{"up": 4}, ScrollUnit: map[string]uint64{"up": 4}}
	store.Days[today] = &DayRoll{Keys: 153, Chars: 150, Clicks: 16, Scrolls: 9, IdleSec: 4321, Shortcuts: 7, ModKeys: 3, Peak10: 99,
		Layouts: map[string]uint64{"English (India)": 153},
		KeyHold: map[string]*HoldStat{"A": {N: 120, Ms: 18345, Max: 912}, "Space": {N: 33, Ms: 4107, Max: 380}},
		MsHold:  map[string]*HoldStat{"left": {N: 14, Ms: 2540, Max: 300}, "right": {N: 2, Ms: 420, Max: 260}},
		Scr:     &ScrollAgg{Sessions: 4, Events: 9, SpanMs: 2760, MaxMs: 1090, Single: 1, Dir: map[string]uint64{"down": 9}},
		Apps: map[string]*AppCount{
			"chrome.exe": {Keys: 130, Clicks: 12, Scroll: 9, Secs: 600, Sess: 4, SessMs: 2760, Last: 1790000000, KeyIDs: map[string]uint64{"A": 100, "Space": 30}, MsIDs: map[string]uint64{"left": 12},
				ScDir: map[string]uint64{"down": 9}, KeyHold: map[string]*HoldStat{"A": {N: 100, Ms: 15000, Max: 900}}, MsHold: map[string]*HoldStat{"left": {N: 12, Ms: 2400, Max: 300}}},
			"keyclick-tracker.exe": {Keys: 23, Clicks: 4, Last: 1790000100, KeyIDs: map[string]uint64{"A": 20}, MsIDs: map[string]uint64{"right": 2, "left": 2}},
		}}
	store.Days[yesterday] = &DayRoll{Keys: 80, Clicks: 5, Scrolls: 4, KeyHold: map[string]*HoldStat{"A": {N: 80, Ms: 9000, Max: 700}}}
	store.Recent = []Interval{
		{K: "key", ID: "A", St: now.Add(-50 * time.Minute).UnixMilli(), Ms: 143, App: "chrome.exe"},
		{K: "key", ID: "Space", St: now.Add(-40 * time.Minute).UnixMilli(), Ms: 98, App: "chrome.exe"},
		{K: "mouse", ID: "left", St: now.Add(-30 * time.Minute).UnixMilli(), Ms: 211, App: "keyclick-tracker.exe"},
		{K: "scroll", ID: "down", St: now.Add(-20 * time.Minute).UnixMilli(), Ms: 850, App: "chrome.exe", N: 7},
		{K: "key", ID: "Q", St: now.Add(-10 * time.Minute).UnixMilli(), App: "code.exe", Sts: "intr"},
	}
	return
}

func exportBody(t *testing.T, format string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/export?format="+format+"&range=all", nil)
	w := httptest.NewRecorder()
	exportHandler(w, req)
	if w.Code != 200 {
		t.Fatalf("export %s: status %d: %s", format, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func j(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Exporting in each of the three formats and reading the file back gives exactly the same data:
// hourly counts, daily totals, measured holds, scroll sessions, per-program detail and history.
func TestExportImportRoundTripAllThreeFormats(t *testing.T) {
	setupTestStore(t)
	today, _ := fillSampleStore(t)
	want := snapshotStore()
	for _, format := range []string{"json", "txt", "md"} {
		body := exportBody(t, format)
		got, err := parseImport([]byte(body))
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if j(t, got.Buckets) != j(t, want.Buckets) {
			t.Errorf("%s: hourly counts differ\n got  %s\n want %s", format, j(t, got.Buckets), j(t, want.Buckets))
		}
		if j(t, got.Days) != j(t, want.Days) {
			t.Errorf("%s: days differ\n got  %s\n want %s", format, j(t, got.Days), j(t, want.Days))
		}
		if j(t, got.Recent) != j(t, want.Recent) {
			t.Errorf("%s: history differs", format)
		}
		d := got.Days[today]
		if d.KeyHold["A"].Ms != 18345 || d.KeyHold["A"].N != 120 || d.MsHold["left"].Max != 300 || d.Scr.SpanMs != 2760 || d.Apps["chrome.exe"].MsHold["left"].Ms != 2400 {
			t.Errorf("%s: exact times not preserved: %+v", format, d)
		}
	}
}

// Replace and merge both bring everything back into an empty app.
func TestImportRestoresEverythingFromEveryFormat(t *testing.T) {
	setupTestStore(t)
	fillSampleStore(t)
	want := snapshotStore()
	bodies := map[string]string{}
	for _, f := range []string{"json", "txt", "md"} {
		bodies[f] = exportBody(t, f)
	}
	for _, mode := range []string{"replace", "merge"} {
		for f, body := range bodies {
			setupTestStore(t) // an empty app
			in, err := parseImport([]byte(body))
			if err != nil {
				t.Fatalf("%s %s: %v", mode, f, err)
			}
			if _, _, _, _, err := importData(in, mode); err != nil {
				t.Fatalf("%s %s: %v", mode, f, err)
			}
			storeMu.RLock()
			gd, gb, gr := j(t, store.Days), j(t, store.Buckets), j(t, store.Recent)
			storeMu.RUnlock()
			if gd != j(t, want.Days) || gb != j(t, want.Buckets) || gr != j(t, want.Recent) {
				t.Errorf("%s from %s: data differs after import\n days got  %s\n days want %s", mode, f, gd, j(t, want.Days))
			}
		}
	}
}

// Importing the same file again changes nothing (no double counting of holds or history).
func TestImportMergeIsIdempotent(t *testing.T) {
	setupTestStore(t)
	fillSampleStore(t)
	body := exportBody(t, "json")
	in, _ := parseImport([]byte(body))
	setupTestStore(t)
	importData(in, "merge")
	storeMu.RLock()
	first := j(t, store.Days) + j(t, store.Recent)
	storeMu.RUnlock()
	in2, _ := parseImport([]byte(body))
	added, updated, days, _, err := importData(in2, "merge")
	if err != nil {
		t.Fatal(err)
	}
	storeMu.RLock()
	second := j(t, store.Days) + j(t, store.Recent)
	storeMu.RUnlock()
	if first != second || added != 0 || updated != 0 || days != 0 {
		t.Fatalf("second import changed things: added %d updated %d days %d", added, updated, days)
	}
}

// A merge keeps what only exists locally and adds what only exists in the file.
func TestImportMergeKeepsBothSides(t *testing.T) {
	setupTestStore(t)
	today, _ := fillSampleStore(t)
	file, _ := parseImport([]byte(exportBody(t, "json")))
	setupTestStore(t)
	// local copy of the same day: fewer A holds, but a key and a button the file does not have, and a local-only record
	storeMu.Lock()
	store.Days[today] = &DayRoll{Keys: 500, Clicks: 20, KeyHold: map[string]*HoldStat{"A": {N: 200, Ms: 30000, Max: 1500}, "Z": {N: 9, Ms: 777, Max: 200}},
		MsHold: map[string]*HoldStat{"middle": {N: 3, Ms: 90, Max: 50}}, Scr: &ScrollAgg{Sessions: 2, Events: 4, SpanMs: 500, MaxMs: 400, Dir: map[string]uint64{"up": 4}},
		Apps: map[string]*AppCount{"notepad.exe": {Keys: 5}}}
	store.Recent = []Interval{{K: "key", ID: "Z", St: time.Now().UnixMilli(), Ms: 66, App: "notepad.exe"}}
	storeMu.Unlock()
	importData(file, "merge")
	storeMu.RLock()
	defer storeMu.RUnlock()
	d := store.Days[today]
	if d.Keys != 500 { // local day is bigger: its counters stay
		t.Errorf("local counters were replaced: keys %d", d.Keys)
	}
	if h := d.KeyHold["A"]; h.N != 200 || h.Ms != 30000 || h.Max != 1500 {
		t.Errorf("A hold = %+v, want the local one (more measured holds)", h)
	}
	if d.KeyHold["Z"] == nil || d.KeyHold["Space"] == nil || d.KeyHold["Space"].Ms != 4107 {
		t.Errorf("a key was lost or not added: %v", d.KeyHold)
	}
	if d.MsHold["middle"] == nil || d.MsHold["left"] == nil || d.MsHold["right"].Ms != 420 {
		t.Errorf("a button was lost or not added: %v", d.MsHold)
	}
	if d.Scr.Sessions != 4 || d.Scr.SpanMs != 2760 || d.Scr.Dir["up"] != 4 || d.Scr.Dir["down"] != 9 {
		t.Errorf("scroll sessions = %+v", d.Scr)
	}
	if d.Apps["notepad.exe"] == nil || d.Apps["chrome.exe"] == nil || d.Apps["chrome.exe"].KeyHold["A"].Ms != 15000 || d.Apps["keyclick-tracker.exe"] == nil {
		t.Errorf("programs = %v", d.Apps)
	}
	if len(store.Recent) != 6 { // 5 from the file + 1 local
		t.Errorf("history has %d records, want 6", len(store.Recent))
	}
	for i := 1; i < len(store.Recent); i++ {
		if store.Recent[i].St < store.Recent[i-1].St {
			t.Fatal("history is not in time order")
		}
	}
}

// When the file's day is the bigger one, its counters win, and local-only extras survive.
func TestImportMergeBiggerFileDayKeepsLocalExtras(t *testing.T) {
	setupTestStore(t)
	today, _ := fillSampleStore(t)
	file, _ := parseImport([]byte(exportBody(t, "json")))
	setupTestStore(t)
	storeMu.Lock()
	store.Days[today] = &DayRoll{Keys: 10, KeyHold: map[string]*HoldStat{"Z": {N: 1, Ms: 50, Max: 50}}}
	storeMu.Unlock()
	importData(file, "merge")
	storeMu.RLock()
	defer storeMu.RUnlock()
	d := store.Days[today]
	if d.Keys != 153 || d.KeyHold["A"].Ms != 18345 || d.KeyHold["Z"] == nil || d.KeyHold["Z"].Ms != 50 {
		t.Fatalf("day = %+v", d)
	}
}

// The readable part of the txt and md exports lists the hold times and the programs.
func TestTextAndMarkdownReportsListDurations(t *testing.T) {
	setupTestStore(t)
	fillSampleStore(t)
	txt, md := exportBody(t, "txt"), exportBody(t, "md")
	for name, body := range map[string]string{"txt": txt, "md": md} {
		low := strings.ToLower(body)
		// key-held time over the whole range: A 18.345 s + Space 4.107 s today + A 9 s yesterday = 31.452 s
		for _, want := range []string{"input duration", "key-held time", "31.45 s", "held keys", "mouse buttons held", "applications", "chrome.exe", "keyclick-tracker.exe", "scroll sessions", "restorable data"} {
			if !strings.Contains(low, want) {
				t.Errorf("%s report lacks %q", name, want)
			}
		}
	}
}

// The import preview tells how many holds, how much held time and which history the file has.
func TestImportPreviewShowsDurations(t *testing.T) {
	setupTestStore(t)
	fillSampleStore(t)
	file, _ := parseImport([]byte(exportBody(t, "json")))
	setupTestStore(t)
	p := importPreview(file, "merge")
	f := p["file"].(map[string]any)
	if f["holds"].(uint64) != 120+33+14+2+80 || f["held_ms"].(uint64) != 18345+4107+2540+420+9000 || f["scroll_sessions"].(uint64) != 4 || f["history"].(int) != 5 {
		t.Fatalf("preview file info = %v", f)
	}
	if p["historyNew"] != true || p["daysApplied"].(int) != 2 {
		t.Fatalf("preview = %v", p)
	}
}

// A file exported by version 2.3.0 or earlier (no durations) still imports, and adds none.
func TestOldExportWithoutDurationsStillImports(t *testing.T) {
	setupTestStore(t)
	old := `{"format":"keyclick-tracker-export","format_version":1,"app":"KeyClick Tracker","app_version":"2.3.0","data":{"version":4,"buckets":{"` + localHourStart(time.Now()).Format(time.RFC3339) + `":{"keys":{"A":5},"mouse":{},"scroll":{},"scroll_unit":{}}},"days":{"` + dayKey(time.Now()) + `":{"keys":5,"clicks":0,"scrolls":0,"apps":{"old.exe":{"k":5}}}}}}`
	in, err := parseImport([]byte(old))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := importData(in, "merge"); err != nil {
		t.Fatal(err)
	}
	storeMu.RLock()
	defer storeMu.RUnlock()
	d := store.Days[dayKey(time.Now())]
	if d == nil || d.Keys != 5 || d.KeyHold != nil || d.Scr != nil || d.Apps["old.exe"].Keys != 5 {
		t.Fatalf("day = %+v", d)
	}
}
