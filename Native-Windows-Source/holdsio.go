package main

// v2.4.0: import and export of the duration data, in all three formats (json, txt, md).
//
// The export file already carries the whole data block (the new fields travel with it). What was
// missing was the import side: the merge only kept the bigger day and the history records were
// ignored. The rules here keep the import idempotent (importing a file twice changes nothing),
// and never lose hold times, scroll sessions, per-program detail or history that only exists
// locally or only in the file:
//   - a counter takes the larger of the local and the imported value;
//   - for a measured hold (a key or a button) the entry with more measured holds wins as a whole,
//     so its count, total time and longest stay consistent with each other;
//   - history records are the union of both lists (same key, button or session, start, length,
//     program), newest recentLimit kept.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func maxU(dst *uint64, v uint64) bool {
	if v > *dst {
		*dst = v
		return true
	}
	return false
}

func maxInt64(dst *int64, v int64) bool {
	if v > *dst {
		*dst = v
		return true
	}
	return false
}

// mergeHold merges src into *dst per key or button and reports whether anything changed.
func mergeHold(dst *map[string]*HoldStat, src map[string]*HoldStat) bool {
	changed := false
	for id, s := range src {
		if s == nil {
			continue
		}
		if *dst == nil {
			*dst = map[string]*HoldStat{}
		}
		d := (*dst)[id]
		if d == nil {
			c := *s
			(*dst)[id] = &c
			changed = true
			continue
		}
		if s.N > d.N || (s.N == d.N && s.Ms > d.Ms) {
			d.N, d.Ms = s.N, s.Ms
			changed = true
		}
		if maxU(&d.Max, s.Max) {
			changed = true
		}
	}
	return changed
}

func mergeCountMap(dst *map[string]uint64, src map[string]uint64) bool {
	if len(src) == 0 {
		return false
	}
	if *dst == nil {
		*dst = map[string]uint64{}
	}
	return maxInto(*dst, src)
}

func mergeScroll(dst **ScrollAgg, s *ScrollAgg) bool {
	if s == nil {
		return false
	}
	if *dst == nil {
		c := *s
		c.Dir = map[string]uint64{}
		for k, v := range s.Dir {
			c.Dir[k] = v
		}
		*dst = &c
		return true
	}
	d, changed := *dst, false
	if s.Sessions > d.Sessions || (s.Sessions == d.Sessions && s.SpanMs > d.SpanMs) {
		d.Sessions, d.Events, d.SpanMs, d.Single = s.Sessions, s.Events, s.SpanMs, s.Single
		changed = true
	}
	if maxU(&d.MaxMs, s.MaxMs) {
		changed = true
	}
	if mergeCountMap(&d.Dir, s.Dir) {
		changed = true
	}
	return changed
}

func mergeAppCount(d, s *AppCount) bool {
	changed := false
	for _, p := range []struct {
		dst *uint64
		v   uint64
	}{{&d.Keys, s.Keys}, {&d.Clicks, s.Clicks}, {&d.Scroll, s.Scroll}, {&d.Secs, s.Secs}, {&d.Sess, s.Sess}, {&d.SessMs, s.SessMs}} {
		if maxU(p.dst, p.v) {
			changed = true
		}
	}
	if maxInt64(&d.Last, s.Last) {
		changed = true
	}
	if mergeCountMap(&d.KeyIDs, s.KeyIDs) {
		changed = true
	}
	if mergeCountMap(&d.MsIDs, s.MsIDs) {
		changed = true
	}
	if mergeCountMap(&d.ScDir, s.ScDir) {
		changed = true
	}
	if mergeHold(&d.KeyHold, s.KeyHold) {
		changed = true
	}
	if mergeHold(&d.MsHold, s.MsHold) {
		changed = true
	}
	return changed
}

// mergeDayExtras merges everything on a day that is not part of the basic counters: measured
// holds, scroll sessions, per-program detail, idle time, shortcuts, layouts. It reports a change.
func mergeDayExtras(d, s *DayRoll) bool {
	changed := false
	if mergeHold(&d.KeyHold, s.KeyHold) {
		changed = true
	}
	if mergeHold(&d.MsHold, s.MsHold) {
		changed = true
	}
	if mergeScroll(&d.Scr, s.Scr) {
		changed = true
	}
	for name, sa := range s.Apps {
		if sa == nil {
			continue
		}
		if d.Apps == nil {
			d.Apps = map[string]*AppCount{}
		}
		da := d.Apps[name]
		if da == nil {
			c := AppCount{}
			d.Apps[name] = &c
			da = &c
			changed = true
		}
		if mergeAppCount(da, sa) {
			changed = true
		}
	}
	for _, p := range []struct {
		dst *uint64
		v   uint64
	}{{&d.IdleSec, s.IdleSec}, {&d.Shortcuts, s.Shortcuts}, {&d.ModKeys, s.ModKeys}, {&d.Peak10, s.Peak10}} {
		if maxU(p.dst, p.v) {
			changed = true
		}
	}
	if mergeCountMap(&d.Layouts, s.Layouts) {
		changed = true
	}
	return changed
}

func cloneDay(d *DayRoll) *DayRoll {
	var c DayRoll
	b, _ := json.Marshal(d)
	_ = json.Unmarshal(b, &c)
	return &c
}

// mergeDay returns the merged day and whether it differs from the local one. The basic counters
// follow the long-standing rule (the day with the bigger total wins); the extras are merged.
func mergeDay(ex, in *DayRoll) (*DayRoll, bool) {
	if ex == nil {
		return cloneDay(in), true
	}
	if dayTotal(in) > dayTotal(ex) {
		m := cloneDay(in)
		mergeDayExtras(m, ex)
		return m, true
	}
	return ex, mergeDayExtras(ex, in)
}

// dayWouldChange is the read-only twin of mergeDay, used by the import preview.
func dayWouldChange(ex, in *DayRoll) bool {
	if ex == nil || dayTotal(in) > dayTotal(ex) {
		return true
	}
	return mergeDayExtras(cloneDay(ex), in)
}

func intervalKey(iv Interval) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s|%d|%s", iv.K, iv.ID, iv.St, iv.Ms, iv.App, iv.N, iv.Sts)
}

// mergeRecent returns the union of two history lists, oldest first, newest recentLimit kept.
func mergeRecent(local, in []Interval) ([]Interval, bool) {
	seen := make(map[string]bool, len(local)+len(in))
	out := make([]Interval, 0, len(local)+len(in))
	for _, iv := range local {
		k := intervalKey(iv)
		if !seen[k] {
			seen[k] = true
			out = append(out, iv)
		}
	}
	changed := false
	for _, iv := range in {
		k := intervalKey(iv)
		if !seen[k] {
			seen[k] = true
			out = append(out, iv)
			changed = true
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].St < out[j].St })
	if len(out) > recentLimit {
		out = out[len(out)-recentLimit:]
	}
	return out, changed
}

// ------------------------------------------------------------------ report sections (txt and md)

func msText(v uint64) string {
	switch {
	case v < 1000:
		return fmt.Sprintf("%d ms", v)
	case v < 60000:
		return fmt.Sprintf("%.2f s", float64(v)/1000)
	case v < 3600000:
		return fmt.Sprintf("%d m %02d s", v/60000, v%60000/1000)
	}
	return fmt.Sprintf("%d h %02d m", v/3600000, v%3600000/60000)
}

// reportDuration writes the human readable "Input duration" and "Applications" sections for the
// days of data inside [start, end). The exact numbers are in the restorable data block.
func reportDuration(data *Store, start, end time.Time, md bool) string {
	keys, btns := map[string]*HoldStat{}, map[string]*HoldStat{}
	var sc ScrollAgg
	sc.Dir = map[string]uint64{}
	apps := map[string]*AppCount{}
	for k, d := range data.Days {
		t, err := time.ParseInLocation("2006-01-02", k, time.Local)
		if err != nil || t.Before(start) || !t.Before(end) {
			continue
		}
		for id, h := range d.KeyHold {
			t := keys[id]
			if t == nil {
				t = &HoldStat{}
				keys[id] = t
			}
			t.N += h.N
			t.Ms += h.Ms
			if h.Max > t.Max {
				t.Max = h.Max
			}
		}
		for id, h := range d.MsHold {
			t := btns[id]
			if t == nil {
				t = &HoldStat{}
				btns[id] = t
			}
			t.N += h.N
			t.Ms += h.Ms
			if h.Max > t.Max {
				t.Max = h.Max
			}
		}
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
		for name, a := range d.Apps {
			t := apps[name]
			if t == nil {
				t = &AppCount{KeyHold: map[string]*HoldStat{}, MsHold: map[string]*HoldStat{}}
				apps[name] = t
			}
			t.Keys += a.Keys
			t.Clicks += a.Clicks
			t.Scroll += a.Scroll
			t.Sess += a.Sess
			t.SessMs += a.SessMs
			for id, h := range a.KeyHold {
				x := t.KeyHold[id]
				if x == nil {
					x = &HoldStat{}
					t.KeyHold[id] = x
				}
				x.N += h.N
				x.Ms += h.Ms
			}
			for id, h := range a.MsHold {
				x := t.MsHold[id]
				if x == nil {
					x = &HoldStat{}
					t.MsHold[id] = x
				}
				x.N += h.N
				x.Ms += h.Ms
			}
		}
	}
	sum := func(m map[string]*HoldStat) (ms, n, mx uint64) {
		for _, h := range m {
			ms += h.Ms
			n += h.N
			if h.Max > mx {
				mx = h.Max
			}
		}
		return
	}
	var b strings.Builder
	head := func(t string) {
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
	kms, kn, kmx := sum(keys)
	bms, bn, bmx := sum(btns)
	head("Input duration")
	line("Key-held time", msText(kms)+" over "+fn(kn)+" measured presses (longest "+msText(kmx)+")")
	line("Mouse-button-held time", msText(bms)+" over "+fn(bn)+" measured clicks (longest "+msText(bmx)+")")
	line("Scroll-active time (estimated)", msText(sc.SpanMs)+" over "+fn(sc.Sessions)+" scroll sessions, "+fn(sc.Single)+" single wheel clicks (longest session "+msText(sc.MaxMs)+")")
	line("Note", "totals add up every hold, so they can be longer than the time that passed; scroll time is estimated from the gaps between wheel events; durations are recorded from version 2.4.0 on")
	type kv struct {
		id string
		h  *HoldStat
	}
	rank := func(m map[string]*HoldStat) []kv {
		out := make([]kv, 0, len(m))
		for id, h := range m {
			out = append(out, kv{id, h})
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].h.Ms > out[j].h.Ms || (out[i].h.Ms == out[j].h.Ms && out[i].id < out[j].id)
		})
		return out
	}
	if len(keys) > 0 {
		head("Held keys")
		for i, e := range rank(keys) {
			if i >= 15 {
				break
			}
			line(e.id, msText(e.h.Ms)+" held over "+fn(e.h.N)+" presses (average "+msText(e.h.Ms/max64(e.h.N, 1))+", longest "+msText(e.h.Max)+")")
		}
	}
	if len(btns) > 0 {
		head("Mouse buttons held")
		for _, e := range rank(btns) {
			line(e.id, msText(e.h.Ms)+" held over "+fn(e.h.N)+" clicks (average "+msText(e.h.Ms/max64(e.h.N, 1))+", longest "+msText(e.h.Max)+")")
		}
	}
	if len(apps) > 0 {
		head("Applications")
		names := make([]string, 0, len(apps))
		for n := range apps {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool {
			a, c := apps[names[i]], apps[names[j]]
			ta, tc := a.Keys+a.Clicks+a.Scroll, c.Keys+c.Clicks+c.Scroll
			return ta > tc || (ta == tc && names[i] < names[j])
		})
		for _, n := range names {
			a := apps[n]
			akm, _, _ := sum(a.KeyHold)
			abm, _, _ := sum(a.MsHold)
			line(n, fn(a.Keys)+" keys (held "+msText(akm)+"), "+fn(a.Clicks)+" clicks (held "+msText(abm)+"), "+fn(a.Scroll)+" scrolls ("+msText(a.SessMs)+" estimated)")
		}
	}
	return b.String()
}

func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
