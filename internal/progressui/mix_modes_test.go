package progressui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestColorPlanModelShowsSideBySideSequences(t *testing.T) {
	model := testColorPlanModel()
	view := model.View()
	for _, want := range []string{
		"Review color plan", "Original order", "New order / mode / replacement",
		"T1 \x1b[48;2;94;67;183m  \x1b[0m #5E43B7 used", "-> T5",
		"T5 \x1b[48;2;91;71;171m  \x1b[0m #5B47AB", "Mixed", "Ratio", "Keep",
		"Local-Z settings", "Layer-height subdivision", "Subdivide infill", "Subdivide whole objects",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestColorPlanModelChangesModeForSelectedOutput(t *testing.T) {
	model := testColorPlanModel()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(colorPlanModel)
	if result.outputs[4].Mode != 1 || result.outputs[5].Mode != 0 {
		t.Fatalf("modes = %d,%d, want 1,0", result.outputs[4].Mode, result.outputs[5].Mode)
	}
	if view := result.View(); !strings.Contains(view, "Cycle") {
		t.Fatalf("view does not show changed mode:\n%s", view)
	}
}

func TestColorPlanModelReplacesWholeOutputSlot(t *testing.T) {
	model := testColorPlanModel()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated, _ = updated.(colorPlanModel).Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(colorPlanModel)
	if result.outputs[4].ReplacementSlot != 6 {
		t.Fatalf("replacement = T%d, want T6", result.outputs[4].ReplacementSlot)
	}
	if result.effectiveOutput(result.sources[0].OutputSlot) != 6 || result.effectiveOutput(result.sources[1].OutputSlot) != 6 {
		t.Fatalf("shared source mappings did not change together: %+v", result.sources[:2])
	}
	view := result.View()
	if !strings.Contains(view, "T6 #30A45B") {
		t.Fatalf("replacement target is not visible:\n%s", view)
	}
}

func TestColorPlanModelMovesOnlyAcrossMixedOutputs(t *testing.T) {
	model := testColorPlanModel()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	result := updated.(colorPlanModel)
	if result.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", result.cursor)
	}
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyUp})
	result = updated.(colorPlanModel)
	if result.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", result.cursor)
	}
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := updated.(colorPlanModel).cursor; got != 0 {
		t.Fatalf("cursor moved into base outputs: %d", got)
	}
}

func TestColorPlanModelTogglesLocalZSettingsIndependently(t *testing.T) {
	model := testColorPlanModel()
	model.cursor = model.editableCount()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(colorPlanModel)
	if !result.localZ.LayerHeight || result.localZ.Infill || result.localZ.WholeObjects {
		t.Fatalf("local Z after first toggle = %+v", result.localZ)
	}
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.(colorPlanModel).Update(tea.KeyMsg{Type: tea.KeySpace})
	result = updated.(colorPlanModel)
	if !result.localZ.LayerHeight || !result.localZ.Infill || result.localZ.WholeObjects {
		t.Fatalf("local Z after second toggle = %+v", result.localZ)
	}
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.(colorPlanModel).Update(tea.KeyMsg{Type: tea.KeyLeft})
	result = updated.(colorPlanModel)
	if !result.localZ.LayerHeight || !result.localZ.Infill || !result.localZ.WholeObjects {
		t.Fatalf("local Z after third toggle = %+v", result.localZ)
	}
	view := result.View()
	if strings.Count(view, "Enabled") != 3 {
		t.Fatalf("view does not show three enabled settings:\n%s", view)
	}
}

func TestColorPlanModelSupportsSettingsOnlyView(t *testing.T) {
	model := newColorPlanModel(nil, nil, nil, LocalZSelection{Infill: true})
	view := model.View()
	if !strings.Contains(view, "Configure full-spectrum printing") || strings.Contains(view, "Original order") {
		t.Fatalf("unexpected settings-only view:\n%s", view)
	}
	if err := validateColorPlan(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	selection := model.selection()
	if len(selection.Colors) != 0 || !selection.LocalZ.Infill {
		t.Fatalf("settings-only selection = %+v", selection)
	}
}

func TestColorPlanModelEditsNonMixedOutputWithoutShowingMixMode(t *testing.T) {
	model := newColorPlanModel(
		[]ColorSourceRow{{Slot: 1, Color: "#FF0000", Used: true, OutputSlot: 2}},
		[]ColorOutputRow{
			{Slot: 1, Color: "#00FFFF", Base: true, ReplacementSlot: 1},
			{Slot: 2, Color: "#FF00FF", Base: true, Editable: true, ReplacementSlot: 2},
			{Slot: 3, Color: "#FFFF00", Base: true, ReplacementSlot: 3},
			{Slot: 4, Color: "#808080", Base: true, ReplacementSlot: 4},
		},
		nil,
		LocalZSelection{},
	)
	if model.focus != colorPlanReplacementFocus {
		t.Fatalf("focus = %d, want replacement", model.focus)
	}
	view := model.View()
	if !strings.Contains(view, "Base") || strings.Contains(view, "Ratio") {
		t.Fatalf("unexpected non-mixed view:\n%s", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(colorPlanModel)
	if result.outputs[1].ReplacementSlot != 3 {
		t.Fatalf("replacement = T%d, want T3", result.outputs[1].ReplacementSlot)
	}
	selection := result.selection()
	if len(selection.Colors) != 1 || selection.Colors[0].Mode != "" || selection.Colors[0].ReplacementSlot != 3 {
		t.Fatalf("selection = %+v", selection.Colors)
	}
}

func TestColorPlanModelUsesCompactColumnsOnNarrowTerminal(t *testing.T) {
	model := testColorPlanModel()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	view := updated.(colorPlanModel).View()
	for _, line := range strings.Split(strings.TrimSuffix(view, "\n"), "\n") {
		if width := visibleWidth(line); width > 70 {
			t.Fatalf("line width = %d, want <= 70: %q", width, line)
		}
	}
}

func testColorPlanModel() colorPlanModel {
	return newColorPlanModel(
		[]ColorSourceRow{
			{Slot: 1, Color: "#5E43B7", Used: true, OutputSlot: 5},
			{Slot: 2, Color: "#5E43B7", Used: false, OutputSlot: 5},
			{Slot: 3, Color: "#00AE42", Used: true, OutputSlot: 6},
		},
		[]ColorOutputRow{
			{Slot: 1, Color: "#00FFFF", Base: true, ReplacementSlot: 1},
			{Slot: 2, Color: "#FF00FF", Base: true, ReplacementSlot: 2},
			{Slot: 3, Color: "#FFFF00", Base: true, ReplacementSlot: 3},
			{Slot: 4, Color: "#808080", Base: true, ReplacementSlot: 4},
			{Slot: 5, Color: "#5B47AB", Mixed: true, Editable: true, Mode: 0, ReplacementSlot: 5},
			{Slot: 6, Color: "#30A45B", Mixed: true, Editable: true, Mode: 0, ReplacementSlot: 6},
		},
		[]MixModeOption{
			{Label: "Ratio", Value: "ratio"},
			{Label: "Cycle", Value: "cycle"},
			{Label: "Match", Value: "match"},
			{Label: "Gradient", Value: "gradient"},
		},
		LocalZSelection{},
	)
}
