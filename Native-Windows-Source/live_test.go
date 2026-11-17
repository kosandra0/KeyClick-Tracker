package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestLiveRatesDecayToZero: after the 10 s window has passed with no input, the speed is 0.
// This is the rule the "speed stuck at 5 or 2" bug broke on the display side.
func TestLiveRatesDecayToZero(t *testing.T) {
	setupTestStore(t)
	for i := 0; i < 20; i++ {
		record("key", "A", 0) // a letter key: counts as a typed character
	}
	now := time.Now()
	w, c, k := liveRates(now)
	if w <= 0 || k <= 0 {
		t.Fatalf("right after typing: wpm=%v kpm=%v, want above 0", w, k)
	}
	_ = c
	for _, later := range []time.Duration{11 * time.Second, 30 * time.Second, 5 * time.Minute} {
		w, c, k = liveRates(now.Add(later))
		if w != 0 || c != 0 || k != 0 {
			t.Fatalf("%v later: wpm=%v cpm=%v kpm=%v, want all 0", later, w, c, k)
		}
	}
}

func TestLiveShouldSend(t *testing.T) {
	cases := []struct {
		name         string
		w, c, lw, lc float64
		since        time.Duration
		want         bool
	}{
		{"unchanged and recent", 0, 0, 0, 0, 500 * time.Millisecond, false},
		{"unchanged but older than the heartbeat", 0, 0, 0, 0, 2 * time.Second, true},
		{"speed changed", 3, 0, 5, 0, 100 * time.Millisecond, true},
		{"click rate changed", 0, 2, 0, 0, 100 * time.Millisecond, true},
		{"never sent (zero time)", 0, 0, 0, 0, 24 * time.Hour, true},
	}
	for _, tc := range cases {
		if got := liveShouldSend(tc.w, tc.c, tc.lw, tc.lc, tc.since); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestNewSubscriberGetsCurrentSpeedAtOnce: a page that (re)connects must be told the speed
// right away; with nothing typed, that is 0, so it cannot keep an old number.
func TestNewSubscriberGetsCurrentSpeedAtOnce(t *testing.T) {
	setupTestStore(t)
	srv := httptest.NewServer(http.HandlerFunc(sseHandler))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var f struct {
			WPM float64 `json:"wpm"`
			CPM float64 `json:"cpm"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &f); err != nil {
			t.Fatalf("first frame is not JSON: %v (%s)", err, line)
		}
		if f.WPM != 0 || f.CPM != 0 {
			t.Fatalf("idle first frame has wpm=%v cpm=%v, want 0", f.WPM, f.CPM)
		}
		return
	}
	t.Fatal("no data frame arrived on connect")
}

// TestHeartbeatFrameWhenNothingChanges: with a page connected and no input at all, a frame
// still arrives within the heartbeat (plus the 500 ms tick), so a lost frame is corrected.
func TestHeartbeatFrameWhenNothingChanges(t *testing.T) {
	setupTestStore(t)
	ch := make(chan []byte, 8)
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
	startBroadcaster()
	select {
	case b := <-ch:
		var f struct {
			WPM float64 `json:"wpm"`
		}
		if err := json.Unmarshal(b, &f); err != nil || f.WPM != 0 {
			t.Fatalf("heartbeat frame wrong: %s (%v)", b, err)
		}
	case <-time.After(3500 * time.Millisecond):
		t.Fatal("no frame within the heartbeat while idle")
	}
}
