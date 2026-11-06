package main

import "testing"

func TestShortcutConflict(t *testing.T) {
	cases := []struct {
		mods, vk uint32
		pass     bool
		want     string
	}{
		{kCtrl, 'A', false, "Also Select all"},
		{kCtrl | kShift, '8', false, ""},
		{0, 'V', false, "Blocks typing V everywhere"},
		{0, 'V', true, ""}, // hold keys pass through: no typing problem
		{kShift, 'K', false, "Blocks Shift+K while typing"},
		{0, 0x77, false, ""}, // F8 alone is fine
		{kAlt, 0x73, false, "Also Close window"},
	}
	for _, c := range cases {
		if got := ShortcutConflict(c.mods, c.vk, c.pass); got != c.want {
			t.Errorf("ShortcutConflict(%x, %x, %v) = %q, want %q", c.mods, c.vk, c.pass, got, c.want)
		}
	}
}

func TestFluidAndHeart(t *testing.T) {
	var f Fluid
	UpdateFluid(&f, 12.5, FluidPalette(Hex(0x0a84ff, 1)), 1.4)
	if f.W*f.H*3 != len(f.Pix) {
		t.Fatal("bad fluid size")
	}
	for _, v := range f.Pix {
		if v < 0 || v > 1.01 {
			t.Fatalf("fluid value out of range: %v", v)
		}
	}
	if d := sdHeart(10, 10, 10, 10, 8); d >= 0 {
		t.Errorf("heart center should be inside, got %v", d)
	}
	if d := sdHeart(40, 40, 10, 10, 8); d <= 0 {
		t.Errorf("far point should be outside, got %v", d)
	}
}
