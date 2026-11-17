package main

// Pure helper functions for v2.0.0. They have no Win32 or store dependencies so
// they can be tested with go test (see logic_test.go).

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// maxIdleSpan caps the idle time recorded for one pause. A pause longer than this
// (for example the PC was off overnight without a resume message) counts as this much.
const maxIdleSpan = 12 * time.Hour

// idleSplit returns the idle time of the gap between two recorded events, per local
// day key (yyyy-mm-dd). The first `threshold` of the gap is normal thinking time and
// is not idle. A pause is split at local midnight so each day gets its own share.
// prev is the time of the previous recorded event; a zero prev means no anchor.
func idleSplit(prev, now time.Time, threshold time.Duration) map[string]time.Duration {
	out := map[string]time.Duration{}
	if prev.IsZero() || !now.After(prev) || threshold <= 0 {
		return out
	}
	from := prev.Add(threshold)
	to := now
	if limit := from.Add(maxIdleSpan); to.After(limit) {
		to = limit
	}
	if !to.After(from) {
		return out
	}
	from = from.In(time.Local)
	to = to.In(time.Local)
	for from.Before(to) {
		y, m, d := from.Date()
		nextMidnight := time.Date(y, m, d+1, 0, 0, 0, 0, time.Local)
		seg := to
		if nextMidnight.Before(to) {
			seg = nextMidnight
		}
		out[from.Format("2006-01-02")] += seg.Sub(from)
		from = seg
	}
	return out
}

// appNameFromPath returns the executable file name in lower case. An empty path
// (a process that could not be opened) is reported as "(protected)". Only the
// file name is kept; folders and window titles never reach the store.
func appNameFromPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "(protected)"
	}
	return strings.ToLower(filepath.Base(p))
}

// Modifier bits as used by RegisterHotKey.
const (
	modAlt   uint32 = 0x0001
	modCtrl  uint32 = 0x0002
	modShift uint32 = 0x0004
	modWin   uint32 = 0x0008
)

// parseCombo turns text such as "ctrl+alt+k" or "ctrl+alt+shift+f5" into RegisterHotKey
// modifiers and a virtual-key code. At least one modifier is required, so a plain key
// can never be taken system-wide.
func parseCombo(s string) (mods uint32, vk uint32, err error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("combination needs at least one modifier and one key")
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	for _, m := range parts[:len(parts)-1] {
		switch strings.TrimSpace(m) {
		case "ctrl", "control":
			mods |= modCtrl
		case "alt":
			mods |= modAlt
		case "shift":
			mods |= modShift
		case "win":
			mods |= modWin
		default:
			return 0, 0, fmt.Errorf("unknown modifier %q", m)
		}
	}
	if mods == 0 {
		return 0, 0, fmt.Errorf("at least one modifier is required")
	}
	switch {
	case len(key) == 1 && key[0] >= 'a' && key[0] <= 'z':
		vk = uint32(key[0] - 'a' + 'A') // VK_A..VK_Z equal the upper-case ASCII letters
	case len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		vk = uint32(key[0]) // VK_0..VK_9 equal the ASCII digits
	case strings.HasPrefix(key, "f"):
		n, e := strconv.Atoi(key[1:])
		if e != nil || n < 1 || n > 12 {
			return 0, 0, fmt.Errorf("unknown key %q", key)
		}
		vk = uint32(0x70 + n - 1) // VK_F1 = 0x70
	default:
		return 0, 0, fmt.Errorf("unknown key %q", key)
	}
	return mods, vk, nil
}

// Health levels, worst last.
const (
	levelOK    = "ok"
	levelAmber = "amber"
	levelRed   = "red"
)

// worstLevel returns the most severe level in the list.
func worstLevel(levels ...string) string {
	out := levelOK
	for _, l := range levels {
		switch {
		case l == levelRed:
			return levelRed
		case l == levelAmber:
			out = levelAmber
		}
	}
	return out
}

// fileSizeLevel rates the statistics file size: amber above 50 MB, red above 500 MB.
func fileSizeLevel(bytes int64) string {
	switch {
	case bytes > 500<<20:
		return levelRed
	case bytes > 50<<20:
		return levelAmber
	}
	return levelOK
}

// profileLevel rates the browser-engine cache size: amber above 150 MB.
func profileLevel(bytes int64) string {
	if bytes > 150<<20 {
		return levelAmber
	}
	return levelOK
}

// diskLevel rates free space on the data drive: red below 500 MB, amber below 2 GB.
func diskLevel(freeBytes int64) string {
	switch {
	case freeBytes < 500<<20:
		return levelRed
	case freeBytes < 2<<30:
		return levelAmber
	}
	return levelOK
}

// clampInt limits v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// idleMinutesOrDefault returns the configured idle threshold, or the default when unset.
func idleMinutesOrDefault(m int) int {
	if m == 0 {
		return 5
	}
	return clampInt(m, 1, 60)
}
