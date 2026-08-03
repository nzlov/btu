package threemf

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuiltInBaselinesMatchNozzleSize(t *testing.T) {
	for _, nozzle := range []string{"0.2", "0.4", "0.6", "0.8"} {
		t.Run(nozzle, func(t *testing.T) {
			baseline, err := loadU1Baseline(nozzle)
			if err != nil {
				t.Fatal(err)
			}
			wantNozzles := []string{nozzle, nozzle, nozzle, nozzle}
			if got := stringSlice(baseline.projectSettings["nozzle_diameter"]); !reflect.DeepEqual(got, wantNozzles) {
				t.Fatalf("nozzle_diameter = %v, want %v", got, wantNozzles)
			}
			if got := baseline.projectSettings["printer_variant"]; got != nozzle {
				t.Fatalf("printer_variant = %v, want %s", got, nozzle)
			}
			if got := stringSlice(baseline.projectSettings["filament_colour"]); len(got) != 4 {
				t.Fatalf("filament colors = %v, want four physical slots", got)
			}
			if len(baseline.application) == 0 || len(baseline.sliceInfo) == 0 {
				t.Fatal("baseline model or slice metadata is empty")
			}
		})
	}
}

func TestNozzleSizeFromSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		want     string
		wantErr  string
	}{
		{name: "single", settings: map[string]any{"nozzle_diameter": []string{"0.4"}}, want: "0.4"},
		{name: "matching heads", settings: map[string]any{"nozzle_diameter": []string{"0.6", "0.6", "0.6", "0.6"}}, want: "0.6"},
		{name: "missing", settings: map[string]any{}, wantErr: "has no nozzle_diameter"},
		{name: "unsupported", settings: map[string]any{"nozzle_diameter": []string{"0.5"}}, wantErr: "unsupported nozzle size"},
		{name: "mixed", settings: map[string]any{"nozzle_diameter": []string{"0.2", "0.4"}}, wantErr: "multiple nozzle sizes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nozzleSizeFromSettings(test.settings)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("nozzle = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplyNozzleOverridePreservesTemplateSettings(t *testing.T) {
	template := u1Baseline{
		projectSettings: map[string]any{
			"nozzle_diameter": []string{"0.2"},
			"printer_model":   "Custom U1",
			"printer_variant": "custom",
			"custom_setting":  "keep",
		},
		sliceInfo:   []byte("custom slice info"),
		application: []byte("custom application"),
	}
	got, err := applyNozzleOverride(template, "0.8")
	if err != nil {
		t.Fatal(err)
	}
	wantNozzles := []string{"0.8", "0.8", "0.8", "0.8"}
	if nozzles := stringSlice(got.projectSettings["nozzle_diameter"]); !reflect.DeepEqual(nozzles, wantNozzles) {
		t.Fatalf("nozzle_diameter = %v, want %v", nozzles, wantNozzles)
	}
	if got.projectSettings["printer_variant"] != "0.8" {
		t.Fatalf("printer_variant = %v, want 0.8", got.projectSettings["printer_variant"])
	}
	if got.projectSettings["printer_model"] != "Custom U1" || got.projectSettings["custom_setting"] != "keep" {
		t.Fatalf("unrelated template settings changed: %v", got.projectSettings)
	}
	if string(got.sliceInfo) != "custom slice info" || string(got.application) != "custom application" {
		t.Fatal("template metadata changed")
	}
}
