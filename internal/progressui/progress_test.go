package progressui

import (
	"context"
	"os"
	"strings"
	"testing"
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
