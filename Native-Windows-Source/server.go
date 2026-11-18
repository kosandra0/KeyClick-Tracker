package main

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed ui
var uiFiles embed.FS

var (
	token     string
	serverURL string
	subsMu    sync.Mutex
	subs      = map[chan []byte]struct{}{}
	// hooks into the window layer (set by main.go)
	onExitRequest     func()
	onSettingsChanged func()
	onShowRequest     func()
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func newToken() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}

// ------------------------------------------------------------------- live feed

func broadcast(b []byte) {
	subsMu.Lock()
	defer subsMu.Unlock()
	for ch := range subs {
		select {
		case ch <- b:
		default: // subscriber stalled: drop it, the page reconnects and resyncs
			close(ch)
			delete(subs, ch)
		}
	}
}
func hasSubs() bool { subsMu.Lock(); defer subsMu.Unlock(); return len(subs) > 0 }

type liveFrame struct {
	pendingDelta
	WPM    float64 `json:"wpm"`
	CPM    float64 `json:"cpm"`
	KPM    float64 `json:"kpm"`
	Paused bool    `json:"p"`
}

// liveHeartbeat is how long the live feed may stay silent while pages are connected.
const liveHeartbeat = 2 * time.Second

// liveShouldSend decides whether the 500 ms tick sends a frame: when the speed changed, or when
// the last frame is older than the heartbeat.
func liveShouldSend(w, c, lastW, lastC float64, sinceLast time.Duration) bool {
	return w != lastW || c != lastC || sinceLast >= liveHeartbeat
}

// currentLiveFrame builds the JSON of a live frame with the speeds as they are now.
func currentLiveFrame(d pendingDelta) (b []byte, wpm, cpm float64) {
	w, c, k := liveRates(time.Now())
	b, _ = json.Marshal(liveFrame{d, w, c, k, isPaused()})
	return b, w, c
}

func startBroadcaster() {
	go func() {
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		var lastW, lastC float64
		var lastSent time.Time
		send := func(d pendingDelta) {
			if !hasSubs() {
				return
			}
			b, w, c := currentLiveFrame(d)
			lastW, lastC = w, c
			lastSent = time.Now()
			broadcast(b)
		}
		for {
			select {
			case <-signalLive:
				time.Sleep(25 * time.Millisecond) // coalesce bursts, still ~40 fps
				storeMu.Lock()
				d := pend
				pend = pendingDelta{}
				storeMu.Unlock()
				if !d.empty() {
					send(d)
				}
			case <-tick.C:
				w, c, _ := liveRates(time.Now())
				// Also send when nothing changed but a while has passed: if one frame was lost
				// (page reconnecting, hidden window), the next one corrects the display. Without
				// this a lost "0" frame left the speed stuck until the next key press.
				if liveShouldSend(w, c, lastW, lastC, time.Since(lastSent)) {
					send(pendingDelta{})
				}
			}
		}
	}()
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := make(chan []byte, 256)
	subsMu.Lock()
	subs[ch] = struct{}{}
	subsMu.Unlock()
	defer func() {
		subsMu.Lock()
		if _, ok := subs[ch]; ok {
			delete(subs, ch)
			close(ch)
		}
		subsMu.Unlock()
	}()
	fmt.Fprint(w, "retry: 800\n\n")
	// A page that connects or reconnects gets the current speed at once, so it never keeps
	// showing a number from before the connection was lost.
	first, _, _ := currentLiveFrame(pendingDelta{})
	fmt.Fprintf(w, "data: %s\n\n", first)
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case b, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// --------------------------------------------------------------- import/export

type exportEnvelope struct {
	Format        string         `json:"format"`
	FormatVersion int            `json:"formatVersion"`
	App           string         `json:"app"`
	AppVersion    string         `json:"appVersion"`
	Exported      string         `json:"exported"`
	Summary       *StatsResponse `json:"summary,omitempty"`
	Data          *Store         `json:"data"`
}

const exportMarker = `{"format":"keyclick-tracker-export"`

func snapshotStore() *Store {
	storeMu.Lock()
	rollMinuteLocked(time.Now(), false)
	b, _ := json.Marshal(&store)
	storeMu.Unlock()
	var cp Store
	_ = json.Unmarshal(b, &cp)
	return &cp
}

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
func fn(n uint64) string {
	s := strconv.FormatUint(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func reportText(s StatsResponse, md bool) string {
	var b strings.Builder
	h := func(t string) {
		if md {
			b.WriteString("\n## " + t + "\n\n")
		} else {
			b.WriteString("\n" + strings.ToUpper(t) + "\n" + strings.Repeat("-", len(t)) + "\n")
		}
	}
	line := func(k, v string) {
		if md {
			b.WriteString("- **" + k + ":** " + v + "\n")
		} else {
			fmt.Fprintf(&b, "%-26s %s\n", k+":", v)
		}
	}
	if md {
		b.WriteString("# " + appName + " report\n\n")
	} else {
		b.WriteString(appName + " report\n" + strings.Repeat("=", len(appName)+7) + "\n")
	}
	line("Generated", time.Now().Format("2006-01-02 15:04 MST"))
	line("Range", s.RangeLabel+" · grouped "+s.Unit)
	line("App version", appVersion)
	h("Totals")
	line("Key presses", fn(s.Totals.Keys))
	line("Mouse clicks", fn(s.Totals.Clicks))
	line("Scroll events", fn(s.Totals.Scrolls))
	line("All events", fn(s.Totals.Events))
	h("Speed")
	line("Overall WPM", f1(s.Rates.OverallWPM))
	line("Average WPM", f1(s.Rates.AvgWPM))
	line("Peak WPM", f1(s.Rates.PeakWPM))
	line("Overall CPM (clicks/min)", f1(s.Rates.OverallCPM))
	line("Average CPM", f1(s.Rates.AvgCPM))
	line("Peak CPM", f1(s.Rates.PeakCPM))
	line("Active minutes", strconv.Itoa(int(s.Rates.ActiveMinutes)))
	h("Streak")
	line("Current streak (days)", strconv.Itoa(s.Streak.Current))
	line("Longest streak (days)", strconv.Itoa(s.Streak.Longest))
	h("Most used keys")
	for i, k := range s.Keys {
		if i >= 15 {
			break
		}
		line(k.ID, fn(k.Count))
	}
	h("Mouse buttons")
	for _, k := range s.Mouse {
		line(k.ID, fn(k.Count))
	}
	h("Activity by period")
	for _, p := range s.Series {
		line(p.Label, fn(p.Events)+" events ("+fn(p.Keys)+" keys, "+fn(p.Clicks)+" clicks, "+fn(p.Scrolls)+" scrolls)")
	}
	return b.String()
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := first(q, "format", "json")
	stats := buildStats(q)
	env := exportEnvelope{Format: "keyclick-tracker-export", FormatVersion: 1, App: appName, AppVersion: appVersion, Exported: time.Now().Format(time.RFC3339), Data: snapshotStore()}
	stamp := time.Now().Format("2006-01-02")
	switch format {
	case "json":
		env.Summary = &stats
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=keyclick-tracker-"+stamp+".json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(env)
	case "txt", "md":
		blob, _ := json.Marshal(env)
		var b strings.Builder
		b.WriteString(reportText(stats, format == "md"))
		if format == "md" {
			b.WriteString("\n---\n\n## Restorable data\n\nThe block below lets " + appName + " restore this data (Data → Import). Please do not edit it.\n\n```json\n" + string(blob) + "\n```\n")
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		} else {
			b.WriteString("\n=== RESTORABLE DATA (do not edit) ===\n" + string(blob) + "\n=== END OF DATA ===\n")
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		w.Header().Set("Content-Disposition", "attachment; filename=keyclick-tracker-"+stamp+"."+format)
		_, _ = io.WriteString(w, b.String())
	default:
		// Only JSON, TXT and MD are supported for backups and reports.
		http.Error(w, "Unsupported format. Use json, txt or md.", http.StatusBadRequest)
	}
}

// parseImport accepts a .json export, a raw activity.json, or a .txt/.md
// report containing the restorable data block.
func parseImport(raw []byte) (*Store, error) {
	text := bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	trim := bytes.TrimSpace(text)
	var src []byte
	if len(trim) > 0 && trim[0] == '{' {
		src = trim
	} else if i := bytes.Index(text, []byte(exportMarker)); i >= 0 {
		src = text[i:]
	} else {
		return nil, fmt.Errorf("no %s data found in this file", appName)
	}
	var probe map[string]json.RawMessage
	if err := json.NewDecoder(bytes.NewReader(src)).Decode(&probe); err != nil {
		return nil, fmt.Errorf("the file could not be read as %s data (%v)", appName, err)
	}
	var st Store
	switch {
	case probe["format"] != nil && probe["data"] != nil:
		if err := json.Unmarshal(probe["data"], &st); err != nil {
			return nil, fmt.Errorf("data block is damaged (%v)", err)
		}
	case probe["buckets"] != nil:
		if err := json.Unmarshal(src, &st); err != nil {
			return nil, fmt.Errorf("data is damaged (%v)", err)
		}
	default:
		return nil, fmt.Errorf("this file is not a %s or KeyScope data file", appName)
	}
	if st.Version > storeVersion {
		return nil, fmt.Errorf("this file comes from a newer version (data format %d); update the app first", st.Version)
	}
	for k := range st.Buckets {
		if _, e := time.Parse(time.RFC3339, k); e != nil {
			return nil, fmt.Errorf("invalid hour entry %q", k)
		}
	}
	for k := range st.Days {
		if _, e := time.Parse("2006-01-02", k); e != nil {
			return nil, fmt.Errorf("invalid day entry %q", k)
		}
	}
	if len(st.Buckets) == 0 && len(st.Days) == 0 {
		return nil, fmt.Errorf("the file contains no activity data")
	}
	st.normalize()
	return &st, nil
}

func maxInto(dst, src map[string]uint64) bool {
	changed := false
	for k, v := range src {
		if v > dst[k] {
			dst[k] = v
			changed = true
		}
	}
	return changed
}
func dayTotal(d *DayRoll) uint64 { return d.Keys + d.Clicks + d.Scrolls }

// importData is idempotent: importing the same file twice changes nothing,
// because each counter takes the larger of the local and imported value.
func importData(in *Store, mode string) (added, updated, days int, backup string, err error) {
	if p, e := backupNow("before-import"); e == nil {
		backup = filepath.Base(p)
	} else if !os.IsNotExist(e) {
		return 0, 0, 0, "", fmt.Errorf("could not create a safety backup first: %v", e)
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	if mode == "replace" {
		store.Buckets, store.Days = in.Buckets, in.Days
		return len(in.Buckets), 0, len(in.Days), backup, nil
	}
	for k, b := range in.Buckets {
		ex := store.Buckets[k]
		if ex == nil {
			store.Buckets[k] = b
			added++
			continue
		}
		c1 := maxInto(ex.Keys, b.Keys)
		c2 := maxInto(ex.Mouse, b.Mouse)
		c3 := maxInto(ex.Scroll, b.Scroll)
		c4 := maxInto(ex.ScrollUnit, b.ScrollUnit)
		if c1 || c2 || c3 || c4 {
			updated++
		}
	}
	for k, d := range in.Days {
		if ex := store.Days[k]; ex == nil || dayTotal(d) > dayTotal(ex) {
			store.Days[k] = d
			days++
		}
	}
	return
}

// -------------------------------------------------------------------- handler

func listBackups() []map[string]any {
	es, _ := os.ReadDir(backupDir())
	out := []map[string]any{}
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			out = append(out, map[string]any{"name": e.Name(), "size": fi.Size(), "modified": fi.ModTime().Format(time.RFC3339)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["modified"].(string) > out[j]["modified"].(string) })
	return out
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	t := r.Header.Get("X-KeyScope-Token")
	if t == "" {
		t = r.URL.Query().Get("token")
	}
	if t != token {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	switch r.URL.Path {
	case "/api/status":
		if r.Method == http.MethodPost {
			var body struct {
				Paused *bool `json:"paused"`
			}
			if json.NewDecoder(r.Body).Decode(&body) == nil && body.Paused != nil {
				setPaused(*body.Paused)
			}
		}
		storeMu.RLock()
		ct := store.CloseToTray
		storeMu.RUnlock()
		writeJSON(w, map[string]any{"paused": isPaused(), "agentConnected": hooksOK(), "monitoring": !isPaused(), "closeToTray": ct,
			"dataPath": dataPath, "version": appVersion, "name": appName, "startup": startupEnabled(), "quiet": quietNow()})
	case "/api/stats":
		writeJSON(w, buildStats(q))
	case "/api/idle": // v2.0.0 (F2): active and idle time for the range
		writeJSON(w, idleStats(q))
	case "/api/health": // v2.0.0 (F10): read-only data health check
		writeJSON(w, healthStats())
	case "/api/hotkeys": // v2.0.0 (F12): status of the global hotkeys
		writeJSON(w, hotkeysJSON())
	case "/api/apps": // v2.0.0 (F1): per-program totals
		writeJSON(w, appsJSON(q))
	case "/api/backups/preview": // v2.0.0 (F9): read-only look at a backup
		status, body := backupPreview(q.Get("name"))
		writeJSONStatus(w, status, body)
	case "/api/backups/restore": // v2.0.0 (F9): saves current data first, then restores
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}
		var in struct {
			Name    string `json:"name"`
			Confirm string `json:"confirm"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "Bad request. Nothing was changed."})
			return
		}
		status, body := restoreBackup(in.Name, in.Confirm)
		writeJSONStatus(w, status, body)
	case "/api/live":
		sseHandler(w, r)
	case "/api/settings":
		if r.Method == http.MethodPost {
			var in struct {
				Settings
				CloseToTray *bool `json:"close_to_tray"`
			}
			storeMu.RLock()
			in.Settings = *store.Settings
			storeMu.RUnlock()
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
				http.Error(w, "bad settings", 400)
				return
			}
			s := in.Settings
			if len(s.Theme) > 40 || len(s.Font) > 40 || len(s.OverlayTheme) > 40 || len(s.AppLogo) > 60 {
				http.Error(w, "bad settings", 400)
				return
			}
			storeMu.Lock()
			applyOverlayPreset(store.Settings, &s) // F13: each preset remembers its own size
			*store.Settings = s
			if in.CloseToTray != nil {
				store.CloseToTray = *in.CloseToTray
			}
			store.normalize()
			storeMu.Unlock()
			_ = saveStore()
			if onSettingsChanged != nil {
				onSettingsChanged()
			}
		}
		storeMu.RLock()
		out := map[string]any{"settings": *store.Settings, "close_to_tray": store.CloseToTray}
		storeMu.RUnlock()
		writeJSON(w, out)
	case "/api/startup":
		if r.Method == http.MethodPost {
			var b struct {
				Enabled bool `json:"enabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			if err := setStartup(b.Enabled); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		writeJSON(w, map[string]bool{"enabled": startupEnabled()})
	case "/api/export":
		exportHandler(w, r)
	case "/api/import":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 128<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		in, err := parseImport(raw)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		mode := "merge"
		if first(q, "mode", "merge") == "replace" {
			mode = "replace"
		}
		a, u, d, bk, err := importData(in, mode)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = saveStore()
		writeJSON(w, map[string]any{"mode": mode, "hoursAdded": a, "hoursUpdated": u, "daysApplied": d, "backup": bk})
	case "/api/backups":
		if r.Method == http.MethodPost {
			p, err := backupNow("manual")
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writeJSON(w, map[string]any{"created": filepath.Base(p)})
			return
		}
		writeJSON(w, listBackups())
	case "/api/openfolder":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}
		openFolder(dataDir)
		writeJSON(w, map[string]bool{"opened": true})
	case "/api/save":
		if err := saveStore(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]bool{"saved": true})
	case "/api/data":
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", 405)
			return
		}
		var b struct {
			Confirm string `json:"confirm"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b.Confirm != "DELETE" {
			http.Error(w, "confirmation missing", 400)
			return
		}
		_ = saveStore()
		bk, err := backupNow("before-clear")
		if err != nil {
			http.Error(w, "could not create a safety backup, nothing was deleted: "+err.Error(), 500)
			return
		}
		storeMu.Lock()
		store.Buckets = map[string]*HourBucket{}
		store.Days = map[string]*DayRoll{}
		cur = minuteAcc{start: time.Now().Truncate(time.Minute)}
		storeMu.Unlock()
		_ = saveStore()
		writeJSON(w, map[string]any{"cleared": true, "backup": filepath.Base(bk)})
	case "/api/dev/inject": // automated-test helper; only exists when KEYCLICK_DEV=1 is set
		if os.Getenv("KEYCLICK_DEV") != "1" {
			http.NotFound(w, r)
			return
		}
		n, _ := strconv.Atoi(first(q, "n", "1"))
		for i := 0; i < n && i < 10000; i++ {
			if k := first(q, "kind", "key"); k == "shortcut" || k == "modkey" {
				recordCombo(k) // test hook for the F4 counters
			} else {
				record(k, first(q, "id", "A"), 1)
			}
		}
		writeJSON(w, map[string]int{"injected": n})
	case "/api/exit":
		if onExitRequest != nil {
			onExitRequest()
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		if api2Handler(w, r, q) { // v2.0.0 endpoints (api2.go)
			return
		}
		http.NotFound(w, r)
	}
}

func startServer() (net.Listener, error) {
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	token = newToken()
	sub, _ := fs.Sub(uiFiles, "ui")
	mux := http.NewServeMux()
	mux.Handle("/ui/", http.StripPrefix("/ui/", noStore(http.FileServer(http.FS(sub)))))
	mux.HandleFunc("/api/", apiHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, _ := uiFiles.ReadFile("ui/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	})
	go func() {
		if e := http.Serve(ln, mux); e != nil && e != http.ErrServerClosed {
			logLine("HTTP server: " + e.Error())
		}
	}()
	startBroadcaster()
	return ln, nil
}

func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
