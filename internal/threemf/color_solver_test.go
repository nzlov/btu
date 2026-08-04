package threemf

import (
	"math"
	"testing"
)

func TestPurpleRecipeImprovesFullSpectrumPreview(t *testing.T) {
	target := [3]int{0x5E, 0x43, 0xB7}
	palette, err := ParsePalette("cmyw")
	if err != nil {
		t.Fatal(err)
	}
	recipe, ok := bestColorRecipe(target, palette)
	if !ok {
		t.Fatal("purple has no recipe")
	}
	if len(recipe.components) != 4 {
		t.Fatalf("purple recipe = %+v, want all four physical colors", recipe)
	}

	oldPalette := Palette{Slots: [4]ColorRole{ColorCyan, ColorMagenta, ColorWhite, ColorBlack}}
	old := evaluateRecipe(target, oldPalette, []int{1, 2, 3}, []int{55, 13, 32})
	if recipe.error >= old.error {
		t.Fatalf("new recipe %+v (delta %.2f) is not better than old %+v (delta %.2f)",
			recipe, math.Sqrt(recipe.error), old, math.Sqrt(old.error))
	}
	if delta := math.Sqrt(recipe.error); delta > 3 {
		t.Fatalf("preview %s has delta %.2f from %s", canonicalColor(recipe.preview), delta, canonicalColor(target))
	}
}

func TestMixModesApplyTheirComponentRules(t *testing.T) {
	target := [3]int{0x5E, 0x43, 0xB7}
	palette, err := ParsePalette("cmyw")
	if err != nil {
		t.Fatal(err)
	}

	ratio, ok := bestColorRecipeForMode(target, palette, MixModeRatio)
	if !ok || len(ratio.components) > 3 {
		t.Fatalf("ratio recipe = %+v, ok = %v", ratio, ok)
	}
	gradient, ok := bestColorRecipeForMode(target, palette, MixModeGradient)
	if !ok || len(gradient.components) != 2 || gradient.weights[0] != 50 || gradient.weights[1] != 50 {
		t.Fatalf("gradient recipe = %+v, ok = %v", gradient, ok)
	}
}

func TestMixModesDoNotAffectDirectBaseColors(t *testing.T) {
	palette := DefaultPalette()
	for _, mode := range []MixMode{MixModeRatio, MixModeCycle, MixModeMatch, MixModeGradient} {
		recipe, ok := bestColorRecipeForMode(roleRGB(ColorCyan), palette, mode)
		if !ok || len(recipe.components) != 1 || recipe.components[0] != 1 {
			t.Fatalf("%s recipe = %+v, ok = %v", mode, recipe, ok)
		}
	}
}
