package progressui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMixModeModelShowsSourceSwatchesAndChangesOneRow(t *testing.T) {
	model := mixModeModel{
		rows: []MixModeRow{
			{Label: "T1 (used)", Color: "#5E43B7", Selected: 0},
			{Label: "T2 (unused)", Color: "#00AE42", Selected: 0},
		},
		options: []MixModeOption{
			{Label: "Ratio", Value: "ratio"},
			{Label: "Cycle", Value: "cycle"},
			{Label: "Match", Value: "match"},
			{Label: "Gradient", Value: "gradient"},
		},
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(mixModeModel)
	if result.rows[0].Selected != 1 || result.rows[1].Selected != 0 {
		t.Fatalf("selections = %d,%d, want 1,0", result.rows[0].Selected, result.rows[1].Selected)
	}
	view := result.View()
	for _, want := range []string{"T1 (used)", "#5E43B7", "Cycle", "T2 (unused)", "#00AE42", "Ratio", "\x1b[48;2;94;67;183m  \x1b[0m"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
}
