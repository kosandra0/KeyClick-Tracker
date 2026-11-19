package main

// v2.4.0 input duration tracking, scroll sessions and the per-application detail.
//
// Timing model (documented here because the user interface explains it):
//   - A key or mouse button hold is measured from the key-down / button-down to the matching
//     key-up / button-up with Go's monotonic clock (time.Since), never by subtracting wall-clock
//     times. OS auto-repeat key-downs do not start a new hold. Every key and button is tracked
//     independently, so Ctrl+Shift+A is three concurrent holds.
//   - The application is the foreground program when the hold STARTED, kept until release.
//   - A hold is added to the day on which it started (it is not split at midnight).
//   - A key-up whose key-down was consumed by Windows (Win and Alt shortcuts, see handleKey) is
//     counted as a press but has no measurable duration, so it adds nothing to the hold totals.
//   - Holds longer than maxHold, and holds still open when recording is paused, are dropped as
//     "interrupted": they never add hours of false hold time.
//   - Scrolling has no physical hold. Wheel events less than scrollGap apart form one SESSION.
//     The session span is last event minus first event (an estimate); a one-event session has no
//     span and is counted as "single". A session is also split when the foreground program changes.
//   - Nothing here stores typed text, window titles or full paths.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	scrollGap     = 400 * time.Millisecond // central setting: wheel events closer than this are one session
	maxHold       = 15 * time.Minute       // a longer hold is treated as a lost key-up
	recentLimit   = 2000                   // newest interval records kept for History and carried in every export
	unknownAppKey = "(protected)"
)

// HoldStat is the measured holds of one key or button: N measured holds, total and longest ms.
type HoldStat struct {
	N   uint64 `json:"n,omitempty"`
	Ms  uint64 `json:"ms,omitempty"`
	Max uint64 `json:"mx,omitempty"`
}

func (h *HoldStat) add(ms uint64) {
	h.N++
	h.Ms += ms
	if ms > h.Max {
		h.Max = ms
	}
}

// ScrollAgg is one day's scroll sessions.
type ScrollAgg struct {
	Sessions uint64            `json:"n,omitempty"`
	Events   uint64            `json:"e,omitempty"`
	SpanMs   uint64            `json:"ms,omitempty"`
	MaxMs    uint64            `json:"mx,omitempty"`
	Single   uint64            `json:"one,omitempty"` // sessions with one event: no span
	Dir      map[string]uint64 `json:"d,omitempty"`
}

// Interval is one history record: a key hold, a mouse button hold or a scroll session.
type Interval struct {
	K   string `json:"k"`            // key, mouse or scroll
	ID  string `json:"id"`           // key name, button, or scroll direction (up, down, left, right, mixed)
	St  int64  `json:"st"`           // start, unix milliseconds
	Ms  uint64 `json:"ms,omitempty"` // measured hold, or estimated scroll span
	App string `json:"app,omitempty"`
	N   uint64 `json:"n,omitempty"`   // scroll events in the session
	Sts string `json:"sts,omitempty"` // "" completed, "intr" interrupted
	// Seq numbers every recorded hold and scroll session in order. It lives in memory only (never
	// saved): the page uses it to know exactly which holds a total already contains.
	Seq uint64 `json:"-"`
}

// holdSeq is the number of the last recorded hold or scroll session. Guarded by storeMu.
var holdSeq uint64

// lastScroll is the scroll session that closed most recently. Guarded by storeMu.
var lastScroll struct{ Seq, SpanMs, Events uint64 }

// ----------------------------------------------------------------- live held state (input thread)

type heldItem struct {
	start time.Time // carries the monotonic reading
	app   string
	kind  string // k or m
	id    string
}

var (
	heldMu  sync.Mutex
	heldNow = map[string]*heldItem{}
	// currentAppAtomic mirrors currentApp so the input thread can read it without storeMu.
	currentAppAtomic atomic.Value
)

func appNow() string {
	if v, ok := currentAppAtomic.Load().(string); ok {
		return v
	}
	return ""
}

// holdDown starts a hold unless the same key or button is already held (auto-repeat).
func holdDown(kind, id string) {
	k := kind + ":" + id
	heldMu.Lock()
	if _, ok := heldNow[k]; !ok {
		heldNow[k] = &heldItem{start: time.Now(), app: appNow(), kind: kind, id: id}
	}
	heldMu.Unlock()
}

// holdUp ends a hold and queues the measured interval for the recorder.
func holdUp(kind, id string) {
	k := kind + ":" + id
	heldMu.Lock()
	h := heldNow[k]
	delete(heldNow, k)
	heldMu.Unlock()
	if h == nil {
		return
	}
	ms := uint64(time.Since(h.start) / time.Millisecond)
	select {
	case hookEvents <- hookEvent{kind: "hold", id: kind + ":" + id, units: ms, app: h.app, t: h.start}:
	default:
	}
}

// clearHeld forgets every open hold without recording it (pause, sleep).
func clearHeld() {
	heldMu.Lock()
	heldNow = map[string]*heldItem{}
	heldMu.Unlock()
}

// sweepHolds drops holds that exceeded maxHold (a lost key-up) and everything while paused.
func sweepHolds(now time.Time) {
	var gone []*heldItem
	paused := isPaused()
	heldMu.Lock()
	for k, h := range heldNow {
		if paused || now.Sub(h.start) > maxHold {
			gone = append(gone, h)
			delete(heldNow, k)
		}
	}
	heldMu.Unlock()
	if paused {
		return
	}
	for _, h := range gone {
		select {
		case hookEvents <- hookEvent{kind: "hold", id: h.kind + ":" + h.id, sub: "intr", app: h.app, t: h.start}:
		default:
		}
	}
}

// ----------------------------------------------------------------- recorder side

type scrollSess struct {
	app         string
	first, last time.Time
	events      uint64
	units       uint64
	dirs        map[string]uint64
}

var (
	sess      *scrollSess // recorder goroutine only
	liveScrMu sync.Mutex
	liveScr   struct {
		last   time.Time
		events uint64
		spanMs uint64
		dir    string
	}
)

func (s *scrollSess) dominant() string {
	if len(s.dirs) == 1 {
		for d := range s.dirs {
			return d
		}
	}
	return "mixed"
}

// scrollEvent adds one wheel event to the current session, closing the previous one when the
// gap was long enough or the foreground program changed.
func scrollEvent(ev hookEvent) {
	if sess != nil && (ev.app != sess.app || ev.t.Sub(sess.last) > scrollGap) {
		closeScroll()
	}
	if sess == nil {
		sess = &scrollSess{app: ev.app, first: ev.t, dirs: map[string]uint64{}}
	}
	sess.last = ev.t
	sess.events++
	sess.units += ev.units
	sess.dirs[ev.id]++
	liveScrMu.Lock()
	liveScr.last, liveScr.events, liveScr.spanMs, liveScr.dir = ev.t, sess.events, uint64(sess.last.Sub(sess.first)/time.Millisecond), sess.dominant()
	liveScrMu.Unlock()
}

// closeStaleScroll ends the session once no wheel event arrived for scrollGap.
func closeStaleScroll(now time.Time) {
	if sess != nil && now.Sub(sess.last) > scrollGap {
		closeScroll()
	}
}

func closeScroll() {
	s := sess
	sess = nil
	if s == nil {
		return
	}
	span := uint64(s.last.Sub(s.first) / time.Millisecond)
	if s.events == 1 {
		span = 0
	}
	storeMu.Lock()
	d := dayFor(s.first)
	if d.Scr == nil {
		d.Scr = &ScrollAgg{}
	}
	d.Scr.Sessions++
	d.Scr.Events += s.events
	d.Scr.SpanMs += span
	if span > d.Scr.MaxMs {
		d.Scr.MaxMs = span
	}
	if s.events == 1 {
		d.Scr.Single++
	}
	if d.Scr.Dir == nil {
		d.Scr.Dir = map[string]uint64{}
	}
	for dir, n := range s.dirs {
		d.Scr.Dir[dir] += n
	}
	if s.app != "" && store.Settings != nil && !store.Settings.AppTrackingOff {
		ac := appEntryLocked(s.first, s.app)
		ac.Sess++
		ac.SessMs += span
		if ac.ScDir == nil {
			ac.ScDir = map[string]uint64{}
		}
		for dir, n := range s.dirs {
			ac.ScDir[dir] += n
		}
	}
	holdSeq++
	lastScroll.Seq, lastScroll.SpanMs, lastScroll.Events = holdSeq, span, s.events
	addRecentLocked(Interval{K: "scroll", ID: s.dominant(), St: s.first.UnixMilli(), Ms: span, App: s.app, N: s.events, Seq: holdSeq})
	dirty.Store(true)
	storeMu.Unlock()
}

// recordHold stores one finished (or interrupted) hold. Runs on the recorder goroutine.
func recordHold(ev hookEvent) {
	parts := strings.SplitN(ev.id, ":", 2)
	if len(parts) != 2 {
		return
	}
	kind, id := parts[0], parts[1]
	storeMu.Lock()
	defer storeMu.Unlock()
	k := "key"
	if kind == "m" {
		k = "mouse"
	}
	if ev.sub == "intr" {
		addRecentLocked(Interval{K: k, ID: id, St: ev.t.UnixMilli(), App: ev.app, Sts: "intr"})
		dirty.Store(true)
		return
	}
	d := dayFor(ev.t)
	m := &d.KeyHold
	if kind == "m" {
		m = &d.MsHold
	}
	if *m == nil {
		*m = map[string]*HoldStat{}
	}
	hs := (*m)[id]
	if hs == nil {
		hs = &HoldStat{}
		(*m)[id] = hs
	}
	hs.add(ev.units)
	if ev.app != "" && store.Settings != nil && !store.Settings.AppTrackingOff {
		ac := appEntryLocked(ev.t, ev.app)
		am := &ac.KeyHold
		if kind == "m" {
			am = &ac.MsHold
		}
		if *am == nil {
			*am = map[string]*HoldStat{}
		}
		ah := (*am)[id]
		if ah == nil {
			ah = &HoldStat{}
			(*am)[id] = ah
		}
		ah.add(ev.units)
	}
	holdSeq++
	addRecentLocked(Interval{K: k, ID: id, St: ev.t.UnixMilli(), Ms: ev.units, App: ev.app, Seq: holdSeq})
	dirty.Store(true)
}

func addRecentLocked(iv Interval) {
	store.Recent = append(store.Recent, iv)
	if n := len(store.Recent); n > recentLimit {
		store.Recent = append([]Interval(nil), store.Recent[n-recentLimit:]...)
	}
}

// ----------------------------------------------------------------- API

// heldJSON is GET /api/held: what is held right now, with each key's total held time today
// (finished holds plus the one in progress), and the current scroll session.
func heldJSON() map[string]any {
	now := time.Now()
	type snap struct {
		kind, id, app string
		ms            uint64
	}
	var items []snap
	heldMu.Lock()
	for _, h := range heldNow {
		items = append(items, snap{h.kind, h.id, h.app, uint64(now.Sub(h.start) / time.Millisecond)})
	}
	heldMu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].ms > items[j].ms })
	keys, btns := []map[string]any{}, []map[string]any{}
	storeMu.RLock()
	d := store.Days[dayKey(now)]
	// Holds that ended in the last 1.5 s, so a quick tap still shows (and glows) on the page.
	released := []map[string]any{}
	for i, n := len(store.Recent)-1, 0; i >= 0 && n < 40; i, n = i-1, n+1 {
		iv := store.Recent[i]
		if iv.K == "scroll" || iv.Sts != "" {
			continue
		}
		ago := now.UnixMilli() - (iv.St + int64(iv.Ms))
		if ago > 1500 {
			continue
		}
		var hs *HoldStat
		if d != nil {
			m := d.KeyHold
			if iv.K == "mouse" {
				m = d.MsHold
			}
			hs = m[iv.ID]
		}
		row := map[string]any{"k": iv.K, "id": iv.ID, "st": iv.St, "seq": iv.Seq, "ms": iv.Ms, "ago_ms": ago, "app": iv.App, "total_ms": uint64(0), "holds": uint64(0)}
		if hs != nil {
			row["total_ms"], row["holds"] = hs.Ms, hs.N
		}
		released = append(released, row)
	}
	for _, it := range items {
		var base HoldStat
		if d != nil {
			m := d.KeyHold
			if it.kind == "m" {
				m = d.MsHold
			}
			if hs := m[it.id]; hs != nil {
				base = *hs
			}
		}
		row := map[string]any{"id": it.id, "ms": it.ms, "app": it.app, "total_ms": base.Ms + it.ms, "holds": base.N + 1}
		if it.kind == "k" {
			keys = append(keys, row)
		} else {
			btns = append(btns, row)
		}
	}
	closed := map[string]any{"seq": lastScroll.Seq, "span_ms": lastScroll.SpanMs, "events": lastScroll.Events}
	storeMu.RUnlock()
	liveScrMu.Lock()
	sc := map[string]any{"active": !liveScr.last.IsZero() && now.Sub(liveScr.last) <= scrollGap, "events": liveScr.events, "span_ms": liveScr.spanMs, "dir": liveScr.dir, "closed": closed}
	liveScrMu.Unlock()
	return map[string]any{"keys": keys, "mouse": btns, "released": released, "scroll": sc, "paused": isPaused(), "gap_ms": scrollGap / time.Millisecond, "fg": appNow()}
}

// durationJSON is GET /api/duration: measured holds and scroll sessions for the range.
func durationJSON(q map[string][]string) map[string]any {
	start, end, label := rangeBounds(q)
	keys, btns := map[string]*HoldStat{}, map[string]*HoldStat{}
	var sc ScrollAgg
	sc.Dir = map[string]uint64{}
	first := ""
	merge := func(dst map[string]*HoldStat, src map[string]*HoldStat) {
		for id, h := range src {
			t := dst[id]
			if t == nil {
				t = &HoldStat{}
				dst[id] = t
			}
			t.N += h.N
			t.Ms += h.Ms
			if h.Max > t.Max {
				t.Max = h.Max
			}
		}
	}
	storeMu.RLock()
	for k, d := range store.Days {
		t, err := time.ParseInLocation("2006-01-02", k, time.Local)
		if err != nil || t.Before(start) || !t.Before(end) {
			continue
		}
		if len(d.KeyHold)+len(d.MsHold) > 0 || d.Scr != nil {
			if first == "" || k < first {
				first = k
			}
		}
		merge(keys, d.KeyHold)
		merge(btns, d.MsHold)
		if d.Scr != nil {
			sc.Sessions += d.Scr.Sessions
			sc.Events += d.Scr.Events
			sc.SpanMs += d.Scr.SpanMs
			sc.Single += d.Scr.Single
			if d.Scr.MaxMs > sc.MaxMs {
				sc.MaxMs = d.Scr.MaxMs
			}
			for dir, n := range d.Scr.Dir {
				sc.Dir[dir] += n
			}
		}
	}
	seq := holdSeq
	storeMu.RUnlock()
	return map[string]any{"range": label, "keys": keys, "mouse": btns, "scroll": sc, "since": first, "seq": seq}
}

// recentJSON is GET /api/recent: newest interval records inside the range.
func recentJSON(q map[string][]string) map[string]any {
	start, end, label := rangeBounds(q)
	out := []Interval{}
	storeMu.RLock()
	for i := len(store.Recent) - 1; i >= 0; i-- {
		iv := store.Recent[i]
		t := time.UnixMilli(iv.St)
		if t.Before(start) || !t.Before(end) {
			continue
		}
		out = append(out, iv)
	}
	storeMu.RUnlock()
	return map[string]any{"range": label, "limit": recentLimit, "rows": out}
}

// apps2JSON is GET /api/apps2: every program with the full per-key and per-button detail.
func apps2JSON(q map[string][]string) map[string]any {
	start, end, label := rangeBounds(q)
	type row struct {
		Name   string               `json:"name"`
		Keys   uint64               `json:"keys"`
		Clicks uint64               `json:"clicks"`
		Scroll uint64               `json:"scroll"`
		Secs   uint64               `json:"secs"`
		Last   int64                `json:"last"`
		KI     map[string]uint64    `json:"ki"`
		MI     map[string]uint64    `json:"mi"`
		KH     map[string]*HoldStat `json:"kh"`
		MH     map[string]*HoldStat `json:"mh"`
		Sess   uint64               `json:"sess"`
		SessMs uint64               `json:"sess_ms"`
		SD     map[string]uint64    `json:"sd"`
	}
	agg := map[string]*row{}
	var totK, totC, totS uint64
	storeMu.RLock()
	for k, d := range store.Days {
		t, err := time.ParseInLocation("2006-01-02", k, time.Local)
		if err != nil || t.Before(start) || !t.Before(end) {
			continue
		}
		totK += d.Keys
		totC += d.Clicks
		totS += d.Scrolls
		for name, ac := range d.Apps {
			r := agg[name]
			if r == nil {
				r = &row{Name: name, KI: map[string]uint64{}, MI: map[string]uint64{}, KH: map[string]*HoldStat{}, MH: map[string]*HoldStat{}, SD: map[string]uint64{}}
				agg[name] = r
			}
			r.Keys += ac.Keys
			r.Clicks += ac.Clicks
			r.Scroll += ac.Scroll
			r.Secs += ac.Secs
			r.Sess += ac.Sess
			r.SessMs += ac.SessMs
			if ac.Last > r.Last {
				r.Last = ac.Last
			}
			for id, n := range ac.KeyIDs {
				r.KI[id] += n
			}
			for id, n := range ac.MsIDs {
				r.MI[id] += n
			}
			for id, n := range ac.ScDir {
				r.SD[id] += n
			}
			for id, h := range ac.KeyHold {
				t := r.KH[id]
				if t == nil {
					t = &HoldStat{}
					r.KH[id] = t
				}
				t.N += h.N
				t.Ms += h.Ms
				if h.Max > t.Max {
					t.Max = h.Max
				}
			}
			for id, h := range ac.MsHold {
				t := r.MH[id]
				if t == nil {
					t = &HoldStat{}
					r.MH[id] = t
				}
				t.N += h.N
				t.Ms += h.Ms
				if h.Max > t.Max {
					t.Max = h.Max
				}
			}
		}
	}
	tracking := store.Settings != nil && !store.Settings.AppTrackingOff
	seq := holdSeq
	storeMu.RUnlock()
	rows := make([]*row, 0, len(agg))
	for _, r := range agg {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].Keys+rows[i].Clicks+rows[i].Scroll, rows[j].Keys+rows[j].Clicks+rows[j].Scroll
		if a != b {
			return a > b
		}
		return rows[i].Name < rows[j].Name
	})
	if len(rows) > 200 {
		rows = rows[:200]
	}
	return map[string]any{"range": label, "tracking": tracking, "apps": rows, "total_keys": totK, "total_clicks": totC, "total_scroll": totS, "seq": seq}
}

// ----------------------------------------------------------------- native application icons

var (
	appPaths       sync.Map // program file name -> full path. In memory only: paths are never stored in the data file.
	user32Icon     = syscall.NewLazyDLL("user32.dll")
	pExtractIconEx = shell32.NewProc("ExtractIconExW")
	pGetIconInfo   = user32Icon.NewProc("GetIconInfo")
	pDestroyIcon   = user32Icon.NewProc("DestroyIcon")
	pGetDC         = user32Icon.NewProc("GetDC")
	pReleaseDC     = user32Icon.NewProc("ReleaseDC")
	pGetObjectW    = gdi32.NewProc("GetObjectW")
	pGetDIBits     = gdi32.NewProc("GetDIBits")
)

func rememberAppPath(name, path string) {
	if name != "" && path != "" {
		appPaths.Store(name, path)
	}
}

func iconFile(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	return filepath.Join(dataDir, "AppIcons", clean+".png")
}

// appIconPNG returns the cached PNG of a program's own icon, extracting it the first time the
// program has been seen in this run. false means no icon is available (the page shows a letter).
func appIconPNG(name string) ([]byte, bool) {
	f := iconFile(name)
	if b, err := os.ReadFile(f); err == nil {
		return b, true
	}
	v, ok := appPaths.Load(name)
	if !ok {
		return nil, false
	}
	b, err := extractIconPNG(v.(string))
	if err != nil || len(b) == 0 {
		return nil, false
	}
	_ = os.MkdirAll(filepath.Dir(f), 0700)
	_ = os.WriteFile(f, b, 0600)
	return b, true
}

type bitmapT struct {
	Type, Width, Height, WidthBytes int32
	Planes, BitsPixel               uint16
	Bits                            uintptr
}
type iconInfoT struct {
	IsIcon      int32
	XHot, YHot  uint32
	Mask, Color uintptr
}
type bmiHeader struct {
	Size                 uint32
	Width, Height        int32
	Planes, BitCount     uint16
	Compression, SizeImg uint32
	XPels, YPels         int32
	ClrUsed, ClrImp      uint32
}

func extractIconPNG(path string) ([]byte, error) {
	var large uintptr
	n, _, _ := pExtractIconEx.Call(uintptr(unsafe.Pointer(utf16p(path))), 0, uintptr(unsafe.Pointer(&large)), 0, 1)
	if n == 0 || large == 0 {
		return nil, os.ErrNotExist
	}
	defer pDestroyIcon.Call(large)
	var ii iconInfoT
	if r, _, _ := pGetIconInfo.Call(large, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil, os.ErrInvalid
	}
	defer pDeleteObject.Call(ii.Color)
	if ii.Mask != 0 {
		defer pDeleteObject.Call(ii.Mask)
	}
	var bm bitmapT
	pGetObjectW.Call(ii.Color, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	w, h := int(bm.Width), int(bm.Height)
	if w <= 0 || h <= 0 || w > 512 || h > 512 {
		return nil, os.ErrInvalid
	}
	dc, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, dc)
	read := func(bmp uintptr) []byte {
		hdr := bmiHeader{Size: uint32(unsafe.Sizeof(bmiHeader{})), Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
		buf := make([]byte, w*h*4)
		r, _, _ := pGetDIBits.Call(dc, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&hdr)), 0)
		if r == 0 {
			return nil
		}
		return buf
	}
	col := read(ii.Color)
	if col == nil {
		return nil, os.ErrInvalid
	}
	hasAlpha := false
	for i := 3; i < len(col); i += 4 {
		if col[i] != 0 {
			hasAlpha = true
			break
		}
	}
	var mask []byte
	if !hasAlpha && ii.Mask != 0 {
		mask = read(ii.Mask)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		b, g, r, a := col[i*4], col[i*4+1], col[i*4+2], col[i*4+3]
		if !hasAlpha {
			a = 255
			if mask != nil && mask[i*4] != 0 { // mask bit set = transparent
				a = 0
			}
		}
		img.SetNRGBA(i%w, i/w, color.NRGBA{R: r, G: g, B: b, A: a})
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// appIconHandler serves GET /api/appicon?name=program.exe.
func appIconHandler(w http.ResponseWriter, q map[string][]string) {
	name := first(q, "name", "")
	if name == "" || len(name) > 200 || strings.ContainsAny(name, "/\\:") {
		w.WriteHeader(404)
		return
	}
	b, ok := appIconPNG(name)
	if !ok {
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(b)
}

var _ = strconv.Itoa

// ----------------------------------------------------------------- History: hold time per period and per day

func holdSumMs(m map[string]*HoldStat) (ms, n uint64) {
	for _, h := range m {
		ms += h.Ms
		n += h.N
	}
	return
}

// durSeriesJSON is GET /api/durseries: measured hold time for each period of the History table, in
// the same periods (unit) and range as /api/stats. Holds are kept per day, so hourly periods have none.
func durSeriesJSON(q map[string][]string) map[string]any {
	start, end, label := rangeBounds(q)
	unit := first(q, "unit", "daily")
	switch unit {
	case "hour", "daily", "weekly", "monthly", "quarterly", "yearly":
	default:
		unit = "daily"
	}
	startDay := midnight(start)
	rows := map[string]map[string]uint64{}
	storeMu.RLock()
	if unit != "hour" {
		for k, d := range store.Days {
			t, err := time.ParseInLocation("2006-01-02", k, time.Local)
			if err != nil || t.Before(startDay) || !t.Before(end) {
				continue
			}
			bk, _ := bucketFor(t, unit)
			r := rows[bk]
			if r == nil {
				r = map[string]uint64{}
				rows[bk] = r
			}
			kms, kn := holdSumMs(d.KeyHold)
			mms, mn := holdSumMs(d.MsHold)
			r["k_ms"] += kms
			r["k_n"] += kn
			r["m_ms"] += mms
			r["m_n"] += mn
			if d.Scr != nil {
				r["s_ms"] += d.Scr.SpanMs
				r["s_n"] += d.Scr.Sessions
			}
		}
	}
	seq := holdSeq
	storeMu.RUnlock()
	nowKey, _ := bucketFor(time.Now(), unit)
	return map[string]any{"range": label, "unit": unit, "rows": rows, "now_key": nowKey, "hourly": unit == "hour", "seq": seq}
}

// durDayJSON is GET /api/durday?date=YYYY-MM-DD: everything held and scrolled on one day.
func durDayJSON(date string) map[string]any {
	storeMu.RLock()
	defer storeMu.RUnlock()
	out := map[string]any{"date": date, "seq": holdSeq, "key_hold": map[string]*HoldStat{}, "mouse_hold": map[string]*HoldStat{}, "scroll": nil, "apps": []map[string]any{}}
	d := store.Days[date]
	// the counts of the day, including the minute that is still running when the day is today
	var keys, clicks, scrolls uint64
	if d != nil {
		keys, clicks, scrolls = d.Keys, d.Clicks, d.Scrolls
	}
	if date == dayKey(time.Now()) {
		keys, clicks, scrolls = keys+cur.keys, clicks+cur.clicks, scrolls+cur.scroll
	}
	out["keys"], out["clicks"], out["scrolls"] = keys, clicks, scrolls
	if d == nil {
		return out
	}
	if d.KeyHold != nil {
		out["key_hold"] = d.KeyHold
	}
	if d.MsHold != nil {
		out["mouse_hold"] = d.MsHold
	}
	if d.Scr != nil {
		out["scroll"] = d.Scr
	}
	apps := make([]map[string]any, 0, len(d.Apps))
	for name, a := range d.Apps {
		kms, _ := holdSumMs(a.KeyHold)
		mms, _ := holdSumMs(a.MsHold)
		apps = append(apps, map[string]any{"name": name, "keys": a.Keys, "clicks": a.Clicks, "scroll": a.Scroll, "k_ms": kms, "m_ms": mms, "s_ms": a.SessMs, "sess": a.Sess, "last": a.Last})
	}
	sort.Slice(apps, func(i, j int) bool {
		a := apps[i]["keys"].(uint64) + apps[i]["clicks"].(uint64) + apps[i]["scroll"].(uint64)
		b := apps[j]["keys"].(uint64) + apps[j]["clicks"].(uint64) + apps[j]["scroll"].(uint64)
		return a > b || (a == b && apps[i]["name"].(string) < apps[j]["name"].(string))
	})
	out["apps"] = apps
	return out
}
