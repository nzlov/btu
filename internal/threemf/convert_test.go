package threemf

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestConvertRequiresReplaceForExistingOutput(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "output.3mf")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Convert(context.Background(), Request{
		Source: filepath.Join(directory, "source.3mf"),
		Output: outputPath,
	}, nil)
	var exists *OutputExistsError
	if !errors.As(err, &exists) || exists.Path != outputPath {
		t.Fatalf("error = %v, want OutputExistsError for %s", err, outputPath)
	}
	data, readErr := os.ReadFile(outputPath)
	if readErr != nil || string(data) != "existing" {
		t.Fatalf("existing output changed: data=%q err=%v", data, readErr)
	}
}

func TestConvertReplaceOverwritesExistingOutput(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	outputPath := filepath.Join(directory, "output.3mf")
	writeTest3MF(t, sourcePath, map[string]any{
		"filament_colour": []string{"#FF0000"},
		"filament_type":   []string{"PLA"},
		"nozzle_diameter": []string{"0.4"},
	}, map[string]string{
		mainModelName:     `<model><metadata name="Application">BambuStudio-source</metadata></model>`,
		modelSettingsName: `<config><metadata key="extruder" value="1"/></config>`,
	})
	if err := os.WriteFile(outputPath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := Convert(context.Background(), Request{
		Source:  sourcePath,
		Output:  outputPath,
		Replace: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Output != outputPath {
		t.Fatalf("output = %q", report.Output)
	}
	reader, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("replacement is not a 3MF archive: %v", err)
	}
	reader.Close()
}

func TestConvertNativeMixedProject(t *testing.T) {
	directory := t.TempDir()
	templatePath := filepath.Join(directory, "template.3mf")
	sourcePath := filepath.Join(directory, "source.3mf")
	outputPath := filepath.Join(directory, "output.3mf")

	templateSettings := map[string]any{
		"filament_colour":                     []string{"#FFFFFF", "#FF0000", "#FFFF00", "#0000FF"},
		"filament_type":                       []string{"PLA", "PLA", "PLA", "PLA"},
		"nozzle_diameter":                     []string{"0.4"},
		"printer_model":                       "Snapmaker U1",
		"mixed_filament_definitions":          "old",
		"dithering_local_z_mode":              "0",
		"dithering_local_z_infill":            "0",
		"dithering_local_z_direct_multicolor": "0",
	}
	sourceSettings := map[string]any{
		"filament_colour":                []string{"#FF0000", "#FFFFFF", "#0000FF", "#808080"},
		"filament_type":                  []string{"PLA", "PLA", "PLA", "PLA"},
		"filament_is_mixed":              []string{"0", "0", "0", "1"},
		"filament_mixed_components":      []string{"", "", "", "1,2"},
		"filament_mixed_sublayer_ratios": []string{"", "", "", "0,5000,0,5000"},
		"filament_mixed_gradient":        []string{"0", "0", "0", "0"},
		"enable_mixed_color_sublayer":    "1",
		"layer_height":                   "0.1",
		"initial_layer_print_height":     "0.2",
	}

	writeTest3MF(t, templatePath, templateSettings, map[string]string{
		mainModelName:     `<model><metadata name="Application">BambuStudio-2.3.5</metadata></model>`,
		modelSettingsName: `<config/>`,
		sliceInfoName:     `<config><header_item key="target" value="u1"/></config>`,
	})
	writeTest3MF(t, sourcePath, sourceSettings, map[string]string{
		mainModelName:           `<model><metadata name="Application">BambuStudio-source</metadata><triangle paint_color="1C"/></model>`,
		modelSettingsName:       `<config><metadata key="extruder" value="4"/></config>`,
		"Metadata/plate_1.json": `{"first_extruder":4,"nozzle_diameter":0.6}`,
	})

	report, err := Convert(context.Background(), Request{Source: sourcePath, Template: templatePath, Output: outputPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "native-mixed" || report.VirtualMixes != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	settings := readTestJSONMember(t, outputPath, projectSettingsName)
	if got := settings["printer_model"]; got != "Snapmaker U1" {
		t.Fatalf("printer_model = %v", got)
	}
	if got := settings["layer_height"]; got != "0.1" {
		t.Fatalf("layer_height = %v", got)
	}
	wantDefinition := "2,4,1,1,50,0,g,w,m2,z0,xa0,xb0,d0,o0,u1,cm0"
	if got := settings["mixed_filament_definitions"]; got != wantDefinition {
		t.Fatalf("mixed_filament_definitions = %v", got)
	}

	modelSettings := readTestMember(t, outputPath, modelSettingsName)
	if !strings.Contains(modelSettings, `value="5"`) {
		t.Fatalf("virtual material reference was not moved after four U1 slots: %s", modelSettings)
	}
	mainModel := readTestMember(t, outputPath, mainModelName)
	if !strings.Contains(mainModel, `BambuStudio-2.3.5`) || !strings.Contains(mainModel, `paint_color="2C"`) {
		t.Fatalf("main model metadata was not translated: %s", mainModel)
	}
	plate := readTestJSONMember(t, outputPath, "Metadata/plate_1.json")
	if got := plate["first_extruder"]; got != float64(5) {
		t.Fatalf("first_extruder = %v, want 5", got)
	}
	if got := plate["nozzle_diameter"]; got != 0.4 {
		t.Fatalf("nozzle_diameter = %v, want 0.4", got)
	}
	if got := readTestMember(t, outputPath, sliceInfoName); !strings.Contains(got, `value="u1"`) {
		t.Fatalf("slice info was not copied from template: %s", got)
	}
}

func TestConvertFullSpectrumColors(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	outputPath := filepath.Join(directory, "output.3mf")
	sourceSettings := map[string]any{
		"filament_colour":            []string{"#FFFFFF", "#FF0000", "#00FF00"},
		"filament_type":              []string{"PLA", "PLA", "PLA"},
		"layer_height":               "0.12",
		"initial_layer_print_height": "0.2",
		"nozzle_diameter":            []string{"0.4"},
	}
	writeTest3MF(t, sourcePath, sourceSettings, map[string]string{
		mainModelName:     `<model><metadata name="Application">BambuStudio-source</metadata></model>`,
		modelSettingsName: `<config><metadata key="extruder" value="1"/><metadata key="extruder" value="2"/><metadata key="extruder" value="3"/></config>`,
	})

	report, err := Convert(context.Background(), Request{
		Source:       sourcePath,
		Output:       outputPath,
		FullSpectrum: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "full-spectrum" || report.VirtualMixes != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	settings := readTestJSONMember(t, outputPath, projectSettingsName)
	if settings["printer_model"] != "Snapmaker U1" {
		t.Fatalf("built-in printer model = %v", settings["printer_model"])
	}
	wantDefinition := "1,3,1,1,50,0,g134,w26/72/2,m0,z0,xa0,xb0,d0,o0,u1,cm0"
	if settings["mixed_filament_definitions"] != wantDefinition {
		t.Fatalf("definition = %v", settings["mixed_filament_definitions"])
	}
	if settings["dithering_local_z_mode"] != "1" || settings["dithering_local_z_infill"] != "1" {
		t.Fatalf("full spectrum local Z was not enabled: %v", settings)
	}
	if settings["printer_variant"] != "0.4" {
		t.Fatalf("printer_variant = %v, want source nozzle 0.4", settings["printer_variant"])
	}
	wantColors := []any{"#0000FF", "#FF0000", "#FFFF00", "#808080"}
	if got := settings["filament_colour"]; !reflect.DeepEqual(got, wantColors) {
		t.Fatalf("filament colors = %v, want %v", got, wantColors)
	}
	modelSettings := readTestMember(t, outputPath, modelSettingsName)
	for _, reference := range []string{`value="4"`, `value="2"`, `value="5"`} {
		if !strings.Contains(modelSettings, reference) {
			t.Fatalf("model settings missing %s: %s", reference, modelSettings)
		}
	}
	if got := readTestMember(t, outputPath, mainModelName); !strings.Contains(got, "BambuStudio-2.3.5") {
		t.Fatalf("built-in Application metadata was not used: %s", got)
	}
	if got := readTestMember(t, outputPath, sliceInfoName); !strings.Contains(got, "X-BBL-Client-Type") {
		t.Fatalf("built-in slice info was not used: %s", got)
	}
}

func TestConvertUsesSourceMaterialSettingsWithNozzleOverride(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	outputPath := filepath.Join(directory, "output.3mf")
	writeTest3MF(t, sourcePath, map[string]any{
		"filament_colour":               []string{"#FF0000", "#0000FF", "#FFFF00", "#808080"},
		"filament_type":                 []string{"PLA", "PLA", "PLA", "PLA"},
		"filament_settings_id":          []string{"Source T1", "Source T2", "Source T3", "Source T4"},
		"filament_bambu_only":           []string{"1", "2", "3", "4"},
		"filament_self_index":           []string{"1", "1", "2", "2", "3", "3", "4", "4"},
		"filament_extruder_variant":     []string{"Direct Drive Standard", "Direct Drive High Flow", "Direct Drive Standard", "Direct Drive High Flow", "Direct Drive Standard", "Direct Drive High Flow", "Direct Drive Standard", "Direct Drive High Flow"},
		"filament_map":                  []string{"1", "1", "1", "1"},
		"filament_volume_map":           []string{"1", "0", "1", "0"},
		"extruder_type":                 []string{"Direct Drive"},
		"filament_flow_ratio":           []string{"0.91", "9.91", "0.92", "9.92", "0.93", "9.93", "0.94", "9.94"},
		"filament_max_volumetric_speed": []string{"91", "92", "93", "94"},
		"nozzle_temperature":            []string{"211", "911", "222", "922", "233", "933", "244", "944"},
		"pressure_advance":              []string{"0.91", "0.92", "0.93", "0.94"},
		"nozzle_diameter":               []string{"0.4"},
	}, map[string]string{
		mainModelName:     `<model><metadata name="Application">BambuStudio-source</metadata></model>`,
		modelSettingsName: `<config><metadata key="extruder" value="1"/><metadata key="extruder" value="2"/><metadata key="extruder" value="3"/><metadata key="extruder" value="4"/></config>`,
	})

	_, err := Convert(context.Background(), Request{
		Source: sourcePath,
		Output: outputPath,
		Nozzle: "0.2",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	settings := readTestJSONMember(t, outputPath, projectSettingsName)
	for key, want := range map[string][]string{
		"filament_settings_id": {"Source T2", "Source T1", "Source T3", "Source T4"},
		"filament_flow_ratio":  {"0.92", "9.91", "9.93", "0.94"},
		"nozzle_temperature":   {"222", "911", "933", "244"},
	} {
		if got := stringSlice(settings[key]); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, want source values %v", key, got, want)
		}
	}
	for key, want := range map[string][]string{
		"filament_max_volumetric_speed": {"1.6", "1.6", "1.6", "1.6"},
		"pressure_advance":              {"0.02", "0.02", "0.02", "0.02"},
	} {
		if got := stringSlice(settings[key]); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, want 0.2 mm U1 values %v", key, got, want)
		}
	}
	if got := stringSlice(settings["nozzle_diameter"]); !reflect.DeepEqual(got, []string{"0.2", "0.2", "0.2", "0.2"}) {
		t.Fatalf("nozzle_diameter = %v, want 0.2 mm U1 nozzles", got)
	}
	if got := settings["printer_model"]; got != "Snapmaker U1" {
		t.Fatalf("printer_model = %v, want Snapmaker U1", got)
	}
	if _, exists := settings["filament_bambu_only"]; exists {
		t.Fatal("Bambu-only filament setting was copied into U1 output")
	}
}

func TestConvertPropagatesSourceMaterialSettingsToMixedComponents(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	outputPath := filepath.Join(directory, "output.3mf")
	writeTest3MF(t, sourcePath, map[string]any{
		"filament_colour":     []string{"#5E43B7"},
		"filament_type":       []string{"PLA"},
		"filament_flow_ratio": []string{"0.93"},
		"nozzle_temperature":  []string{"235"},
		"nozzle_diameter":     []string{"0.4"},
	}, map[string]string{
		mainModelName:     `<model><metadata name="Application">BambuStudio-source</metadata></model>`,
		modelSettingsName: `<config><metadata key="extruder" value="1"/></config>`,
	})

	_, err := Convert(context.Background(), Request{
		Source:       sourcePath,
		Output:       outputPath,
		FullSpectrum: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	settings := readTestJSONMember(t, outputPath, projectSettingsName)
	for key, want := range map[string][]string{
		"filament_flow_ratio": {"0.93", "0.93", "0.98", "0.93"},
		"nozzle_temperature":  {"235", "235", "220", "235"},
	} {
		if got := stringSlice(settings[key]); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, want source values %v", key, got, want)
		}
	}
}

func TestRemapMaterialSettingKeepsBaselineForConflictingSources(t *testing.T) {
	mapped, ok := remapMaterialSettingValue(
		[]string{"210", "230", "240"},
		[]string{"220", "220", "220", "220"},
		3,
		4,
		map[int][]int{1: {1}, 2: {2}, 3: {2}},
	)
	if !ok {
		t.Fatal("remapMaterialSettingValue rejected valid slot settings")
	}
	want := []string{"210", "220", "220", "220"}
	if got := stringSlice(mapped); !reflect.DeepEqual(got, want) {
		t.Fatalf("mapped values = %v, want %v", got, want)
	}
}

func TestConvertPreservesAllMaterialSlotsWithoutMixing(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	outputPath := filepath.Join(directory, "output.3mf")
	colors := []string{"#FCE300", "#FB0207", "#161616", "#FFFFFF", "#5E43B7", "#00AE42"}
	types := []string{"PETG", "PLA", "PLA", "PLA", "PLA", "PLA"}
	settingsIDs := []string{"Bambu PETG Basic", "Bambu PLA Basic", "Bambu PLA Basic", "Bambu PLA Basic", "Bambu PLA Basic", "Bambu PLA Basic"}
	flowRatios := []string{"0.95", "0.98", "0.98", "0.98", "0.98", "0.98"}
	nozzleTemperatures := []string{"255", "255", "220", "220", "220", "220", "220", "220", "220", "220", "220", "220"}
	flushMatrix := repeatedSlotValue("123", 36)
	flushVector := repeatedSlotValue("61", 12)
	writeTest3MF(t, sourcePath, map[string]any{
		"filament_colour":                colors,
		"filament_type":                  types,
		"filament_settings_id":           settingsIDs,
		"filament_flow_ratio":            flowRatios,
		"filament_max_volumetric_speed":  repeatedSlotValue("99", 6),
		"filament_is_mixed":              []string{"0", "0", "0", "0", "0", "0"},
		"filament_mixed_components":      []string{"", "", "", "", "", ""},
		"filament_mixed_sublayer_ratios": []string{"", "", "", "", "", ""},
		"filament_mixed_gradient":        []string{"0", "0", "0", "0", "0", "0"},
		"filament_mixed_gradient_range":  []string{"", "", "", "", "", ""},
		"nozzle_diameter":                []string{"0.4"},
		"nozzle_temperature":             nozzleTemperatures,
		"pressure_advance":               repeatedSlotValue("0.99", 6),
		"flush_volumes_matrix":           flushMatrix,
		"flush_volumes_vector":           flushVector,
		"enable_mixed_color_sublayer":    "1",
	}, map[string]string{
		mainModelName:     `<model><metadata name="Application">BambuStudio-source</metadata></model>`,
		modelSettingsName: `<config><metadata key="extruder" value="1"/><metadata key="extruder" value="6"/></config>`,
	})

	report, err := Convert(context.Background(), Request{
		Source:                sourcePath,
		Output:                outputPath,
		Nozzle:                "0.2",
		PreserveMaterialSlots: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "material-slots" || report.VirtualMixes != 0 || report.Colors != strings.Join(colors, ",") {
		t.Fatalf("unexpected report: %+v", report)
	}
	wantMapping := map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6}
	if !reflect.DeepEqual(report.PhysicalMapping, wantMapping) {
		t.Fatalf("mapping = %v, want %v", report.PhysicalMapping, wantMapping)
	}

	settings := readTestJSONMember(t, outputPath, projectSettingsName)
	for key, want := range map[string][]string{
		"filament_colour":      colors,
		"filament_type":        types,
		"filament_settings_id": settingsIDs,
		"filament_flow_ratio":  flowRatios,
		"nozzle_temperature":   nozzleTemperatures,
	} {
		if got := stringSlice(settings[key]); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}
	if got := stringSlice(settings["nozzle_diameter"]); !reflect.DeepEqual(got, []string{"0.2", "0.2", "0.2", "0.2"}) {
		t.Fatalf("nozzle_diameter = %v, want four U1 heads", got)
	}
	for key, want := range map[string][]string{
		"filament_max_volumetric_speed": repeatedSlotValue("1.6", 6),
		"pressure_advance":              repeatedSlotValue("0.02", 6),
		"flush_volumes_matrix":          flushMatrix,
		"flush_volumes_vector":          flushVector,
	} {
		if got := stringSlice(settings[key]); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}
	if got := stringSlice(settings["filament_unloading_speed"]); len(got) != len(colors) {
		t.Fatalf("filament_unloading_speed has %d slots, want %d", len(got), len(colors))
	}
	if settings["mixed_filament_definitions"] != "" || settings["dithering_local_z_mode"] != "0" {
		t.Fatalf("mixing remained enabled: %v", settings)
	}
	if got := stringSlice(settings["filament_is_mixed"]); !reflect.DeepEqual(got, []string{"0", "0", "0", "0", "0", "0"}) {
		t.Fatalf("filament_is_mixed = %v", got)
	}
	if got := stringSlice(settings["filament_mixed_gradient"]); !reflect.DeepEqual(got, []string{"0", "0", "0", "0", "0", "0"}) {
		t.Fatalf("filament_mixed_gradient = %v", got)
	}
	modelSettings := readTestMember(t, outputPath, modelSettingsName)
	for _, reference := range []string{`value="1"`, `value="6"`} {
		if !strings.Contains(modelSettings, reference) {
			t.Fatalf("model settings missing %s: %s", reference, modelSettings)
		}
	}
}

func TestConvertSelectsNozzleBaseline(t *testing.T) {
	tests := []struct {
		name          string
		sourceNozzle  string
		requestNozzle string
		wantNozzle    string
	}{
		{name: "from source", sourceNozzle: "0.6", wantNozzle: "0.6"},
		{name: "request override", sourceNozzle: "0.2", requestNozzle: "0.8", wantNozzle: "0.8"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			sourcePath := filepath.Join(directory, "source.3mf")
			outputPath := filepath.Join(directory, "output.3mf")
			writeTest3MF(t, sourcePath, map[string]any{
				"filament_colour": []string{"#FFFFFF"},
				"filament_type":   []string{"PLA"},
				"nozzle_diameter": []string{test.sourceNozzle},
			}, map[string]string{
				mainModelName:           `<model><metadata name="Application">BambuStudio-source</metadata></model>`,
				modelSettingsName:       `<config><metadata key="extruder" value="1"/></config>`,
				"Metadata/plate_1.json": `{"first_extruder":1,"nozzle_diameter":0.2}`,
			})

			_, err := Convert(context.Background(), Request{
				Source: sourcePath,
				Output: outputPath,
				Nozzle: test.requestNozzle,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			settings := readTestJSONMember(t, outputPath, projectSettingsName)
			wantNozzles := []string{test.wantNozzle, test.wantNozzle, test.wantNozzle, test.wantNozzle}
			if got := stringSlice(settings["nozzle_diameter"]); !reflect.DeepEqual(got, wantNozzles) {
				t.Fatalf("nozzle_diameter = %v, want %v", got, wantNozzles)
			}
			if got := settings["printer_variant"]; got != test.wantNozzle {
				t.Fatalf("printer_variant = %v, want %s", got, test.wantNozzle)
			}
			printSettingsID, ok := settings["print_settings_id"].(string)
			if !ok || !strings.Contains(printSettingsID, "("+test.wantNozzle+" nozzle)") {
				t.Fatalf("print_settings_id = %v", settings["print_settings_id"])
			}
			plate := readTestJSONMember(t, outputPath, "Metadata/plate_1.json")
			wantPlateNozzle, _ := strconv.ParseFloat(test.wantNozzle, 64)
			if got := plate["nozzle_diameter"]; got != wantPlateNozzle {
				t.Fatalf("plate nozzle_diameter = %v, want %v", got, wantPlateNozzle)
			}
		})
	}
}

func writeTest3MF(t *testing.T, path string, settings map[string]any, members map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	settingsData, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	writeTestZipMember(t, writer, projectSettingsName, string(settingsData))
	for name, data := range members {
		writeTestZipMember(t, writer, name, data)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestZipMember(t *testing.T, writer *zip.Writer, name, data string) {
	t.Helper()
	member, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(member, data); err != nil {
		t.Fatal(err)
	}
}

func readTestJSONMember(t *testing.T, path, name string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(readTestMember(t, path, name)), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func readTestMember(t *testing.T, path, name string) string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		member, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(member)
		member.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Fatalf("member %s not found", name)
	return ""
}
