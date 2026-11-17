package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// call runs api2Handler for one request and returns the status and body.
func call(t *testing.T, method, path string, body []byte) (int, []byte) {
	t.Helper()
	u, _ := url.Parse(path)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	if !api2Handler(rec, req, u.Query()) {
		t.Fatalf("%s %s: not handled", method, path)
	}
	return rec.Code, rec.Body.Bytes()
}

func TestCSVExportHasBOMAndHeader(t *testing.T) {
	setupTestStore(t)
	storeMu.Lock()
	store.Days["2026-11-01"] = &DayRoll{Keys: 7, Clicks: 3, Shortcuts: 2}
	storeMu.Unlock()
	code, b := call(t, "GET", "/api/export/csv", nil)
	if code != 200 || !bytes.HasPrefix(b, []byte("\xef\xbb\xbf")) {
		t.Fatalf("csv status %d or missing byte-order mark", code)
	}
	text := strings.ReplaceAll(string(b[3:]), "\r\n", "\n")
	if !strings.HasPrefix(text, "date,keys,clicks,scrolls,words,active_min,idle_min,shortcuts\n2026-11-01,7,3,0,0,0,0,2\n") {
		t.Fatalf("csv body wrong: %q", text)
	}
}

func TestPracticeScoresValidatedAndNoText(t *testing.T) {
	setupTestStore(t)
	bad := []string{
		`{"lesson":"","wpm":40,"acc":90,"errors":1,"secs":30}`,
		`{"lesson":"a","wpm":999,"acc":90,"errors":1,"secs":30}`,
		`{"lesson":"a","wpm":40,"acc":101,"errors":1,"secs":30}`,
		`{"lesson":"a","wpm":40,"acc":90,"errors":1,"secs":0}`,
		`not json`,
	}
	for _, b := range bad {
		if code, _ := call(t, "POST", "/api/practice", []byte(b)); code == 200 {
			t.Errorf("accepted bad score %s", b)
		}
	}
	// a score with an extra "text" field must be accepted but the text must not be kept
	code, body := call(t, "POST", "/api/practice", []byte(`{"lesson":"home-row","wpm":42.5,"acc":96.2,"errors":3,"secs":60,"text":"secret typed words"}`))
	if code != 200 {
		t.Fatalf("good score refused: %d %s", code, body)
	}
	storeMu.RLock()
	raw, _ := json.Marshal(store.Practice)
	storeMu.RUnlock()
	if strings.Contains(string(raw), "secret") || !strings.Contains(string(raw), "home-row") {
		t.Fatalf("stored practice data wrong: %s", raw)
	}
}

func TestImportPreviewChangesNothing(t *testing.T) {
	setupTestStore(t)
	record("key", "A", 0)
	storeMu.Lock()
	rollMinuteLocked(time.Now(), true)
	storeMu.Unlock()
	before := snapshotJSON(t)
	in := &Store{Days: map[string]*DayRoll{"2020-01-01": {Keys: 100}}, Buckets: map[string]*HourBucket{}}
	in.normalize()
	out := importPreview(in, "merge")
	if out["daysApplied"] != 1 {
		t.Fatalf("preview daysApplied = %v, want 1", out["daysApplied"])
	}
	rep := importPreview(in, "replace")
	if rep["daysOnlyInCurrent"] != 1 {
		t.Fatalf("replace preview: daysOnlyInCurrent = %v, want 1 (today)", rep["daysOnlyInCurrent"])
	}
	if after := snapshotJSON(t); after != before {
		t.Fatal("the preview changed the stored data")
	}
}

func snapshotJSON(t *testing.T) string {
	t.Helper()
	storeMu.RLock()
	defer storeMu.RUnlock()
	b, _ := json.Marshal(&store)
	return string(b)
}

func TestStatsTotalsIncludeShortcuts(t *testing.T) {
	setupTestStore(t)
	recordCombo("shortcut")
	recordCombo("shortcut")
	recordCombo("modkey")
	s := buildStats(map[string][]string{"range": {"today"}})
	if s.Totals.Shortcuts != 2 || s.Totals.ModKeys != 1 {
		t.Fatalf("totals: shortcuts=%d modKeys=%d, want 2 and 1", s.Totals.Shortcuts, s.Totals.ModKeys)
	}
}

func TestTimelineHoursFromBuckets(t *testing.T) {
	setupTestStore(t)
	record("key", "A", 0)
	record("mouse", "left", 0)
	out := timelineJSON(dayKey(time.Now()))
	hours := out["hours"].([]map[string]any)
	if len(hours) != 24 {
		t.Fatalf("hours = %d, want 24", len(hours))
	}
	h := time.Now().Hour()
	if hours[h]["keys"] != uint64(1) || hours[h]["clicks"] != uint64(1) || out["peak"] != uint64(2) {
		t.Fatalf("hour %d wrong: %v peak %v", h, hours[h], out["peak"])
	}
}

func TestStrainJSONOffByDefaultAndLearning(t *testing.T) {
	setupTestStore(t)
	if strainJSON(time.Now())["on"] != false {
		t.Fatal("strain signals must be off by default")
	}
	storeMu.Lock()
	store.Settings.StrainOn = true
	storeMu.Unlock()
	j := strainJSON(time.Now())
	if j["enough_history"] != false || j["flagged"] != false || !strings.Contains(j["message"].(string), "Learning") {
		t.Fatalf("new user must see the learning message: %v", j)
	}
	// 20 earlier days with ordinary peaks, then a very busy day
	storeMu.Lock()
	for i := 1; i <= 20; i++ {
		store.Days[midnight(time.Now()).AddDate(0, 0, -i).Format("2006-01-02")] = &DayRoll{Peak10: uint64(300 + i*5)}
	}
	store.Days[dayKey(time.Now())] = &DayRoll{Peak10: 1500}
	storeMu.Unlock()
	j = strainJSON(time.Now())
	msg, _ := j["message"].(string)
	if j["flagged"] != true || !strings.Contains(msg, "A short break may help") || strings.Contains(strings.ToLower(msg), "injur") || strings.Contains(strings.ToLower(msg), "diagnos") {
		t.Fatalf("busy day not flagged with the suggestion wording: %v", j)
	}
}

func TestBuildReportSectionsAndFile(t *testing.T) {
	setupTestStore(t)
	for i := 0; i < 10; i++ {
		record("key", "A", 0)
	}
	title, body, key := buildReport("day", time.Now())
	for _, want := range []string{"# " + appName + " daily summary", "## Totals", "Key presses:", "Best hour:", "Compared with the period before", "never what was typed"} {
		if !strings.Contains(body, want) {
			t.Errorf("report missing %q", want)
		}
	}
	if !strings.HasPrefix(key, "daily:") || title == "" {
		t.Fatalf("title/key wrong: %q %q", title, key)
	}
	n1, err := saveReportFile("day", body)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := saveReportFile("day", body)
	if err != nil || n1 == n2 {
		t.Fatalf("a second report must get a new name (never overwrite): %q %q %v", n1, n2, err)
	}
	if !strings.HasSuffix(n1, ".md") {
		t.Fatalf("report file must be .md: %s", n1)
	}
	wk, wbody, wkey := buildReport("week", time.Now())
	if !strings.HasPrefix(wkey, "weekly:") || !strings.Contains(wbody, "Last week") || wk == "" {
		t.Fatalf("weekly report wrong: %q", wbody)
	}
}

func TestGoalHistoryJSONShape(t *testing.T) {
	setupTestStore(t)
	goalTick(time.Now())
	code, b := call(t, "GET", "/api/goals?days=7", nil)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var out struct {
		Days []struct {
			Date  string `json:"date"`
			State string `json:"state"`
		} `json:"days"`
	}
	if err := json.Unmarshal(b, &out); err != nil || len(out.Days) != 7 {
		t.Fatalf("goal history wrong: %v %s", err, b)
	}
	if out.Days[6].Date != dayKey(time.Now()) || out.Days[0].State != "nodata" {
		t.Fatalf("order/state wrong: %+v", out.Days)
	}
}

func TestAutoBackupAndPruning(t *testing.T) {
	setupTestStore(t)
	record("key", "A", 0)
	if err := saveStore(); err != nil {
		t.Fatal(err)
	}
	autoBackupIfDue()
	if n := len(backupNamesWithPrefix("activity-auto-")); n != 1 {
		t.Fatalf("auto backups after first run = %d, want 1", n)
	}
	autoBackupIfDue() // not due again within six hours
	if n := len(backupNamesWithPrefix("activity-auto-")); n != 1 {
		t.Fatalf("auto backup repeated inside the interval: %d", n)
	}
}

func TestDisplaysJSON(t *testing.T) {
	setupTestStore(t)
	out := displaysJSON()
	if out["count"].(int) < 1 {
		t.Fatalf("no monitors found: %v", out)
	}
	m := out["monitors"].([]map[string]any)[0]
	if m["w"].(int32) <= 0 || m["dpi"].(uint32) < 96 {
		t.Fatalf("monitor data wrong: %v", m)
	}
}

// B13: only JSON, TXT and MD data can be imported. A CSV file (or any other text) is refused
// before anything is parsed into the store.
func TestImportRefusesCSVAndOtherFormats(t *testing.T) {
	for _, body := range []string{
		"date,keys,clicks\n2026-11-01,5,2\n",
		"<html><body>not data</body></html>",
		"",
		"{\"hello\": \"world\"}",
	} {
		if _, err := parseImport([]byte(body)); err == nil {
			t.Errorf("parseImport accepted %q", body)
		}
	}
}
