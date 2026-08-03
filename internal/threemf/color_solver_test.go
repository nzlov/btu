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

	oldPalette, err := ParsePalette("cmwb")
	if err != nil {
		t.Fatal(err)
	}
	old := evaluateRecipe(target, oldPalette, []int{1, 2, 3}, []int{55, 13, 32})
	if recipe.error >= old.error {
		t.Fatalf("new recipe %+v (delta %.2f) is not better than old %+v (delta %.2f)",
			recipe, math.Sqrt(recipe.error), old, math.Sqrt(old.error))
	}
	if delta := math.Sqrt(recipe.error); delta > 3 {
		t.Fatalf("preview %s has delta %.2f from %s", canonicalColor(recipe.preview), delta, canonicalColor(target))
	}
}

func TestBlackCatPaletteIncludesRecipesForUnusedColors(t *testing.T) {
	colors := [][3]int{
		{0xFC, 0xE3, 0x00},
		{0xFB, 0x02, 0x07},
		{0x16, 0x16, 0x16},
		{0xFF, 0xFF, 0xFF},
		{0x5E, 0x43, 0xB7},
		{0x00, 0xAE, 0x42},
	}
	palette, recipes, err := selectColorRecipes(colors, DefaultPalette(), materialUsage{2: 10, 3: 100, 4: 30, 5: 20})
	if err != nil {
		t.Fatal(err)
	}
	if got := palette.String(); got != "cmyw" {
		t.Fatalf("palette = %q, want cmyw", got)
	}
	if len(recipes[0].components) != 1 || recipes[0].components[0] != 3 {
		t.Fatalf("unused yellow recipe = %+v, want physical T3", recipes[0])
	}
	if len(recipes[5].components) < 2 {
		t.Fatalf("unused green recipe = %+v, want a generated mix", recipes[5])
	}
}
