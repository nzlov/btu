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

func TestParsePaletteOrderWithBlack(t *testing.T) {
	palette, err := ParsePalette("cmyb")
	if err != nil {
		t.Fatal(err)
	}
	if palette.slot(ColorCyan) != 1 || palette.slot(ColorMagenta) != 2 || palette.slot(ColorYellow) != 3 || palette.slot(ColorBlack) != 4 {
		t.Fatalf("unexpected slots: %v", palette.Slots)
	}
	wantColors := []string{"#0000FF", "#FF0000", "#FFFF00", "#000000"}
	if got := palette.outputColors(); !reflect.DeepEqual(got, wantColors) {
		t.Fatalf("output colors = %v, want %v", got, wantColors)
	}
}

func TestParsePaletteRejectsMovingCMY(t *testing.T) {
	if _, err := ParsePalette("wmmb"); err == nil {
		t.Fatal("expected fixed CMY order error")
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
