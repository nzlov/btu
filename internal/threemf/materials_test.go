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

	plan, err := planMaterials(source, template, DefaultPalette(), false, projectMaterialUsage{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "native-mixed" || plan.virtualMixes != 2 || !plan.hasMultiColor {
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

	plan, err := planMaterials(source, template, DefaultPalette(), false, projectMaterialUsage{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.allMapping[4]; got != 5 {
		t.Fatalf("virtual T4 mapped to T%d, want T5", got)
	}
}

func TestPlanFullSpectrumWithBlackPreviewNeutral(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#00FF00"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}
	template := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}
	palette, err := ParsePalette("cmyb")
	if err != nil {
		t.Fatal(err)
	}

	plan, err := planMaterials(source, template, palette, true, projectMaterialUsage{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "full-spectrum" || !plan.forceLocalZ || plan.virtualMixes != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	wantMapping := map[int]int{1: 4, 2: 2, 3: 5}
	if !reflect.DeepEqual(plan.allMapping, wantMapping) {
		t.Fatalf("mapping = %v, want %v", plan.allMapping, wantMapping)
	}
	if plan.palette.String() != "cmyb" || len(plan.plates) != 1 || plan.plates[0].Colors != "cmyw" {
		t.Fatalf("palette plan = %+v, plates = %+v", plan.palette, plan.plates)
	}
}

func TestPlanPreservesUnusedDeclaredBaseColors(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FFFF00", "#FF0000"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 100, 3: 10, 4: 20}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.physicalMapping[1]; got != 4 {
		t.Fatalf("black T1 mapped to U1 T%d, want black slot T4", got)
	}
	if got := plan.physicalMapping[2]; got <= 4 {
		t.Fatalf("unused white T2 mapped to physical U1 T%d", got)
	}
	if got := plan.palette.String(); got != "cmyg" || len(plan.plates) != 1 || plan.plates[0].Colors != "cmyb" {
		t.Fatalf("palette = %q, plates = %+v", got, plan.plates)
	}
}

func TestPlanReplacesUnusedRedSlotWithRequiredWhite(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FFFF00"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 20, 2: 30, 3: 10}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "cmyg" || plan.plates[0].Colors != "cmyw" {
		t.Fatalf("palette = %q, plates = %+v", got, plan.plates)
	}
	want := map[int]int{1: 5, 2: 4, 3: 3}
	if !reflect.DeepEqual(plan.physicalMapping, want) {
		t.Fatalf("mapping = %v, want %v", plan.physicalMapping, want)
	}
}

func TestPlanCanReplaceAnotherUnusedBaseSlot(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FF0000"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 20, 2: 30, 3: 10}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "cmyg" || plan.plates[0].Colors != "cmyw" {
		t.Fatalf("palette = %q, plates = %+v", got, plan.plates)
	}
}

func TestPlanChoosesBaseSlotForNearWhite(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#F8F8F8", "#FF0000"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 20, 2: 30, 3: 10}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "cmyg" || plan.plates[0].Colors != "cmyw" {
		t.Fatalf("palette = %q, plates = %+v", got, plan.plates)
	}
}

func TestPlanGroupsDuplicateUsedColors(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 10, 2: 20, 3: 10, 4: 10, 5: 10}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.palette.String(); got != "cmyg" || plan.plates[0].Colors != "cmyw" {
		t.Fatalf("palette = %q, plates = %+v", got, plan.plates)
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

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{2: 10}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.physicalMapping[2]; got != 2 {
		t.Fatalf("used red T2 mapped to U1 T%d, want T2", got)
	}
	if plan.physicalMapping[1] == 0 {
		t.Fatalf("unused PETG color has no mapping: %v", plan.physicalMapping)
	}
}

func TestPlanMixesFifthBaseColorWithoutPrompt(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 1, 2: 1, 3: 1, 4: 1, 5: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "full-spectrum" || plan.virtualMixes != 1 || plan.palette.String() != "cmyg" || plan.plates[0].Colors != "cmyw" {
		t.Fatalf("unexpected five-base plan: %+v", plan)
	}
}

func TestPlanFullSpectrumPreservesBothDeclaredNeutrals(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	tests := []struct {
		name  string
		usage materialUsage
		want  string
	}{
		{name: "black", usage: materialUsage{1: 100, 2: 1, 3: 1, 4: 1, 5: 1}, want: "cmyw"},
		{name: "white", usage: materialUsage{1: 1, 2: 100, 3: 1, 4: 1, 5: 1}, want: "cmyw"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := planMaterials(source, testTemplate(), DefaultPalette(), true, projectMaterialUsage{Total: test.usage}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.plates[0].Colors; got != test.want || plan.palette.String() != "cmyg" {
				t.Fatalf("palette = %q, plate = %q, want %q", plan.palette.String(), got, test.want)
			}
		})
	}
}

func TestPlanFullSpectrumRecognizesNearNeutralColors(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#161616", "#F8F8F8", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), true, projectMaterialUsage{Total: materialUsage{1: 1, 2: 100, 3: 1, 4: 1, 5: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.plates[0].Colors; got != "cmyw" || plan.palette.String() != "cmyg" {
		t.Fatalf("palette = %q, plate = %q", plan.palette.String(), got)
	}
}

func TestPlanRequestsMappingForEveryDeclaredNonBaseColor(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FCE300", "#FB0207", "#161616", "#FFFFFF", "#5E43B7", "#00AE42"},
		"filament_type":   []string{"PETG", "PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	usage := materialUsage{2: 10, 3: 100, 4: 30, 5: 20}

	_, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: usage}, nil)
	var required *FullSpectrumRequiredError
	if !errors.As(err, &required) {
		t.Fatalf("error = %v, want FullSpectrumRequiredError", err)
	}
	if required.ColorCount != 6 || required.NonBaseCount != 2 || len(required.Mappings) != 6 {
		t.Fatalf("unexpected required colors: %+v", required)
	}
	if required.Mappings[0].Used || !required.Mappings[4].Used || required.Mappings[5].Used {
		t.Fatalf("used markers = %+v", required.Mappings)
	}
	if required.Mappings[4].Suggested != ColorCyan || required.Mappings[5].Suggested != ColorCyan {
		t.Fatalf("suggestions = %+v", required.Mappings)
	}
}

func TestPlanMapsBlackCatWithPerPlateNeutralSlots(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FCE300", "#FB0207", "#161616", "#FFFFFF", "#5E43B7", "#00AE42"},
		"filament_type":   []string{"PETG", "PLA", "PLA", "PLA", "PLA", "PLA"},
	}
	usage := projectMaterialUsage{
		Total: materialUsage{2: 10, 3: 100, 4: 30, 5: 20},
		Plates: []plateMaterialUsage{
			{ID: 1, Name: "eyes", Materials: materialUsage{4: 30}},
			{ID: 2, Name: "body", Materials: materialUsage{3: 100}},
			{ID: 3, Name: "ears", Materials: materialUsage{5: 20}},
			{ID: 4, Name: "nose", Materials: materialUsage{2: 10}},
		},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), true, usage, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "full-spectrum" || plan.palette.String() != "cmyg" || plan.virtualMixes != 2 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	want := map[int]int{1: 3, 2: 2, 3: 4, 4: 4, 5: 5, 6: 6}
	if !reflect.DeepEqual(plan.allMapping, want) {
		t.Fatalf("mapping = %v, want %v", plan.allMapping, want)
	}
	wantPlates := []PlateReport{
		{Number: 1, Name: "eyes", Colors: "cmyw", Neutral: ColorWhite},
		{Number: 2, Name: "body", Colors: "cmyb", Neutral: ColorBlack},
		{Number: 3, Name: "ears", Colors: "cmyw", Neutral: ColorWhite},
		{Number: 4, Name: "nose", Colors: "cmyg", Neutral: ColorGray},
	}
	if !reflect.DeepEqual(plan.plates, wantPlates) {
		t.Fatalf("plates = %+v, want %+v", plan.plates, wantPlates)
	}
}

func TestPlanAllowsMappingSeveralColorsToOneMixedTarget(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FF0000", "#5E43B7", "#00AE42"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}
	targets := map[int]string{1: "#FF0000", 2: "#5E43B7", 3: "#5E43B7"}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), true, projectMaterialUsage{Total: materialUsage{1: 1, 2: 1, 3: 1}}, targets)
	if err != nil {
		t.Fatal(err)
	}
	if plan.virtualMixes != 1 || plan.allMapping[2] != plan.allMapping[3] {
		t.Fatalf("unexpected mapped plan: %+v", plan)
	}
}

func TestPlanUsesWhiteNeutralAndMixesBlackOnSamePlate(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FF0000", "#FFFF00", "#FFFFFF", "#000000"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 10, 2: 10, 3: 10, 4: 10}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "full-spectrum" || plan.virtualMixes != 1 || plan.palette.String() != "cmyg" || plan.plates[0].Colors != "cmyw" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	want := map[int]int{1: 2, 2: 3, 3: 4, 4: 5}
	if !reflect.DeepEqual(plan.allMapping, want) {
		t.Fatalf("mapping = %v, want %v", plan.allMapping, want)
	}
}

func TestPlanUsesFreeSlotForBlackInsteadOfMixingIt(t *testing.T) {
	source := map[string]any{
		"filament_colour": []string{"#FF0000", "#FFFF00", "#000000"},
		"filament_type":   []string{"PLA", "PLA", "PLA"},
	}

	plan, err := planMaterials(source, testTemplate(), DefaultPalette(), false, projectMaterialUsage{Total: materialUsage{1: 10, 2: 10, 3: 10}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.mode != "layered" || plan.virtualMixes != 0 || plan.palette.String() != "cmyg" || plan.plates[0].Colors != "cmyb" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if got := plan.allMapping[3]; got != 4 {
		t.Fatalf("black mapped to T%d, want direct slot T4", got)
	}
}

func testTemplate() map[string]any {
	return map[string]any{
		"filament_colour": []string{"#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":   []string{"PLA", "PLA", "PLA", "PLA"},
	}
}
