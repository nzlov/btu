package threemf

import (
	"errors"
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

	plan, err := planMaterials(source, template, DefaultPalette(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "native-mixed" || plan.virtualMixes != 2 || !plan.hasThreeColor {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if got := plan.definitions; got != "1,2,1,1,50,0,g123,w44/37/19,m0,z0,xa0,xb0,d0,o0,u1,cm0;4,2,1,1,50,0,g,w,m0,z2,xa0,xb0,d0,o0,u2,cm3,r1/0.9000/0.1000" {
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

	plan, err := planMaterials(source, template, DefaultPalette(), false, nil)
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

	plan, err := planMaterials(source, template, palette, true, nil)
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

func TestPlanIgnoresUnusedColorWhenMappingUsedMaterials(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FFFF00", "#FF0000"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, materialUsage{1: 100, 3: 10, 4: 20})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.physicalMapping[1]; got != 4 {
		t.Fatalf("used black T1 mapped to U1 T%d, want black slot T4", got)
	}
	if got := plan.physicalMapping[2]; got != 4 {
		t.Fatalf("unused white T2 mapped to U1 T%d, want neutral fallback T4", got)
	}
	if got := plan.palette.String(); got != "bryk" {
		t.Fatalf("palette = %q, want bryk", got)
	}
}

func TestPlanReplacesUnusedRedSlotWithRequiredWhite(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FFFF00"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, materialUsage{1: 20, 2: 30, 3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "bwyk" {
		t.Fatalf("palette = %q, want bwyk", got)
	}
	want := map[int]int{1: 4, 2: 2, 3: 3}
	if !reflect.DeepEqual(plan.physicalMapping, want) {
		t.Fatalf("mapping = %v, want %v", plan.physicalMapping, want)
	}
}

func TestPlanCanReplaceAnotherUnusedBaseSlot(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FF0000"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, materialUsage{1: 20, 2: 30, 3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "wryk" {
		t.Fatalf("palette = %q, want wryk", got)
	}
}

func TestPlanChoosesBaseSlotForNearWhite(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#F8F8F8", "#FF0000"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, materialUsage{1: 20, 2: 30, 3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "wryk" {
		t.Fatalf("palette = %q, want wryk", got)
	}
}

func TestPlanGroupsDuplicateUsedColors(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, materialUsage{1: 10, 2: 20, 3: 10, 4: 10, 5: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "bryw" {
		t.Fatalf("palette = %q, want bryw", got)
	}
	if plan.physicalMapping[1] != plan.physicalMapping[2] {
		t.Fatalf("duplicate whites mapped differently: %v", plan.physicalMapping)
	}
}

func TestPlanDoesNotRejectUnusedMaterialType(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FFFF00", "#FF0000"},
		"filament_type":   []string{"PETG", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, materialUsage{2: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.physicalMapping[2]; got != 2 {
		t.Fatalf("used red T2 mapped to U1 T%d, want T2", got)
	}
	if plan.physicalMapping[1] == 0 {
		t.Fatalf("unused PETG metadata has no fallback mapping: %v", plan.physicalMapping)
	}
}

func TestPlanRequiresFullSpectrumForMoreThanFourUsedColors(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	_, err := planMaterials(source, testTemplate(), DefaultPalette(), false, materialUsage{1: 1, 2: 1, 3: 1, 4: 1, 5: 1})
	var required *FullSpectrumRequiredError
	if !errors.As(err, &required) || required.ColorCount != 5 {
		t.Fatalf("error = %v, want five-color FullSpectrumRequiredError", err)
	}
}

func TestPlanFullSpectrumChoosesMoreVisibleNeutral(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	tests := []struct {
		name  string
		usage materialUsage
		want  string
	}{
		{name: "black", usage: materialUsage{1: 100, 2: 1, 3: 1, 4: 1, 5: 1}, want: "bryk"},
		{name: "white", usage: materialUsage{1: 1, 2: 100, 3: 1, 4: 1, 5: 1}, want: "bryw"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := planMaterials(source, testTemplate(), DefaultPalette(), true, test.usage)
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.palette.String(); got != test.want {
				t.Fatalf("palette = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPlanFullSpectrumRecognizesNearNeutralColors(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#161616", "#F8F8F8", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), true, materialUsage{1: 1, 2: 100, 3: 1, 4: 1, 5: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "bryw" {
		t.Fatalf("palette = %q, want bryw", got)
	}
}

func testTemplate() map[string]any {
	return map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}
}
