package main

import (
	"strings"
	"testing"
)

// feedKeys runs scripted key transitions through the real handleKey and returns what it queued.
// A key is "+X" for down and "-X" for up (X is a virtual-key code in hex); "E" marks an extended key.
func feedKeys(t *testing.T, steps []keyStep) []string {
	t.Helper()
	for len(hookEvents) > 0 { // drain anything left by an earlier test
		<-hookEvents
	}
	pressedAt = map[uint32]int64{}
	mods = modState{}
	for _, s := range steps {
		handleKey(s.vk, s.scan, s.down, s.ext)
	}
	var out []string
	for len(hookEvents) > 0 {
		e := <-hookEvents
		out = append(out, e.kind+":"+e.id)
	}
	return out
}

type keyStep struct {
	vk, scan  uint32
	down, ext bool
}

func TestHandleKeyShortcutAndModifierEvents(t *testing.T) {
	const ctrl, shift, alt, win, keyC, keyA = 0x11, 0x10, 0x12, 0x5B, 0x43, 0x41
	got := strings.Join(feedKeys(t, []keyStep{
		{ctrl, 0x1D, true, false}, {keyC, 0x2E, true, false}, {keyC, 0x2E, false, false}, {ctrl, 0x1D, false, false}, // Ctrl+C
		{shift, 0x2A, true, false}, {keyA, 0x1E, true, false}, {keyA, 0x1E, false, false}, {shift, 0x2A, false, false}, // Shift+A: not a shortcut
		{keyA, 0x1E, true, false}, {keyA, 0x1E, false, false}, // plain A
		{win, 0x5B, true, true}, {0x45, 0x12, true, false}, {0x45, 0x12, false, false}, {win, 0x5B, false, true}, // Win+E
	}), " ")
	want := "key:ControlLeft modkey: key:C shortcut: key:ShiftLeft key:A key:A key:Windows modkey: key:E shortcut:"
	// the Win key's name comes from keyName; accept whatever it is called but require the same event pattern
	normalized := got
	for _, n := range []string{"key:MetaLeft", "key:OSLeft", "key:VK_5B"} {
		normalized = strings.Replace(normalized, n, "key:Windows", 1)
	}
	if normalized != want {
		t.Fatalf("queued events:\n got  %s\n want %s", got, want)
	}
}

// TestRightControlAndAltAreNotLeft covers the old bug: the extended-key flag was never passed
// to keyName, so right-hand Ctrl and Alt were counted as the left-hand keys.
func TestRightControlAndAltAreNotLeft(t *testing.T) {
	got := feedKeys(t, []keyStep{
		{0x11, 0x1D, true, true}, {0x11, 0x1D, false, true}, // right Ctrl
		{0x12, 0x38, true, true}, {0x12, 0x38, false, true}, // right Alt
		{0x11, 0x1D, true, false}, {0x11, 0x1D, false, false}, // left Ctrl
		{0x12, 0x38, true, false}, {0x12, 0x38, false, false}, // left Alt
	})
	var keys []string
	for _, g := range got {
		if strings.HasPrefix(g, "key:") {
			keys = append(keys, strings.TrimPrefix(g, "key:"))
		}
	}
	if strings.Join(keys, ",") != "ControlRight,AltRight,ControlLeft,AltLeft" {
		t.Fatalf("key names = %v", keys)
	}
}

func TestAutoRepeatIsNotACountedShortcutTwice(t *testing.T) {
	got := feedKeys(t, []keyStep{
		{0x11, 0x1D, true, false},
		{0x43, 0x2E, true, false}, {0x43, 0x2E, true, false}, {0x43, 0x2E, true, false}, // key held: auto-repeat
		{0x43, 0x2E, false, false}, {0x11, 0x1D, false, false},
	})
	n := 0
	for _, g := range got {
		if g == "shortcut:" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("shortcut events = %d, want 1 (auto-repeat must not repeat it): %v", n, got)
	}
}
