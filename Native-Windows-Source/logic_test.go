package main

import (
	"testing"
	"time"
)

func at(day int, h, m int) time.Time {
	return time.Date(2026, 11, day, h, m, 0, 0, time.Local)
}

func TestIdleSplit(t *testing.T) {
	five := 5 * time.Minute
	cases := []struct {
		name      string
		prev, now time.Time
		th        time.Duration
		want      map[string]time.Duration
	}{
		{"short gap is not idle (C1.1)", at(16, 10, 0), at(16, 10, 2), five, map[string]time.Duration{}},
		{"20 min gap gives 15 min idle (C1.2)", at(16, 10, 0), at(16, 10, 20), five,
			map[string]time.Duration{"2026-11-16": 15 * time.Minute}},
		{"gap across midnight is split (C1.3)", at(15, 23, 50), at(16, 0, 20), five,
			map[string]time.Duration{"2026-11-15": 5 * time.Minute, "2026-11-16": 20 * time.Minute}},
		{"clock moved back gives nothing (C1.5)", at(16, 10, 30), at(16, 10, 0), five, map[string]time.Duration{}},
		{"zero anchor gives nothing", time.Time{}, at(16, 10, 0), five, map[string]time.Duration{}},
		{"zero threshold gives nothing", at(16, 10, 0), at(16, 11, 0), 0, map[string]time.Duration{}},
	}
	for _, c := range cases {
		got := idleSplit(c.prev, c.now, c.th)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: day %s got %v want %v", c.name, k, got[k], v)
			}
		}
	}
}

func TestIdleSplitCap(t *testing.T) {
	// C1.6: a 14 hour gap counts at most 12 hours of idle.
	got := idleSplit(at(16, 0, 0), at(16, 0, 0).Add(14*time.Hour), 5*time.Minute)
	var total time.Duration
	for _, v := range got {
		total += v
	}
	if total != maxIdleSpan {
		t.Errorf("cap: got %v want %v", total, maxIdleSpan)
	}
}

func TestAppNameFromPath(t *testing.T) {
	cases := map[string]string{
		`C:\Windows\System32\notepad.exe`: "notepad.exe",
		`C:\tools\App`:                    "app",
		`  C:\Apps\Chrome.EXE  `:          "chrome.exe",
		``:                                "(protected)",
		`   `:                             "(protected)",
	}
	for in, want := range cases {
		if got := appNameFromPath(in); got != want {
			t.Errorf("appNameFromPath(%q) = %q want %q", in, got, want)
		}
	}
}

func TestParseCombo(t *testing.T) {
	type out struct {
		mods, vk uint32
		ok       bool
	}
	cases := map[string]out{
		"ctrl+alt+k":         {modCtrl | modAlt, 0x4B, true},
		"ctrl+alt+shift+f5":  {modCtrl | modAlt | modShift, 0x74, true},
		"win+1":              {modWin, 0x31, true},
		"control+alt+l":      {modCtrl | modAlt, 0x4C, true},
		"k":                  {0, 0, false},
		"ctrl+alt+f13":       {0, 0, false},
		"ctrl+alt+":          {0, 0, false},
		"hyper+k":            {0, 0, false},
		"ctrl+alt+escape":    {0, 0, false},
		"ctrl+alt+shift+f12": {modCtrl | modAlt | modShift, 0x7B, true},
	}
	for in, want := range cases {
		mods, vk, err := parseCombo(in)
		if want.ok {
			if err != nil || mods != want.mods || vk != want.vk {
				t.Errorf("parseCombo(%q) = %x,%x,%v want %x,%x", in, mods, vk, err, want.mods, want.vk)
			}
		} else if err == nil {
			t.Errorf("parseCombo(%q) accepted, want error", in)
		}
	}
}

func TestWorstLevel(t *testing.T) {
	if worstLevel() != levelOK || worstLevel(levelOK, levelOK) != levelOK {
		t.Error("all ok should be ok")
	}
	if worstLevel(levelOK, levelAmber) != levelAmber {
		t.Error("amber not picked")
	}
	if worstLevel(levelAmber, levelRed, levelOK) != levelRed {
		t.Error("red not picked over amber")
	}
}

func TestSizeLevels(t *testing.T) {
	mb := int64(1 << 20)
	if fileSizeLevel(10*1024) != levelOK || fileSizeLevel(60*mb) != levelAmber || fileSizeLevel(600*mb) != levelRed {
		t.Error("file size levels wrong")
	}
	if profileLevel(100*mb) != levelOK || profileLevel(200*mb) != levelAmber {
		t.Error("profile levels wrong")
	}
	gb := int64(1 << 30)
	if diskLevel(48*gb) != levelOK || diskLevel(1*gb) != levelAmber || diskLevel(100*mb) != levelRed {
		t.Error("disk levels wrong")
	}
}

func TestIdleMinutesOrDefault(t *testing.T) {
	if idleMinutesOrDefault(0) != 5 || idleMinutesOrDefault(-3) != 1 || idleMinutesOrDefault(500) != 60 || idleMinutesOrDefault(10) != 10 {
		t.Error("idle minutes clamp wrong")
	}
}
