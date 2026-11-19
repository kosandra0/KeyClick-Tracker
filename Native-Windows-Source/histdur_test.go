package main

import (
	"testing"
	"time"
)

// Hold time per History period uses the same periods as /api/stats, and adds up the days in it.
func TestDurSeriesGroupsDaysIntoPeriods(t *testing.T) {
	resetInput(t)
	now := time.Now()
	d1, d2 := dayKey(now), dayKey(now.AddDate(0, 0, -1))
	storeMu.Lock()
	store.Days[d1] = &DayRoll{Keys: 5, KeyHold: map[string]*HoldStat{"A": {N: 3, Ms: 900, Max: 400}}, MsHold: map[string]*HoldStat{"left": {N: 2, Ms: 300, Max: 200}}, Scr: &ScrollAgg{Sessions: 2, SpanMs: 700}}
	store.Days[d2] = &DayRoll{Keys: 4, KeyHold: map[string]*HoldStat{"A": {N: 1, Ms: 100, Max: 100}, "B": {N: 2, Ms: 250, Max: 150}}}
	holdSeq = 7
	storeMu.Unlock()
	// daily: one period per day
	r := durSeriesJSON(map[string][]string{"range": {"last7"}, "unit": {"daily"}})
	rows := r["rows"].(map[string]map[string]uint64)
	if rows[d1]["k_ms"] != 900 || rows[d1]["k_n"] != 3 || rows[d1]["m_ms"] != 300 || rows[d1]["s_ms"] != 700 || rows[d1]["s_n"] != 2 {
		t.Fatalf("today = %v", rows[d1])
	}
	if rows[d2]["k_ms"] != 350 || rows[d2]["k_n"] != 3 {
		t.Fatalf("yesterday = %v", rows[d2])
	}
	if r["now_key"] != d1 || r["seq"].(uint64) != 7 || r["hourly"].(bool) {
		t.Fatalf("meta = %v", r)
	}
	// monthly: both days land in the month(s) of those days and nothing is lost
	r = durSeriesJSON(map[string][]string{"range": {"last7"}, "unit": {"monthly"}})
	var keyMs uint64
	for _, row := range r["rows"].(map[string]map[string]uint64) {
		keyMs += row["k_ms"]
	}
	if keyMs != 1250 {
		t.Fatalf("monthly key ms = %d, want 1250", keyMs)
	}
	// hourly periods have no hold data (holds are kept per day)
	if r := durSeriesJSON(map[string][]string{"range": {"today"}, "unit": {"hour"}}); !r["hourly"].(bool) || len(r["rows"].(map[string]map[string]uint64)) != 0 {
		t.Fatalf("hourly = %v", r)
	}
}

func TestDurDayDetail(t *testing.T) {
	resetInput(t)
	d := dayKey(time.Now())
	storeMu.Lock()
	store.Days[d] = &DayRoll{Keys: 9, KeyHold: map[string]*HoldStat{"A": {N: 3, Ms: 900, Max: 400}}, MsHold: map[string]*HoldStat{"left": {N: 2, Ms: 300, Max: 200}},
		Scr: &ScrollAgg{Sessions: 2, SpanMs: 700, Dir: map[string]uint64{"down": 5}},
		Apps: map[string]*AppCount{"a.exe": {Keys: 5, Clicks: 1, KeyHold: map[string]*HoldStat{"A": {N: 3, Ms: 900}}, SessMs: 700, Sess: 2},
			"b.exe": {Keys: 1, MsHold: map[string]*HoldStat{"left": {N: 1, Ms: 50}}}}}
	storeMu.Unlock()
	out := durDayJSON(d)
	if out["key_hold"].(map[string]*HoldStat)["A"].Ms != 900 || out["mouse_hold"].(map[string]*HoldStat)["left"].N != 2 || out["scroll"].(*ScrollAgg).SpanMs != 700 {
		t.Fatalf("day = %v", out)
	}
	apps := out["apps"].([]map[string]any)
	if len(apps) != 2 || apps[0]["name"] != "a.exe" || apps[0]["k_ms"].(uint64) != 900 || apps[0]["s_ms"].(uint64) != 700 || apps[1]["m_ms"].(uint64) != 50 {
		t.Fatalf("apps = %v", apps)
	}
	// a day with no data is an empty answer, not an error
	if e := durDayJSON("2001-01-01"); len(e["apps"].([]map[string]any)) != 0 || e["scroll"] != nil {
		t.Fatalf("empty day = %v", e)
	}
}
