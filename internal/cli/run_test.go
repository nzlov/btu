package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nzlov/btu/internal/threemf"
)

func TestHelpReturnsSuccess(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	if status := Run([]string{"--help"}, stdout, stderr); status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	help := readOutput(t, stdout)
	for _, text := range []string{"--output FILE, -o FILE", "--colors WRBYK, -c WRBYK", "--full-spectrum, -f", "--template FILE, -t FILE"} {
		if !strings.Contains(help, text) {
			t.Fatalf("help is missing %q:\n%s", text, help)
		}
	}
}

func TestOutputDefaultsNextToSource(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	var request threemf.Request
	status := run([]string{"/tmp/models/source.3mf"}, stdout, stderr, func(_ context.Context, got threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		request = got
		return threemf.Report{Mode: "layered", Output: got.Output}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, want 0; stderr = %s", status, readOutput(t, stderr))
	}
	if request.Output != "/tmp/models/source-btu.3mf" {
		t.Fatalf("output = %q", request.Output)
	}
}

func TestFlagsReachConversionRequest(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	var request threemf.Request
	status := run([]string{"-o", "output.3mf", "-c", "kryb", "-f", "-t", "custom.3mf", "source.3mf"}, stdout, stderr, func(_ context.Context, got threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		request = got
		return threemf.Report{Mode: "full-spectrum", Output: got.Output}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	if request.Source != "source.3mf" || request.Output != "output.3mf" || request.Template != "custom.3mf" || !request.FullSpectrum {
		t.Fatalf("unexpected request: %+v", request)
	}
	wantSlots := [4]threemf.ColorRole{threemf.ColorBlack, threemf.ColorRed, threemf.ColorYellow, threemf.ColorBlue}
	if request.Palette.Slots != wantSlots {
		t.Fatalf("palette = %v, want %v", request.Palette.Slots, wantSlots)
	}
}

func TestInvalidColorsAreRejectedBeforeConversion(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	called := false
	status := run([]string{"-o", "output.3mf", "-c", "wwyb", "source.3mf"}, stdout, stderr, func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.Report, error) {
		called = true
		return threemf.Report{}, nil
	})
	if status != 2 || called {
		t.Fatalf("status = %d, called = %v", status, called)
	}
	if got := readOutput(t, stderr); !strings.Contains(got, "appears more than once") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestConversionFailureReturnsOne(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	status := run([]string{"-o", "output.3mf", "source.3mf"}, stdout, stderr, func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.Report, error) {
		return threemf.Report{}, errors.New("conversion failed")
	})
	if status != 1 {
		t.Fatalf("status = %d, want 1", status)
	}
	if got := readOutput(t, stderr); !strings.Contains(got, "conversion failed") {
		t.Fatalf("stderr = %q", got)
	}
}

func tempOutputs(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdout.Close() })
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stderr.Close() })
	return stdout, stderr
}

func readOutput(t *testing.T, file *os.File) string {
	t.Helper()
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
