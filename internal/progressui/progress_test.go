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
	if err == nil || !strings.Contains(err.Error(), "--full-spectrum") {
		t.Fatalf("error = %v", err)
	}
}
