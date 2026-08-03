package threemf

import "testing"

func TestSharedMaterialAcrossDifferentNeutralsUsesCMYOnly(t *testing.T) {
	colors := [][3]int{{0, 0, 0}, {255, 255, 255}}
	usage := projectMaterialUsage{
		Total: materialUsage{1: 20, 2: 10},
		Plates: []plateMaterialUsage{
			{ID: 1, Materials: materialUsage{1: 10}},
			{ID: 2, Materials: materialUsage{1: 10, 2: 10}},
		},
	}
	plans, err := selectPlatePlans(colors, usage, DefaultPalette())
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].neutral != ColorBlack || plans[1].neutral != ColorWhite {
		t.Fatalf("plate neutrals = %+v, want black then white", plans)
	}
	recipes, err := selectProjectRecipes(colors, usage, plans, DefaultPalette())
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range recipes[0].components {
		if component == 4 {
			t.Fatalf("shared black recipe depends on dynamic T4: %+v", recipes[0])
		}
	}
}
