package progressui

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nzlov/btu/internal/i18n"
)

func TestRunFallsBackToPlainProgressForFile(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "progress")
	if err != nil {
		t.Fatal(err)
	}
	value, err := Run(context.Background(), output, i18n.EnglishLocalizer(), func(report func(Progress)) (int, error) {
		report(Progress{Current: 1, Total: 2, Stage: "Analyze", Detail: "a.model", ItemCurrent: 1, ItemTotal: 2})
		report(Progress{Current: 1, Total: 2, Stage: "Analyze", Detail: "b.model", ItemCurrent: 2, ItemTotal: 2})
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
	if !strings.Contains(got, "[1/2] Analyze [1/2]: a.model") || !strings.Contains(got, "[1/2] Analyze [2/2]: b.model") || !strings.Contains(got, "[2/2] Complete") {
		t.Fatalf("plain progress = %q", got)
	}
}

func TestProgressModelShowsExactItemProgressWithinTerminalWidth(t *testing.T) {
	model := model[int]{
		localizer: i18n.EnglishLocalizer(),
		width:     48,
		progress: Progress{
			Current: 3, Total: 6, Stage: "Rewrite 3MF package",
			ItemCurrent: 7, ItemTotal: 10, Detail: "Metadata/very-long-model-member-name.model",
		},
	}
	view := model.View()
	if !strings.Contains(view, "50%  Rewrite 3MF package") || !strings.Contains(view, "7/10 (70%)") {
		t.Fatalf("progress view = %q", view)
	}
	for _, line := range strings.Split(strings.TrimSuffix(view, "\n"), "\n") {
		if len([]rune(line)) > model.width {
			t.Fatalf("line width = %d, want <= %d: %q", len([]rune(line)), model.width, line)
		}
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
	_, err = Confirm(context.Background(), input, output, i18n.EnglishLocalizer(), "Enable?")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("error = %v", err)
	}
}
