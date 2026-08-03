package progressui

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRunFallsBackToPlainProgressForFile(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "progress")
	if err != nil {
		t.Fatal(err)
	}
	value, err := Run(context.Background(), output, func(report func(Progress)) (int, error) {
		report(Progress{Current: 1, Total: 2, Stage: "Analyze"})
		report(Progress{Current: 2, Total: 2, Stage: "Complete"})
		return 42, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatalf("value = %d, want 42", value)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "[1/2] Analyze") || !strings.Contains(got, "[2/2] Complete") {
		t.Fatalf("plain progress = %q", got)
	}
}

func TestConfirmModelAcceptsAndRejects(t *testing.T) {
	tests := []struct {
		key      string
		accepted bool
	}{
		{key: "y", accepted: true},
		{key: "n", accepted: false},
		{key: "enter", accepted: false},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			model := confirmModel{prompt: "Enable?"}
			key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(test.key)}
			if test.key == "enter" {
				key = tea.KeyMsg{Type: tea.KeyEnter}
			}
			updated, command := model.Update(key)
			result := updated.(confirmModel)
			if !result.done || result.accepted != test.accepted || command == nil {
				t.Fatalf("result = %+v, command nil = %v", result, command == nil)
			}
		})
	}
}

func TestConfirmRejectsNonInteractiveFiles(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Confirm(context.Background(), input, output, "Enable?")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("error = %v", err)
	}
}

func TestMappingModelEditsAllRows(t *testing.T) {
	model := mappingModel{
		rows: []ColorMappingRow{
			{Label: "T1", Color: "#5E43B7", Selected: 0},
			{Label: "T2", Color: "#00AE42", Selected: 1},
		},
		options: []ColorOption{
			{Label: "red", Color: "#FF0000"},
			{Label: "purple mix", Color: "#5E43B7"},
		},
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(mappingModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(mappingModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(mappingModel)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(mappingModel)

	if !model.done || command == nil || model.rows[0].Selected != 1 || model.rows[1].Selected != 0 {
		t.Fatalf("unexpected model: %+v, command nil = %v", model, command == nil)
	}
}

func TestMappingModelViewShowsSourceAndTargetColors(t *testing.T) {
	model := mappingModel{
		rows:    []ColorMappingRow{{Label: "T5 (used)", Color: "#5E43B7", Selected: 0}},
		options: []ColorOption{{Label: "purple mix", Color: "#5E43B7"}},
	}
	view := model.View()
	for _, text := range []string{"T5 (used)", "#5E43B7", "purple mix"} {
		if !strings.Contains(view, text) {
			t.Fatalf("view is missing %q: %q", text, view)
		}
	}
}
