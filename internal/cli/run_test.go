package cli

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nzlov/btu/internal/progressui"
	"github.com/nzlov/btu/internal/threemf"
)

func TestHelpReturnsSuccess(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	if status := Run([]string{"--help"}, stdout, stderr); status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	help := readOutput(t, stdout)
	for _, text := range []string{
		"--output FILE, -o FILE",
		"--replace, -r",
		"--colors ORDER, -c ORDER",
		"--full-spectrum, -f",
		"--nozzle DIAMETER_MM, -n DIAMETER_MM",
		"allowed: 0.2, 0.4, 0.6, 0.8",
		"--template FILE, -t FILE",
	} {
		if !strings.Contains(help, text) {
			t.Fatalf("help is missing %q:\n%s", text, help)
		}
	}
}

func TestNozzleFlagReachesConversionRequest(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	var request threemf.Request
	status := run([]string{"--nozzle", "0.6", "source.3mf"}, stdout, stderr, func(_ context.Context, got threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		request = got
		return threemf.Report{Mode: "layered", Output: got.Output}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	if request.Nozzle != "0.6" {
		t.Fatalf("nozzle = %q, want 0.6", request.Nozzle)
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
	wantSlots := [4]threemf.ColorRole{threemf.ColorCyan, threemf.ColorMagenta, threemf.ColorYellow, threemf.ColorGray}
	if request.Palette.Slots != wantSlots {
		t.Fatalf("palette = %v, want %v", request.Palette.Slots, wantSlots)
	}
}

func TestFlagsReachConversionRequest(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	var request threemf.Request
	status := run([]string{"-o", "output.3mf", "-r", "-c", "cmyb", "-f", "-n", "0.8", "-t", "custom.3mf", "source.3mf"}, stdout, stderr, func(_ context.Context, got threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		request = got
		return threemf.Report{Mode: "full-spectrum", Output: got.Output}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	if request.Source != "source.3mf" || request.Output != "output.3mf" || request.Template != "custom.3mf" || request.Nozzle != "0.8" || !request.Replace || !request.FullSpectrum {
		t.Fatalf("unexpected request: %+v", request)
	}
	wantSlots := [4]threemf.ColorRole{threemf.ColorCyan, threemf.ColorMagenta, threemf.ColorYellow, threemf.ColorBlack}
	if request.Palette.Slots != wantSlots {
		t.Fatalf("palette = %v, want %v", request.Palette.Slots, wantSlots)
	}
}

func TestExistingOutputConfirmationRetriesWithReplace(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	status := runWithPrompts(
		[]string{"-o", "output.3mf", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			calls++
			if calls == 1 {
				return threemf.Report{}, &threemf.OutputExistsError{Path: request.Output}
			}
			if !request.Replace {
				t.Fatal("retry did not enable replacement")
			}
			return threemf.Report{Mode: "layered", Output: request.Output}, nil
		},
		func(_ context.Context, _ *os.File, prompt string) (bool, error) {
			if !strings.Contains(prompt, "output.3mf") || !strings.Contains(prompt, "Replace") {
				t.Fatalf("prompt = %q", prompt)
			}
			return true, nil
		},
		func(context.Context, *os.File, []progressui.ColorMappingRow, []progressui.ColorOption) ([]string, error) {
			return nil, errors.New("color mapping was not expected")
		},
	)
	if status != 0 || calls != 2 {
		t.Fatalf("status = %d, calls = %d, stderr = %s", status, calls, readOutput(t, stderr))
	}
}

func TestRejectingExistingOutputStopsConversion(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	status := runWithPrompts(
		[]string{"-o", "output.3mf", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			calls++
			return threemf.Report{}, &threemf.OutputExistsError{Path: request.Output}
		},
		func(context.Context, *os.File, string) (bool, error) { return false, nil },
		func(context.Context, *os.File, []progressui.ColorMappingRow, []progressui.ColorOption) ([]string, error) {
			return nil, errors.New("color mapping was not expected")
		},
	)
	if status != 1 || calls != 1 {
		t.Fatalf("status = %d, calls = %d", status, calls)
	}
	if got := readOutput(t, stderr); !strings.Contains(got, "was not replaced") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestReplacementConfirmationCanContinueToColorMapping(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	confirmations := 0
	status := runWithPrompts(
		[]string{"-o", "output.3mf", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			calls++
			switch calls {
			case 1:
				return threemf.Report{}, &threemf.OutputExistsError{Path: request.Output}
			case 2:
				if !request.Replace {
					t.Fatal("replacement was not retained")
				}
				return threemf.Report{}, &threemf.FullSpectrumRequiredError{
					ColorCount:   1,
					NonBaseCount: 1,
					Mappings: []threemf.ColorMapping{
						{MaterialIDs: []int{1}, Color: "#5E43B7", Used: true, Suggested: threemf.ColorCyan},
					},
				}
			default:
				if !request.Replace || !request.FullSpectrum || request.ColorMapping[1] != "#5E43B7" {
					t.Fatalf("final request = %+v", request)
				}
				return threemf.Report{Mode: "full-spectrum", Output: request.Output}, nil
			}
		},
		func(context.Context, *os.File, string) (bool, error) {
			confirmations++
			return true, nil
		},
		func(_ context.Context, _ *os.File, rows []progressui.ColorMappingRow, options []progressui.ColorOption) ([]string, error) {
			return []string{options[rows[0].Selected].Color}, nil
		},
	)
	if status != 0 || calls != 3 || confirmations != 2 {
		t.Fatalf("status = %d, calls = %d, confirmations = %d; stderr = %s", status, calls, confirmations, readOutput(t, stderr))
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
	if got := readOutput(t, stderr); !strings.Contains(got, "slots 1-3 are fixed") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestPrintsPlateNameAndT4ReplacementStep(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	status := run([]string{"source.3mf"}, stdout, stderr, func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		return threemf.Report{
			Mode: "full-spectrum", Output: request.Output, Colors: "cmyg",
			Plates: []threemf.PlateReport{{Number: 2, Name: "body", Colors: "cmyb", Neutral: threemf.ColorBlack}},
		}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	if got := readOutput(t, stdout); !strings.Contains(got, "replace T4 gray -> black (cmyb): plate 2 - body") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestPrintingStepsGroupPlatesToMinimizeT4Changes(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	status := run([]string{"source.3mf"}, stdout, stderr, func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		return threemf.Report{
			Mode: "full-spectrum", Output: request.Output, Colors: "cmyg",
			Plates: []threemf.PlateReport{
				{Number: 1, Name: "eyes", Colors: "cmyw", Neutral: threemf.ColorWhite},
				{Number: 2, Name: "body", Colors: "cmyb", Neutral: threemf.ColorBlack},
				{Number: 3, Name: "ears", Colors: "cmyw", Neutral: threemf.ColorWhite},
				{Number: 4, Name: "nose", Colors: "cmyg", Neutral: threemf.ColorGray},
			},
		}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	got := readOutput(t, stdout)
	want := "Printing steps (recommended order, 2 T4 changes):\n" +
		"  keep T4 gray (cmyg): plate 4 - nose\n" +
		"  replace T4 gray -> white (cmyw): plate 1 - eyes, plate 3 - ears\n" +
		"  replace T4 white -> black (cmyb): plate 2 - body\n"
	if !strings.Contains(got, want) {
		t.Fatalf("stdout = %q, want block %q", got, want)
	}
}

func TestInvalidNozzleIsRejectedBeforeConversion(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	called := false
	status := run([]string{"--nozzle", "0.5", "source.3mf"}, stdout, stderr, func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.Report, error) {
		called = true
		return threemf.Report{}, nil
	})
	if status != 2 || called {
		t.Fatalf("status = %d, called = %v", status, called)
	}
	if got := readOutput(t, stderr); !strings.Contains(got, "unsupported nozzle size") {
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

func TestFullSpectrumConfirmationRetriesConversion(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	confirmed := false
	status := runWithPrompts([]string{"-o", "output.3mf", "source.3mf"}, stdout, stderr, func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		calls++
		if calls == 1 {
			return threemf.Report{}, &threemf.FullSpectrumRequiredError{
				ColorCount:   2,
				NonBaseCount: 1,
				Mappings: []threemf.ColorMapping{
					{MaterialIDs: []int{1}, Color: "#FF0000", Used: true, Base: true, Suggested: threemf.ColorMagenta},
					{MaterialIDs: []int{2, 3}, Color: "#5E43B7", Used: true, Suggested: threemf.ColorCyan},
				},
			}
		}
		if !request.FullSpectrum {
			t.Fatal("retry did not enable full spectrum")
		}
		if request.ColorMapping[1] != "#FF0000" || request.ColorMapping[2] != "#5E43B7" || request.ColorMapping[3] != "#5E43B7" {
			t.Fatalf("color mapping = %v", request.ColorMapping)
		}
		return threemf.Report{Mode: "full-spectrum", Output: request.Output, Colors: "cmyg"}, nil
	}, func(_ context.Context, _ *os.File, prompt string) (bool, error) {
		confirmed = true
		if !strings.Contains(prompt, "2 colors") || !strings.Contains(prompt, "1 need mixing") {
			t.Fatalf("prompt = %q", prompt)
		}
		return true, nil
	}, func(_ context.Context, _ *os.File, rows []progressui.ColorMappingRow, options []progressui.ColorOption) ([]string, error) {
		result := make([]string, len(rows))
		for index, row := range rows {
			result[index] = options[row.Selected].Color
		}
		return result, nil
	})
	if status != 0 || calls != 2 || !confirmed {
		t.Fatalf("status = %d, calls = %d, confirmed = %v; stderr = %s", status, calls, confirmed, readOutput(t, stderr))
	}
	if got := readOutput(t, stdout); !strings.Contains(got, "U1 colors: cmyg") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRejectingAutomaticMixingStillAllowsExplicitColorMapping(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	status := runWithPrompts(
		[]string{"-o", "output.3mf", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			calls++
			if calls == 1 {
				return threemf.Report{}, &threemf.FullSpectrumRequiredError{
					ColorCount:   2,
					NonBaseCount: 1,
					Mappings: []threemf.ColorMapping{
						{MaterialIDs: []int{1}, Color: "#5E43B7", Used: true, Suggested: threemf.ColorCyan},
						{MaterialIDs: []int{2}, Color: "#00AE42", Suggested: threemf.ColorYellow},
					},
				}
			}
			if request.FullSpectrum {
				t.Fatal("rejected automatic full spectrum was enabled")
			}
			if request.ColorMapping[1] != "#5E43B7" || request.ColorMapping[2] != "#5E43B7" {
				t.Fatalf("color mapping = %v", request.ColorMapping)
			}
			return threemf.Report{Mode: "full-spectrum", Output: request.Output, Colors: "cmwb", VirtualMixes: 1}, nil
		},
		func(context.Context, *os.File, string) (bool, error) { return false, nil },
		func(_ context.Context, _ *os.File, rows []progressui.ColorMappingRow, options []progressui.ColorOption) ([]string, error) {
			if options[rows[0].Selected].Color != "#0000FF" || options[rows[1].Selected].Color != "#FFFF00" {
				t.Fatalf("replacement defaults: rows=%+v options=%+v", rows, options)
			}
			return []string{"#5E43B7", "#5E43B7"}, nil
		},
	)
	if status != 0 || calls != 2 {
		t.Fatalf("status = %d, calls = %d, stderr = %s", status, calls, readOutput(t, stderr))
	}
}

func TestColorMappingRowsOfferCMYGWhiteBlackAndDetectedMixes(t *testing.T) {
	rows, options := colorMappingRows([]threemf.ColorMapping{
		{MaterialIDs: []int{5}, Color: "#5E43B7", Used: true, Suggested: threemf.ColorCyan},
	}, true)
	want := []string{"#0000FF", "#FF0000", "#FFFF00", "#808080", "#FFFFFF", "#000000", "#5E43B7"}
	got := make([]string, len(options))
	for index, option := range options {
		got[index] = option.Color
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %v, want %v", got, want)
	}
	if options[rows[0].Selected].Color != "#5E43B7" {
		t.Fatalf("selected option = %+v", options[rows[0].Selected])
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
