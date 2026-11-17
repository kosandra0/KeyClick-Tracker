package main

// v2.0.0 endpoints and scheduled work: goal history (F7), reports (F3), CSV export (F11),
// practice scores (F8), day timeline (A1), strain signals (F6), layouts (F14), displays (F15)
// and the import preview (A4).

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// api2Handler serves the v2.0.0 endpoints. It returns false when the path is not one of them.
func api2Handler(w http.ResponseWriter, r *http.Request, q map[string][]string) bool {
	switch r.URL.Path {
	case "/api/goals":
		days, _ := strconv.Atoi(first(q, "days", "30"))
		writeJSON(w, goalHistoryJSON(clampInt(days, 1, 366)))
	case "/api/reports/now":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return true
		}
		period := first(q, "period", "day")
		if period != "day" && period != "week" {
			http.Error(w, "period must be day or week", 400)
			return true
		}
		title, body, _ := buildReport(period, time.Now())
		out := map[string]any{"title": title, "text": body}
		if first(q, "save", "0") == "1" {
			name, err := saveReportFile(period, body)
			if err != nil {
				http.Error(w, "Could not save the report: "+err.Error(), 500)
				return true
			}
			out["saved"] = name
		}
		writeJSON(w, out)
	case "/api/export/csv":
		csvHandler(w, q)
	case "/api/practice":
		practiceHandler(w, r)
	case "/api/timeline":
		writeJSON(w, timelineJSON(first(q, "date", dayKey(time.Now()))))
	case "/api/strain":
		writeJSON(w, strainJSON(time.Now()))
	case "/api/layouts":
		writeJSON(w, layoutsJSON(q))
	case "/api/displays":
		writeJSON(w, displaysJSON())
	case "/api/dev/reminder": // test copies only (KEYCLICK_DEV=1): fake "a reminder just fired" and read the state
		if os.Getenv("KEYCLICK_DEV") != "1" {
			http.NotFound(w, r)
			return true
		}
		reminderMu.Lock()
		if first(q, "fire", "0") == "1" {
			reminder.firedAt = time.Now()
		}
		out := map[string]any{"fired": !reminder.firedAt.IsZero(), "snooze_active": time.Now().Before(reminder.snoozeUntil)}
		reminderMu.Unlock()
		writeJSON(w, out)
	case "/api/import/preview":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 128<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return true
		}
		in, err := parseImport(raw)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return true
		}
		writeJSON(w, importPreview(in, first(q, "mode", "merge")))
	default:
		return false
	}
	return true
}

// ------------------------------------------------------------------ F7 goal history

func goalHistoryJSON(days int) map[string]any {
	now := time.Now()
	storeMu.RLock()
	defer storeMu.RUnlock()
	list := make([]map[string]any, 0, days)
	for i := days - 1; i >= 0; i-- {
		k := midnight(now).AddDate(0, 0, -i).Format("2006-01-02")
		g := store.GoalLog[k]
		if g == nil {
			list = append(list, map[string]any{"date": k, "state": "nodata"})
			continue
		}
		list = append(list, map[string]any{"date": k, "state": g.state(), "gk": g.GK, "gc": g.GC, "gw": g.GW, "mk": g.MK, "mc": g.MC, "mw": g.MW})
	}
	return map[string]any{"days": list, "streak": goalStreak(store.GoalLog, now), "goal_wpm": store.Settings.GoalWPM}
}

// ------------------------------------------------------------------ F3 reports

// buildReport makes a markdown report. "day" is today so far; "week" is the last full week
// (Monday to Sunday) compared with the week before. The key identifies the period.
func buildReport(period string, now time.Time) (title, body, key string) {
	var q, pq map[string][]string
	var label string
	if period == "week" {
		mon := startOfWeek(now).AddDate(0, 0, -7)
		sun := mon.AddDate(0, 0, 6)
		pmon := mon.AddDate(0, 0, -7)
		q = map[string][]string{"range": {"custom"}, "from": {mon.Format("2006-01-02")}, "to": {sun.Format("2006-01-02")}, "unit": {"daily"}}
		pq = map[string][]string{"range": {"custom"}, "from": {pmon.Format("2006-01-02")}, "to": {mon.AddDate(0, 0, -1).Format("2006-01-02")}, "unit": {"daily"}}
		label = "Last week, " + mon.Format("2 Jan") + " to " + sun.Format("2 Jan 2006")
		title = appName + " weekly summary"
		y, wk := mon.ISOWeek()
		key = fmt.Sprintf("weekly:%d-W%02d", y, wk)
	} else {
		q = map[string][]string{"range": {"today"}, "unit": {"hour"}}
		pq = map[string][]string{"range": {"yesterday"}, "unit": {"daily"}}
		label = "Today so far, " + now.Format("Monday 2 Jan 2006")
		title = appName + " daily summary"
		key = "daily:" + dayKey(now)
	}
	s := buildStats(q)
	p := buildStats(pq)
	idle := idleStats(q)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n- **Period:** %s\n- **Made:** %s\n\n## Totals\n\n", title, label, now.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "- **Key presses:** %s\n- **Mouse clicks:** %s\n- **Scroll events:** %s\n- **Words typed:** %s\n", fn(s.Totals.Keys), fn(s.Totals.Clicks), fn(s.Totals.Scrolls), fn(s.Totals.Words))
	fmt.Fprintf(&b, "- **Active time:** %d min\n- **Idle time:** %d min\n", int(s.Rates.ActiveMinutes), toU(idle["idle_sec"])/60)
	if s.Totals.Shortcuts > 0 {
		fmt.Fprintf(&b, "- **Shortcuts (Ctrl, Alt, Win combinations):** %s\n", fn(s.Totals.Shortcuts))
	}
	fmt.Fprintf(&b, "- **Typing speed:** overall %s WPM, peak %s WPM\n", f1(s.Rates.OverallWPM), f1(s.Rates.PeakWPM))
	best, bestH := uint64(0), -1
	for h, hs := range s.HourOfDay {
		if t := hs.Keys + hs.Clicks + hs.Scrolls; t > best {
			best, bestH = t, h
		}
	}
	if bestH >= 0 {
		fmt.Fprintf(&b, "- **Best hour:** %02d:00 to %02d:00 (%s events)\n", bestH, (bestH+1)%24, fn(best))
	} else {
		b.WriteString("- **Best hour:** no activity yet\n")
	}
	if apps, ok := appsJSON(q)["apps"].([]map[string]any); ok && len(apps) > 0 {
		b.WriteString("\n## Top programs\n\n")
		for i, a := range apps {
			if i >= 3 {
				break
			}
			fmt.Fprintf(&b, "%d. %v: %s keys, %s clicks\n", i+1, a["name"], fn(a["keys"].(uint64)), fn(a["clicks"].(uint64)))
		}
	}
	b.WriteString("\n## Compared with the period before\n\n")
	if period == "week" {
		fmt.Fprintf(&b, "- **All events:** %s\n", percentChange(s.Totals.Events, p.Totals.Events))
	} else {
		fmt.Fprintf(&b, "- **Yesterday, whole day:** %s events (today so far: %s)\n", fn(p.Totals.Events), fn(s.Totals.Events))
	}
	b.WriteString("\nOnly counts are reported, never what was typed.\n")
	return title, b.String(), key
}

func toU(v any) uint64 {
	switch n := v.(type) {
	case uint64:
		return n
	case int:
		return uint64(n)
	case float64:
		return uint64(n)
	}
	return 0
}

// saveReportFile writes the report to Backups\Reports under a new name; it never overwrites.
func saveReportFile(period, body string) (string, error) {
	dir := filepath.Join(backupDir(), "Reports")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("report-%s-%s.md", period, nowStamp())
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			break
		}
		name = fmt.Sprintf("report-%s-%s-%d.md", period, nowStamp(), i)
	}
	return name, os.WriteFile(filepath.Join(dir, name), []byte(body), 0600)
}

// reportDue reports whether a scheduled report should be made now, and its period.
func reportDue(mode, last string, now time.Time) (period string, due bool) {
	switch mode {
	case "daily":
		return "day", now.Hour() >= 18 && last != "daily:"+dayKey(now)
	case "weekly":
		mon := startOfWeek(now) // the Monday of this week, 00:00
		y, wk := mon.AddDate(0, 0, -7).ISOWeek()
		key := fmt.Sprintf("weekly:%d-W%02d", y, wk)
		return "week", !now.Before(mon.Add(9*time.Hour)) && last != key
	}
	return "", false
}

func reportWatcher() {
	for range time.Tick(time.Minute) {
		storeMu.RLock()
		mode, last, save := store.Settings.ReportMode, store.Settings.LastReport, store.Settings.ReportFile
		storeMu.RUnlock()
		period, due := reportDue(mode, last, time.Now())
		if !due {
			continue
		}
		title, body, key := buildReport(period, time.Now())
		if save {
			if name, err := saveReportFile(period, body); err == nil {
				logLine("report saved: " + name)
			}
		}
		trayNotify(title, reportOneLine(period))
		storeMu.Lock()
		store.Settings.LastReport = key
		storeMu.Unlock()
		dirty.Store(true)
	}
}

// reportOneLine is the short text of the tray notification.
func reportOneLine(period string) string {
	q := map[string][]string{"range": {"today"}}
	if period == "week" {
		mon := startOfWeek(time.Now()).AddDate(0, 0, -7)
		q = map[string][]string{"range": {"custom"}, "from": {mon.Format("2006-01-02")}, "to": {mon.AddDate(0, 0, 6).Format("2006-01-02")}}
	}
	s := buildStats(q)
	return fmt.Sprintf("%s key presses, %s clicks. Open the app for the full summary.", fn(s.Totals.Keys), fn(s.Totals.Clicks))
}

// ------------------------------------------------------------------ F11 CSV (export only)

func csvHandler(w http.ResponseWriter, q map[string][]string) {
	storeMu.RLock()
	rows := csvDayRows(store.Days, first(q, "from", ""), first(q, "to", ""))
	storeMu.RUnlock()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=keyclick-tracker-days-"+time.Now().Format("2006-01-02")+".csv")
	_, _ = io.WriteString(w, "\xef\xbb\xbf") // UTF-8 byte-order mark so Excel opens it correctly
	cw := csv.NewWriter(w)
	_ = cw.WriteAll(rows)
}

// ------------------------------------------------------------------ F8 practice scores

func practiceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in PracticeScore
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in); err != nil {
			http.Error(w, "bad score", 400)
			return
		}
		if len(in.Lesson) == 0 || len(in.Lesson) > 40 || in.WPM < 0 || in.WPM > 400 || in.Acc < 0 || in.Acc > 100 || in.Errors < 0 || in.Errors > 10000 || in.Secs < 1 || in.Secs > 3600 {
			http.Error(w, "score out of range", 422)
			return
		}
		in.At = time.Now().Format(time.RFC3339) // scores only: no text is accepted or stored
		storeMu.Lock()
		store.Practice = append(store.Practice, in)
		if len(store.Practice) > 500 {
			store.Practice = store.Practice[len(store.Practice)-500:]
		}
		storeMu.Unlock()
		dirty.Store(true)
	}
	storeMu.RLock()
	defer storeMu.RUnlock()
	list := store.Practice
	var best, accSum float64
	for _, s := range list {
		if s.WPM > best {
			best = s.WPM
		}
		accSum += s.Acc
	}
	avg := 0.0
	if len(list) > 0 {
		avg = accSum / float64(len(list))
	}
	tail := list
	if tail == nil {
		tail = []PracticeScore{} // an empty list, not null
	}
	if len(tail) > 100 {
		tail = tail[len(tail)-100:]
	}
	writeJSON(w, map[string]any{"scores": tail, "count": len(list), "best_wpm": best, "avg_acc": avg})
}

// ------------------------------------------------------------------ A1 timeline

// timelineJSON gives the activity of each hour of one day, from the hourly buckets.
func timelineJSON(date string) map[string]any {
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		day = midnight(time.Now())
		date = day.Format("2006-01-02")
	}
	storeMu.RLock()
	defer storeMu.RUnlock()
	hours := make([]map[string]any, 24)
	var peak uint64
	for h := 0; h < 24; h++ {
		t := time.Date(day.Year(), day.Month(), day.Day(), h, 0, 0, 0, time.Local)
		var k, c, s uint64
		if b := store.Buckets[t.Format(time.RFC3339)]; b != nil {
			for _, v := range b.Keys {
				k += v
			}
			for _, v := range b.Mouse {
				c += v
			}
			for _, v := range b.Scroll {
				s += v
			}
		}
		tot := k + c + s
		if tot > peak {
			peak = tot
		}
		hours[h] = map[string]any{"hour": h, "keys": k, "clicks": c, "scrolls": s, "total": tot}
	}
	var idle uint64
	if d := store.Days[date]; d != nil {
		idle = d.IdleSec / 60
	}
	return map[string]any{"date": date, "hours": hours, "peak": peak, "idle_min": idle}
}

// ------------------------------------------------------------------ F6 strain signals

func strainJSON(now time.Time) map[string]any {
	storeMu.RLock()
	defer storeMu.RUnlock()
	on := store.Settings.StrainOn
	out := map[string]any{"on": on, "flagged": false, "enough_history": false}
	if !on {
		return out
	}
	var prior []uint64
	var maxPrior uint64
	for i := 1; i <= 30; i++ {
		if d := store.Days[midnight(now).AddDate(0, 0, -i).Format("2006-01-02")]; d != nil && d.Peak10 > 0 {
			prior = append(prior, d.Peak10)
			if d.Peak10 > maxPrior {
				maxPrior = d.Peak10
			}
		}
	}
	var today uint64
	if d := store.Days[dayKey(now)]; d != nil {
		today = d.Peak10
	}
	out["history_days"], out["today_peak10"] = len(prior), today
	if len(prior) < 14 {
		out["message"] = fmt.Sprintf("Learning your usual pace: %d of 14 days so far.", len(prior))
		return out
	}
	out["enough_history"] = true
	flag, p90 := strainFlag(today, prior)
	out["p90"] = p90
	if flag {
		out["flagged"] = true
		if today > maxPrior {
			out["message"] = fmt.Sprintf("Unusually busy stretch: %s key presses, clicks and scrolls in 10 minutes, the highest in 30 days. A short break may help.", fn(today))
		} else {
			out["message"] = fmt.Sprintf("Unusually busy stretch: %s key presses, clicks and scrolls in 10 minutes, busier than most of your recent days. A short break may help.", fn(today))
		}
	}
	return out
}

func strainWatcher() {
	for range time.Tick(5 * time.Minute) {
		j := strainJSON(time.Now())
		if j["flagged"] != true {
			continue
		}
		storeMu.RLock()
		notify, last := store.Settings.StrainNotify, store.Settings.LastStrainNote
		storeMu.RUnlock()
		today := dayKey(time.Now())
		if !notify || last == today {
			continue
		}
		trayNotify("A busy stretch", fmt.Sprint(j["message"]))
		storeMu.Lock()
		store.Settings.LastStrainNote = today
		storeMu.Unlock()
		dirty.Store(true)
	}
}

// ------------------------------------------------------------------ F14 layouts

func layoutsJSON(q map[string][]string) map[string]any {
	start, end, label := rangeBounds(q)
	totals := map[string]uint64{}
	storeMu.RLock()
	for k, d := range store.Days {
		t, err := time.ParseInLocation("2006-01-02", k, time.Local)
		if err != nil || t.Before(start) || !t.Before(end) {
			continue
		}
		for n, c := range d.Layouts {
			totals[n] += c
		}
	}
	cur, off := currentLayout, store.Settings.LayoutOff
	storeMu.RUnlock()
	type row struct {
		Name string `json:"name"`
		Keys uint64 `json:"keys"`
	}
	rows := []row{}
	for n, c := range totals {
		rows = append(rows, row{n, c})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Keys > rows[j].Keys || (rows[i].Keys == rows[j].Keys && rows[i].Name < rows[j].Name)
	})
	return map[string]any{"range": label, "layouts": rows, "current": cur, "tracking": !off}
}

// ------------------------------------------------------------------ A4 import preview

// importPreview describes what an import would change, without changing anything.
func importPreview(in *Store, mode string) map[string]any {
	var fk, fc, fs uint64
	from, to := "", ""
	for k, d := range in.Days {
		fk += d.Keys
		fc += d.Clicks
		fs += d.Scrolls
		if from == "" || k < from {
			from = k
		}
		if to == "" || k > to {
			to = k
		}
	}
	storeMu.RLock()
	defer storeMu.RUnlock()
	var ck, cc uint64
	for _, d := range store.Days {
		ck += d.Keys
		cc += d.Clicks
	}
	out := map[string]any{"mode": mode, "file": map[string]any{"days": len(in.Days), "hours": len(in.Buckets), "keys": fk, "clicks": fc, "scrolls": fs, "from": from, "to": to},
		"current": map[string]any{"days": len(store.Days), "hours": len(store.Buckets), "keys": ck, "clicks": cc}}
	if mode == "replace" {
		lost := 0
		for k := range store.Days {
			if in.Days[k] == nil {
				lost++
			}
		}
		out["daysOnlyInCurrent"] = lost
		return out
	}
	added, updated, days := 0, 0, 0
	for k, b := range in.Buckets {
		ex := store.Buckets[k]
		if ex == nil {
			added++
			continue
		}
		if biggerAny(ex.Keys, b.Keys) || biggerAny(ex.Mouse, b.Mouse) || biggerAny(ex.Scroll, b.Scroll) || biggerAny(ex.ScrollUnit, b.ScrollUnit) {
			updated++
		}
	}
	for k, d := range in.Days {
		if ex := store.Days[k]; ex == nil || dayTotal(d) > dayTotal(ex) {
			days++
		}
	}
	out["hoursAdded"], out["hoursUpdated"], out["daysApplied"] = added, updated, days
	return out
}

func biggerAny(dst, src map[string]uint64) bool {
	for k, v := range src {
		if v > dst[k] {
			return true
		}
	}
	return false
}
