package main

import (
	"bytes"
	"image/png"
	"testing"
)

// v2.1.0: the app logo choice. Only the logos embedded in the program are accepted.
func TestValidLogo(t *testing.T) {
	if !validLogo("") {
		t.Fatal("empty (default logo) must be valid")
	}
	if !validLogo("01_Copper_Bronze.png") {
		t.Fatal("an embedded logo must be valid")
	}
	for _, bad := range []string{"../logo.png", "01_Copper_Bronze.png/..", "nope.png", "01_Copper_Bronze.jpg", "list.json", "01 Copper.png"} {
		if validLogo(bad) {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

func TestLogoPNGFallsBackToDefault(t *testing.T) {
	def, _ := uiFiles.ReadFile("ui/logo.png")
	if !bytes.Equal(logoPNG("bad.png"), def) {
		t.Fatal("an unknown logo must give the default logo bytes")
	}
	if len(logoPNG("01_Copper_Bronze.png")) == 0 {
		t.Fatal("an embedded logo must give its bytes")
	}
}

func TestScaleLogoSize(t *testing.T) {
	data, err := scaleLogo(logoPNG("01_Copper_Bronze.png"), 32)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("size %dx%d, want 32x32", b.Dx(), b.Dy())
	}
}
