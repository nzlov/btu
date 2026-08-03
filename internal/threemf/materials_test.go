package threemf

import (
	"reflect"
	"testing"
)

func TestParseWeights(t *testing.T) {
	weights, err := parseWeights("0,3300,0,3300,0,3400", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{33, 33, 34}
	if !reflect.DeepEqual(weights, want) {
		t.Fatalf("weights = %v, want %v", weights, want)
	}
}

func TestPlanNativeMaterials(t *testing.T) {
	source := map[string]any{
		"filament_colour":                []string{"#FFFFFF", "#BB3D43", "#FFF144", "#2850E0", "#876788", "#DD9EA1"},
		"filament_type":                  []string{"PLA", "PLA", "PLA", "PLA", "PLA", "PLA"},
		"filament_is_mixed":              []string{"0", "0", "0", "0", "1", "1"},
		"filament_mixed_components":      []string{"", "", "", "", "4,2,3", "1,2"},
		"filament_mixed_sublayer_ratios": []string{"", "", "", "", "0,4400,0,3700,0,1900", "0,5000,0,5000"},
		"filament_mixed_gradient":        []string{"0", "0", "0", "0", "0", "1"},
		"filament_mixed_gradient_range":  []string{"", "", "", "", "", "0.9000,0.1000"},
	}
	template := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, template, DefaultPalette(), false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "native-mixed" || plan.virtualMixes != 2 || !plan.hasThreeColor {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if got := plan.definitions; got != "4,2,1,1,50,0,g423,w44/37/19,m0,z0,xa0,xb0,d0,o0,u1,cm0;1,2,1,1,50,0,g,w,m0,z2,xa0,xb0,d0,o0,u2,cm3,r1/0.9000/0.1000" {
		t.Fatalf("definitions = %q", got)
	}
}

func TestPlanPlacesVirtualMaterialsAfterTemplateSlots(t *testing.T) {
	source := map[string]any{
		"filament_colour":                []string{"#FF0000", "#FFFFFF", "#0000FF", "#808080"},
		"filament_type":                  []string{"PLA", "PLA", "PLA", "PLA"},
		"filament_is_mixed":              []string{"0", "0", "0", "1"},
		"filament_mixed_components":      []string{"", "", "", "1,2"},
		"filament_mixed_sublayer_ratios": []string{"", "", "", "0,5000,0,5000"},
		"filament_mixed_gradient":        []string{"0", "0", "0", "0"},
	}
	template := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, template, DefaultPalette(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.allMapping[4]; got != 5 {
		t.Fatalf("virtual T4 mapped to T%d, want T5", got)
	}
}

func TestPlanFullSpectrumWithCustomOrderAndBlack(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#00FF00"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}
	template := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}
	palette, err := ParsePalette("bkry")
	if err != nil {
		t.Fatal(err)
	}

	plan, err := planMaterials(source, template, palette, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "full-spectrum" || !plan.forceLocalZ || plan.virtualMixes != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	wantMapping := map[int]int{1: 2, 2: 3, 3: 5}
	if !reflect.DeepEqual(plan.allMapping, wantMapping) {
		t.Fatalf("mapping = %v, want %v", plan.allMapping, wantMapping)
	}
	wantDefinition := "4,1,1,1,50,0,g,w,m2,z0,xa0,xb0,d0,o0,u1,cm0"
	if plan.definitions != wantDefinition {
		t.Fatalf("definition = %q, want %q", plan.definitions, wantDefinition)
	}
}
