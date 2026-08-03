package threemf

import (
	"reflect"
	"testing"
)

func TestDefaultPaletteIsCMYK(t *testing.T) {
	want := [4]ColorRole{ColorBlue, ColorRed, ColorYellow, ColorBlack}
	if got := DefaultPalette().Slots; got != want {
		t.Fatalf("default palette = %v, want %v", got, want)
	}
}

func TestParsePaletteOrderWithBlack(t *testing.T) {
	palette, err := ParsePalette("bkry")
	if err != nil {
		t.Fatal(err)
	}
	if palette.slot(ColorBlue) != 1 || palette.slot(ColorBlack) != 2 || palette.slot(ColorRed) != 3 || palette.slot(ColorYellow) != 4 {
		t.Fatalf("unexpected slots: %v", palette.Slots)
	}
	wantColors := []string{"#0000FF", "#000000", "#FF0000", "#FFFF00"}
	if got := palette.outputColors(); !reflect.DeepEqual(got, wantColors) {
		t.Fatalf("output colors = %v, want %v", got, wantColors)
	}
}

func TestParsePaletteRejectsDuplicateRole(t *testing.T) {
	if _, err := ParsePalette("wrrb"); err == nil {
		t.Fatal("expected duplicate role error")
	}
}

func TestDecomposeGreenAsYellowAndBlue(t *testing.T) {
	components := decomposeRYB([3]int{0, 255, 0}, DefaultPalette())
	want := []colorComponent{
		{role: ColorYellow, weight: 1},
		{role: ColorBlue, weight: 1},
	}
	if !reflect.DeepEqual(components, want) {
		t.Fatalf("green components = %+v, want %+v", components, want)
	}
}

func TestDecomposeBlackUsesBlackSlot(t *testing.T) {
	palette, err := ParsePalette("kryb")
	if err != nil {
		t.Fatal(err)
	}
	components := decomposeRYB([3]int{}, palette)
	want := []colorComponent{{role: ColorBlack, weight: 1}}
	if !reflect.DeepEqual(components, want) {
		t.Fatalf("black components = %+v, want %+v", components, want)
	}
}

func TestFullSpectrumPaletteRequiresRYBAndOneNeutral(t *testing.T) {
	palette, err := ParsePalette("wrbk")
	if err != nil {
		t.Fatal(err)
	}
	if err := palette.validateFullSpectrum(); err == nil {
		t.Fatal("expected invalid full-spectrum palette")
	}
}
