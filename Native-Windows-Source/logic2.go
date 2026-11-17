package main

// Pure logic for the v2.0.0 features (no Windows calls, no global state), so each rule can be
// tested with scripted input: shortcut counting (F4), break reminders (F5), goal history (F7),
// strain signals (F6), CSV rows (F11), quiet hours (A2) and backup pruning (F9).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ------------------------------------------------------------------ F4 shortcuts

// modState tracks which modifier keys are held. It runs on the input thread only.
type modState struct {
	ctrl, altL, altR, win, shift bool
	last                         time.Time
}

// apply updates the state for one key transition and reports whether the key is a modifier.
// ext is the "extended key" flag (right-hand Ctrl/Alt). A held key sends auto-repeat key-downs,
// so a state not refreshed for 30 s is treated as stale (a missed key-up, for example after Win+L).
func (m *modState) apply(vk uint32, ext, down bool, now time.Time) bool {
	if !m.last.IsZero() && now.Sub(m.last) > 30*time.Second {
		*m = modState{}
	}
	m.last = now
	switch vk {
	case 0x10, 0xA0, 0xA1:
		m.shift = down
	case 0x11, 0xA2, 0xA3:
		m.ctrl = down
	case 0x12, 0xA4, 0xA5:
		if ext || vk == 0xA5 {
			m.altR = down
		} else {
			m.altL = down
		}
	case 0x5B, 0x5C:
		m.win = down
	default:
		return false
	}
	return true
}

// countedModifier reports whether a modifier press is counted in ModKeys (Ctrl, Alt, Win; not Shift).
func countedModifier(vk uint32) bool {
	switch vk {
	case 0x11, 0xA2, 0xA3, 0x12, 0xA4, 0xA5, 0x5B, 0x5C:
		return true
	}
	return false
}

// isShortcut reports whether a non-modifier key pressed now is a shortcut: Ctrl, left Alt or
// Win held. Shift alone is not. On layouts where AltGr sends Ctrl+Alt, right Alt is ignored
// (known limit, documented in the plan).
func (m *modState) isShortcut() bool { return (m.ctrl && !m.altR) || m.altL || m.win }

// ------------------------------------------------------------------ F9 pruning

// namesToPrune returns the oldest names to delete so that only keep remain (names sort by date).
func namesToPrune(names []string, keep int) []string {
	if keep < 0 {
		keep = 0
	}
	s := append([]string(nil), names...)
	sort.Strings(s)
	if len(s) <= keep {
		return nil
	}
	return s[:len(s)-keep]
}

// ------------------------------------------------------------------ A2 quiet hours

// parseHM turns "23:30" into minutes after midnight.
func parseHM(s string) (int, bool) {
	p := strings.Split(strings.TrimSpace(s), ":")
	if len(p) != 2 {
		return 0, false
	}
	h, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	if e1 != nil || e2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// inQuietHours reports whether now falls in [from, to). A range that crosses midnight works
// (23:00 to 07:00). Equal or invalid times mean "never quiet".
func inQuietHours(now time.Time, from, to string) bool {
	f, ok1 := parseHM(from)
	t, ok2 := parseHM(to)
	if !ok1 || !ok2 || f == t {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	if f < t {
		return m >= f && m < t
	}
	return m >= f || m < t
}

// ------------------------------------------------------------------ F5 reminders

type reminderCfg struct {
	Mode     string // "" or "simple": one reminder after BreakMin; "cycle": WorkMin of work, RestMin of rest
	BreakMin int
	WorkMin  int
	RestMin  int
	IdleMin  int
}

type reminderState struct {
	workStart, firedAt, snoozeUntil time.Time
}

// step is called about every 20 s. It returns true when a reminder should be shown now.
// "Continuous" means no gap of IdleMin minutes. A reminder never fires during idle time,
// and never while a full-screen program is in front. A rest is recognised from real idle
// time (RestMin in cycle mode), then the cycle starts again.
func (s *reminderState) step(now, lastEvent time.Time, c reminderCfg, fullscreen bool) (bool, string) {
	if lastEvent.IsZero() {
		*s = reminderState{}
		return false, ""
	}
	idle := time.Duration(idleMinutesOrDefault(c.IdleMin)) * time.Minute
	rest := idle
	work := c.BreakMin
	if c.Mode == "cycle" {
		if c.RestMin > 0 {
			rest = time.Duration(c.RestMin) * time.Minute
		}
		work = c.WorkMin
	}
	gap := now.Sub(lastEvent)
	if gap >= rest {
		*s = reminderState{snoozeUntil: s.snoozeUntil}
		return false, ""
	}
	if gap >= idle {
		return false, "" // paused, not yet a rest: nothing to remind during idle time
	}
	if s.workStart.IsZero() {
		s.workStart = lastEvent
	}
	if work <= 0 || now.Before(s.snoozeUntil) {
		return false, ""
	}
	since := now.Sub(s.workStart)
	if !s.firedAt.IsZero() {
		since = now.Sub(s.firedAt)
	}
	if since < time.Duration(work)*time.Minute {
		return false, ""
	}
	if fullscreen {
		return false, "" // do not disturb; try again at the next step
	}
	s.firedAt = now
	if c.Mode == "cycle" {
		return true, fmt.Sprintf("You have worked for about %d minutes. Take a rest of %d minutes: stand up, stretch, look far away.", work, max(c.RestMin, 1))
	}
	return true, fmt.Sprintf("You have been active for about %d minutes. Stretch, breathe, and rest your eyes.", work)
}

// snooze delays the next reminder by the given minutes.
func (s *reminderState) snooze(now time.Time, minutes int) {
	s.snoozeUntil = now.Add(time.Duration(minutes) * time.Minute)
	s.firedAt = time.Time{}
}

// canSnooze is true shortly after a reminder was shown (the tray menu shows the snooze item).
func (s *reminderState) canSnooze(now time.Time) bool {
	return !s.firedAt.IsZero() && now.Sub(s.firedAt) < 15*time.Minute
}

// rectCoversMonitor reports whether a window rectangle covers a whole monitor (full screen).
func rectCoversMonitor(win, mon rect) bool {
	return win.Left <= mon.Left && win.Top <= mon.Top && win.Right >= mon.Right && win.Bottom >= mon.Bottom
}

// ------------------------------------------------------------------ F7 goals

// GoalDay is the goals in force on one day and whether each was met. Past days are never
// rewritten when a goal changes.
type GoalDay struct {
	GK uint64 `json:"gk,omitempty"`
	GC uint64 `json:"gc,omitempty"`
	GW uint64 `json:"gw,omitempty"`
	MK bool   `json:"mk,omitempty"`
	MC bool   `json:"mc,omitempty"`
	MW bool   `json:"mw,omitempty"`
}

// evaluate sets the met flags from the day's totals. avgWPM is the day's average typing speed.
func (g *GoalDay) evaluate(keys, clicks uint64, avgWPM float64) {
	g.MK = g.GK > 0 && keys >= g.GK
	g.MC = g.GC > 0 && clicks >= g.GC
	g.MW = g.GW > 0 && avgWPM >= float64(g.GW)
}

// state is "met" (every active goal met), "partly", "missed" (active goals, none met) or "none".
func (g *GoalDay) state() string {
	active, met := 0, 0
	for _, p := range [][2]bool{{g.GK > 0, g.MK}, {g.GC > 0, g.MC}, {g.GW > 0, g.MW}} {
		if p[0] {
			active++
			if p[1] {
				met++
			}
		}
	}
	switch {
	case active == 0:
		return "none"
	case met == active:
		return "met"
	case met > 0:
		return "partly"
	}
	return "missed"
}

// goalStreak counts consecutive days with every active goal met, ending today, or ending
// yesterday when today is not met yet (the day is still in progress).
func goalStreak(log map[string]*GoalDay, today time.Time) int {
	day := midnight(today)
	if g := log[day.Format("2006-01-02")]; g == nil || g.state() != "met" {
		day = day.AddDate(0, 0, -1)
	}
	n := 0
	for {
		g := log[day.Format("2006-01-02")]
		if g == nil || g.state() != "met" {
			return n
		}
		n++
		day = day.AddDate(0, 0, -1)
	}
}

// ------------------------------------------------------------------ F6 strain signals

type burstSlot struct {
	idx int64
	n   uint64
}

// burstTracker keeps the activity of the last 10 minutes (one slot per minute).
type burstTracker struct{ slots [10]burstSlot }

// add records n events in the given minute (unix minutes) and returns the 10-minute total.
func (b *burstTracker) add(minute int64, n uint64) uint64 {
	s := &b.slots[((minute%10)+10)%10]
	if s.idx != minute {
		*s = burstSlot{idx: minute}
	}
	s.n += n
	var sum uint64
	for _, x := range b.slots {
		if x.idx > minute-10 && x.idx <= minute {
			sum += x.n
		}
	}
	return sum
}

// percentile returns the p-th percentile (0..100) of the values, nearest-rank.
func percentile(v []uint64, p int) uint64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]uint64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	rank := (p*len(s) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return s[rank-1]
}

// strainFlag: today's busiest 10 minutes are unusually high against the person's own history.
// It needs at least 14 earlier days and an absolute floor, so a quiet user is never flagged.
func strainFlag(today uint64, prior []uint64) (bool, uint64) {
	if len(prior) < 14 || today < 300 {
		return false, 0
	}
	p90 := percentile(prior, 90)
	return today > p90, p90
}

// ------------------------------------------------------------------ F11 CSV / F3 helpers

// csvDayRows returns the header and one row per day in [from, to] (YYYY-MM-DD), oldest first.
func csvDayRows(days map[string]*DayRoll, from, to string) [][]string {
	rows := [][]string{{"date", "keys", "clicks", "scrolls", "words", "active_min", "idle_min", "shortcuts"}}
	var keys []string
	for k := range days {
		if (from == "" || k >= from) && (to == "" || k <= to) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		d := days[k]
		rows = append(rows, []string{k, u(d.Keys), u(d.Clicks), u(d.Scrolls), u(d.Words), strconv.Itoa(int(d.ActiveMin)), u(d.IdleSec / 60), u(d.Shortcuts)})
	}
	return rows
}

func u(v uint64) string { return strconv.FormatUint(v, 10) }

// percentChange words the change from the previous period; "no earlier data" when there is none.
func percentChange(cur, prev uint64) string {
	if prev == 0 {
		return "no earlier data"
	}
	d := (float64(cur) - float64(prev)) * 100 / float64(prev)
	if d >= 0 {
		return fmt.Sprintf("+%.0f%% compared with the period before", d)
	}
	return fmt.Sprintf("%.0f%% compared with the period before", d)
}
