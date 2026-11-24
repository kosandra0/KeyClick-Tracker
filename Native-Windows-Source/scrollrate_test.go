//go:build windows

package main

import (
	"testing"
	"time"
)

// v2.6.0: the scroll rate counts scroll events over the last 10 seconds, like the typing speed.
func TestLiveScrollPerMinDecays(t *testing.T) {
	setupTestStore(t)
	for i := 0; i < 12; i++ {
		record("scroll", "WheelDown", 1)
	}
	now := time.Now()
	if got := liveScrollPerMin(now); got <= 0 {
		t.Fatalf("right after scrolling: spm=%v, want above 0", got)
	}
	for _, later := range []time.Duration{11 * time.Second, 5 * time.Minute} {
		if got := liveScrollPerMin(now.Add(later)); got != 0 {
			t.Fatalf("%v later: spm=%v, want 0", later, got)
		}
	}
}

// Typing alone does not count as scrolling.
func TestLiveScrollPerMinIgnoresKeys(t *testing.T) {
	setupTestStore(t)
	for i := 0; i < 20; i++ {
		record("key", "A", 0)
	}
	if got := liveScrollPerMin(time.Now()); got != 0 {
		t.Fatalf("keys only: spm=%v, want 0", got)
	}
}

// The scroll unit defaults to scrolls per second and any unknown value falls back to it.
func TestScrollRateUnitNormalize(t *testing.T) {
	cases := map[string]string{"": "sps", "bogus": "sps", "spm": "spm", "sph": "sph", "sps": "sps"}
	for in, want := range cases {
		s := &Store{Settings: &Settings{ScrollRateUnit: in}}
		s.normalize()
		if s.Settings.ScrollRateUnit != want {
			t.Errorf("ScrollRateUnit %q normalized to %q, want %q", in, s.Settings.ScrollRateUnit, want)
		}
	}
}

// v2.5.1: a flat box (56 px high) must not be accepted any more; it is reset to the usual height.
func TestOverlayMinimumSizeNormalize(t *testing.T) {
	s := &Store{Settings: &Settings{OverlayW: 512, OverlayH: 56, OverlaySizes: map[string][2]int{"detailed": {150, 56}, "sparkline": {300, 100}}}}
	s.normalize()
	if s.Settings.OverlayH != 92 {
		t.Fatalf("flat height kept: OverlayH=%d, want 92", s.Settings.OverlayH)
	}
	if s.Settings.OverlayW != 512 {
		t.Fatalf("width changed: OverlayW=%d, want 512", s.Settings.OverlayW)
	}
	if _, ok := s.Settings.OverlaySizes["detailed"]; ok {
		t.Fatal("a saved size below the minimum was kept")
	}
	if _, ok := s.Settings.OverlaySizes["sparkline"]; !ok {
		t.Fatal("a valid saved size was removed")
	}
}
