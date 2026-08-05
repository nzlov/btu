package threemf

import (
	"reflect"
	"testing"
)

func TestDefaultPaletteIsCMYG(t *testing.T) {
	want := [4]ColorRole{ColorCyan, ColorMagenta, ColorYellow, ColorGray}
	if got := DefaultPalette().Slots; got != want {
		t.Fatalf("default palette = %v, want %v", got, want)
	}
}

func TestParsePaletteCustomOrderWithBlack(t *testing.T) {
	palette, err := ParsePalette("bmcy")
	if err != nil {
		t.Fatal(err)
	}
	if palette.Slot(ColorBlack) != 1 || palette.Slot(ColorMagenta) != 2 || palette.Slot(ColorCyan) != 3 || palette.Slot(ColorYellow) != 4 {
		t.Fatalf("unexpected slots: %v", palette.Slots)
	}
	wantColors := []string{"#000000", "#FF0000", "#0000FF", "#FFFF00"}
	if got := palette.outputColors(); !reflect.DeepEqual(got, wantColors) {
		t.Fatalf("output colors = %v, want %v", got, wantColors)
	}
}

func TestParsePaletteRejectsInvalidColorSet(t *testing.T) {
	for _, order := range []string{"cmy", "cmyc", "cmgw", "cmyx"} {
		if _, err := ParsePalette(order); err == nil {
			t.Fatalf("ParsePalette(%q) unexpectedly succeeded", order)
		}
	}
}

func TestDecomposeGreenAsYellowAndCyan(t *testing.T) {
	components := decomposeCMY([3]int{0, 255, 0}, DefaultPalette())
	want := []colorComponent{
		{role: ColorYellow, weight: 1},
		{role: ColorCyan, weight: 1},
	}
	if !reflect.DeepEqual(components, want) {
		t.Fatalf("green components = %+v, want %+v", components, want)
	}
}

func TestDecomposeBlackUsesBlackSlot(t *testing.T) {
	palette, err := ParsePalette("cmyb")
	if err != nil {
		t.Fatal(err)
	}
	components := decomposeCMY([3]int{}, palette)
	want := []colorComponent{{role: ColorBlack, weight: 1}}
	if !reflect.DeepEqual(components, want) {
		t.Fatalf("black components = %+v, want %+v", components, want)
	}
}

func TestPurpleIsNotClassifiedAsGray(t *testing.T) {
	role, base := baseColorRole([3]int{0x5E, 0x43, 0xB7})
	if base || role != ColorCyan {
		t.Fatalf("purple classification = %s, base = %v", role, base)
	}
}

func TestDarkMagentaIsNotClassifiedAsGray(t *testing.T) {
	role, base := baseColorRole([3]int{0xBB, 0x3D, 0x43})
	if !base || role != ColorMagenta {
		t.Fatalf("dark magenta classification = %s, base = %v", role, base)
	}
}

func TestGrayUsesDynamicNeutralSlot(t *testing.T) {
	parts, ok := colorParts(roleRGB(ColorGray), DefaultPalette())
	want := []colorComponent{{role: ColorGray, weight: 1}}
	if !ok || !reflect.DeepEqual(parts, want) {
		t.Fatalf("gray parts = %+v, ok = %v, want %+v", parts, ok, want)
	}
}
