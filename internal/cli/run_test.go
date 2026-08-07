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
		"--mix-mode MODE, -m MODE",
		"--[no-]subdivide-layer-height",
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
	if request.MixMode != threemf.MixModeRatio {
		t.Fatalf("mix mode = %q, want ratio", request.MixMode)
	}
}

func TestFlagsReachConversionRequest(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	var request threemf.Request
	status := run([]string{"-o", "output.3mf", "-r", "-c", "bmcy", "-f", "--subdivide-layer-height", "-m", "gradient", "-n", "0.8", "-t", "custom.3mf", "source.3mf"}, stdout, stderr, func(_ context.Context, got threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		request = got
		return threemf.Report{Mode: "full-spectrum", Output: got.Output}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	wantLocalZ := threemf.LocalZSettings{LayerHeight: true, Infill: true, WholeObjects: true}
	if request.Source != "source.3mf" || request.Output != "output.3mf" || request.Template != "custom.3mf" || request.Nozzle != "0.8" || !request.Replace || !request.FullSpectrum || request.LocalZ != wantLocalZ || request.MixMode != threemf.MixModeGradient {
		t.Fatalf("unexpected request: %+v", request)
	}
	wantSlots := [4]threemf.ColorRole{threemf.ColorBlack, threemf.ColorMagenta, threemf.ColorCyan, threemf.ColorYellow}
	if request.Palette.Slots != wantSlots {
		t.Fatalf("palette = %v, want %v", request.Palette.Slots, wantSlots)
	}
}

func TestFullSpectrumConfiguresIndependentLocalZSettingsInTUI(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	want := progressui.LocalZSelection{LayerHeight: true, WholeObjects: true}
	reviewed := false
	status := runWithPrompts(
		[]string{"--full-spectrum", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			if request.LocalZ != (threemf.LocalZSettings{LayerHeight: true, WholeObjects: true}) {
				t.Fatalf("local Z = %+v", request.LocalZ)
			}
			return threemf.Report{Mode: "full-spectrum", Output: request.Output}, nil
		},
		func(context.Context, *os.File, string) (bool, error) {
			return false, errors.New("confirmation was not expected")
		},
		func(_ context.Context, _ *os.File, sources []progressui.ColorSourceRow, outputs []progressui.ColorOutputRow, options []progressui.MixModeOption, localZ progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
			reviewed = true
			if len(sources) != 0 || len(outputs) != 0 || len(options) != 0 || localZ != (progressui.LocalZSelection{}) {
				t.Fatalf("settings-only review received unexpected plan: %v %v %v %+v", sources, outputs, options, localZ)
			}
			return progressui.ColorPlanResult{LocalZ: want}, nil
		},
		unexpectedColorPlanPreview,
	)
	if status != 0 || !reviewed {
		t.Fatalf("status = %d, reviewed = %v; stderr = %s", status, reviewed, readOutput(t, stderr))
	}
}

func TestNoSubdivideLayerHeightFlagSkipsSettingsTUI(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	status := runWithPrompts(
		[]string{"--full-spectrum", "--no-subdivide-layer-height", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			if request.LocalZ != (threemf.LocalZSettings{}) {
				t.Fatalf("local Z was enabled: %+v", request.LocalZ)
			}
			return threemf.Report{Mode: "full-spectrum", Output: request.Output}, nil
		},
		func(context.Context, *os.File, string) (bool, error) {
			return false, errors.New("confirmation was not expected")
		},
		unexpectedColorPlanReview,
		unexpectedColorPlanPreview,
	)
	if status != 0 {
		t.Fatalf("status = %d; stderr = %s", status, readOutput(t, stderr))
	}
}

func TestNonFullSpectrumConversionReviewsBaseColorPlan(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	previews := 0
	reviews := 0
	conversions := 0
	sequence := threemf.ColorSequence{
		Source: []threemf.SourceColorSlot{{Slot: 1, Color: "#FF0000", Used: true, OutputSlot: 2}},
		Output: []threemf.OutputColorSlot{
			{Slot: 1, Color: "#00FFFF", Base: true},
			{Slot: 2, Color: "#FF00FF", Base: true, Editable: true, MaterialIDs: []int{1}},
			{Slot: 3, Color: "#FFFF00", Base: true},
			{Slot: 4, Color: "#808080", Base: true},
		},
	}
	status := runWithPrompts(
		[]string{"source.3mf"}, stdout, stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			conversions++
			want := map[int]threemf.MaterialReplacement{1: {BaseSlot: 3}}
			if request.FullSpectrum || request.PreserveMaterialSlots || !reflect.DeepEqual(request.MaterialReplacements, want) {
				t.Fatalf("conversion request = %+v", request)
			}
			return threemf.Report{Mode: "layered", Output: request.Output}, nil
		},
		func(context.Context, *os.File, string) (bool, error) {
			return false, errors.New("confirmation was not expected")
		},
		func(_ context.Context, _ *os.File, sources []progressui.ColorSourceRow, outputs []progressui.ColorOutputRow, options []progressui.MixModeOption, _ progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
			reviews++
			if len(sources) != 1 || len(outputs) != 4 || !outputs[1].Editable || outputs[1].Mixed || len(options) != 0 {
				t.Fatalf("unexpected base-color review: %v %v %v", sources, outputs, options)
			}
			return progressui.ColorPlanResult{Colors: []progressui.ColorPlanSelection{{OutputSlot: 2, ReplacementSlot: 3}}}, nil
		},
		func(_ context.Context, _ threemf.Request, progress threemf.ProgressFunc) (threemf.ColorSequence, error) {
			previews++
			progress(threemf.Progress{Current: 0, Total: 3, Stage: "Open source"})
			progress(threemf.Progress{Current: 3, Total: 3, Stage: "Color plan ready"})
			return sequence, nil
		},
	)
	if status != 0 || previews != 1 || reviews != 1 || conversions != 1 {
		t.Fatalf("status = %d, previews = %d, reviews = %d, conversions = %d; stderr = %s", status, previews, reviews, conversions, readOutput(t, stderr))
	}
	if got := readOutput(t, stderr); !strings.Contains(got, "[0/3] Open source") || !strings.Contains(got, "[3/3] Color plan ready") {
		t.Fatalf("preview progress = %q", got)
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
		unexpectedColorPlanReview,
		emptyColorPlanPreview,
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
		unexpectedColorPlanReview,
		emptyColorPlanPreview,
	)
	if status != 1 || calls != 1 {
		t.Fatalf("status = %d, calls = %d", status, calls)
	}
	if got := readOutput(t, stderr); !strings.Contains(got, "was not replaced") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestFullSpectrumChoiceCanContinueToReplacementConfirmation(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	confirmations := 0
	reviews := 0
	status := runWithPrompts(
		[]string{"-o", "output.3mf", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			calls++
			if calls == 1 {
				return threemf.Report{}, &threemf.OutputExistsError{Path: request.Output}
			}
			if !request.Replace || !request.FullSpectrum || request.PreserveMaterialSlots {
				t.Fatalf("final request = %+v", request)
			}
			if !request.LocalZ.Infill || request.LocalZ.LayerHeight || request.LocalZ.WholeObjects {
				t.Fatalf("final local Z = %+v", request.LocalZ)
			}
			return threemf.Report{Mode: "full-spectrum", Output: request.Output}, nil
		},
		func(context.Context, *os.File, string) (bool, error) {
			confirmations++
			return true, nil
		},
		func(_ context.Context, _ *os.File, sources []progressui.ColorSourceRow, outputs []progressui.ColorOutputRow, options []progressui.MixModeOption, _ progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
			reviews++
			if len(sources) != 1 || len(outputs) != 5 || len(options) != 4 {
				t.Fatalf("unexpected color review: %v %v %v", sources, outputs, options)
			}
			return progressui.ColorPlanResult{
				Colors: []progressui.ColorPlanSelection{{OutputSlot: 5, Mode: "ratio", ReplacementSlot: 5}},
				LocalZ: progressui.LocalZSelection{Infill: true},
			}, nil
		},
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.ColorSequence, error) {
			if request.FullSpectrum {
				return testMixedColorSequence(), nil
			}
			return threemf.ColorSequence{}, &threemf.FullSpectrumRequiredError{
				ColorCount: 1, NonBaseCount: 1, Sequence: testMixedColorSequence(),
			}
		},
	)
	if status != 0 || calls != 2 || confirmations != 2 || reviews != 1 {
		t.Fatalf("status = %d, calls = %d, confirmations = %d, reviews = %d; stderr = %s", status, calls, confirmations, reviews, readOutput(t, stderr))
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

func TestInvalidMixModeIsRejectedBeforeConversion(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	called := false
	status := run([]string{"--mix-mode", "blend", "source.3mf"}, stdout, stderr, func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.Report, error) {
		called = true
		return threemf.Report{}, nil
	})
	if status != 2 || called {
		t.Fatalf("status = %d, called = %v", status, called)
	}
	if got := readOutput(t, stderr); !strings.Contains(got, "allowed: ratio, cycle, match, gradient") {
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

func TestPrintsAnalyzedRequiredColorOrder(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	status := run([]string{"--colors", "cmyg", "source.3mf"}, stdout, stderr, func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		return threemf.Report{Mode: "full-spectrum", Output: request.Output, Colors: "cmwb"}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	if got := readOutput(t, stdout); !strings.Contains(got, "Required U1 colors: cmwb") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestPrintsReplacementStepForCustomNeutralSlot(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	status := run([]string{"--colors", "bmcy", "source.3mf"}, stdout, stderr, func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		return threemf.Report{
			Mode: "full-spectrum", Output: request.Output, Colors: "bmcy",
			Plates: []threemf.PlateReport{{Number: 2, Name: "body", Colors: "wmcy", Neutral: threemf.ColorWhite}},
		}, nil
	})
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, readOutput(t, stderr))
	}
	if got := readOutput(t, stdout); !strings.Contains(got, "replace T1 black -> white (wmcy): plate 2 - body") {
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

func TestFullSpectrumConfirmationContinuesToColorTUI(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	confirmations := 0
	reviews := 0
	status := runWithPrompts([]string{"-o", "output.3mf", "source.3mf"}, stdout, stderr, func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
		calls++
		if !request.FullSpectrum || request.PreserveMaterialSlots {
			t.Fatalf("conversion request = %+v", request)
		}
		if !request.LocalZ.WholeObjects || request.LocalZ.LayerHeight || request.LocalZ.Infill {
			t.Fatalf("retry local Z = %+v", request.LocalZ)
		}
		return threemf.Report{Mode: "full-spectrum", Output: request.Output, Colors: "cmyg"}, nil
	}, func(_ context.Context, _ *os.File, prompt string) (bool, error) {
		confirmations++
		if confirmations == 1 && (!strings.Contains(prompt, "2 colors") || !strings.Contains(prompt, "1 need mixing")) {
			t.Fatalf("prompt = %q", prompt)
		}
		return true, nil
	}, func(_ context.Context, _ *os.File, sources []progressui.ColorSourceRow, outputs []progressui.ColorOutputRow, options []progressui.MixModeOption, _ progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
		reviews++
		if len(sources) != 1 || len(outputs) != 5 || len(options) != 4 {
			t.Fatalf("unexpected color review: %v %v %v", sources, outputs, options)
		}
		return progressui.ColorPlanResult{
			Colors: []progressui.ColorPlanSelection{{OutputSlot: 5, Mode: "ratio", ReplacementSlot: 5}},
			LocalZ: progressui.LocalZSelection{WholeObjects: true},
		}, nil
	}, func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.ColorSequence, error) {
		return threemf.ColorSequence{}, &threemf.FullSpectrumRequiredError{
			ColorCount: 2, NonBaseCount: 1, Sequence: testMixedColorSequence(),
		}
	})
	if status != 0 || calls != 1 || confirmations != 1 || reviews != 1 {
		t.Fatalf("status = %d, calls = %d, confirmations = %d, reviews = %d; stderr = %s", status, calls, confirmations, reviews, readOutput(t, stderr))
	}
	if got := readOutput(t, stdout); !strings.Contains(got, "Required U1 colors: cmyg") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestColorPlanSelectionOverridesMixedOutputGroups(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	status := runWithPrompts(
		[]string{"-o", "output.3mf", "--mix-mode", "match", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			calls++
			if !request.FullSpectrum || request.MixMode != threemf.MixModeMatch {
				t.Fatalf("conversion request = %+v", request)
			}
			if !request.LocalZ.LayerHeight || request.LocalZ.Infill || !request.LocalZ.WholeObjects {
				t.Fatalf("retry local Z = %+v", request.LocalZ)
			}
			want := map[int]threemf.MixMode{1: threemf.MixModeCycle, 2: threemf.MixModeGradient, 3: threemf.MixModeCycle}
			if !reflect.DeepEqual(request.MaterialMixModes, want) {
				t.Fatalf("material modes = %v, want %v", request.MaterialMixModes, want)
			}
			wantReplacements := map[int]threemf.MaterialReplacement{
				1: {BaseSlot: 1},
				2: {SourceMaterial: 1},
				3: {BaseSlot: 1},
			}
			if !reflect.DeepEqual(request.MaterialReplacements, wantReplacements) {
				t.Fatalf("material replacements = %v, want %v", request.MaterialReplacements, wantReplacements)
			}
			return threemf.Report{Mode: "full-spectrum", Output: request.Output}, nil
		},
		func(context.Context, *os.File, string) (bool, error) { return true, nil },
		func(_ context.Context, _ *os.File, sources []progressui.ColorSourceRow, outputs []progressui.ColorOutputRow, options []progressui.MixModeOption, localZ progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
			if len(sources) != 3 || sources[0].Color != "#5E43B7" || sources[2].OutputSlot != 5 {
				t.Fatalf("sources = %+v", sources)
			}
			if len(outputs) != 6 || outputs[4].Mode != 2 {
				t.Fatalf("outputs = %+v", outputs)
			}
			if len(options) != 4 || options[3].Value != "gradient" {
				t.Fatalf("options = %+v", options)
			}
			if localZ != (progressui.LocalZSelection{}) {
				t.Fatalf("initial local Z = %+v", localZ)
			}
			return progressui.ColorPlanResult{
				Colors: []progressui.ColorPlanSelection{
					{OutputSlot: 5, Mode: "cycle", ReplacementSlot: 1},
					{OutputSlot: 6, Mode: "gradient", ReplacementSlot: 5},
				},
				LocalZ: progressui.LocalZSelection{LayerHeight: true, WholeObjects: true},
			}, nil
		},
		func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.ColorSequence, error) {
			sequence := threemf.ColorSequence{
				Source: []threemf.SourceColorSlot{
					{Slot: 1, Color: "#5E43B7", Used: true, OutputSlot: 5},
					{Slot: 2, Color: "#00AE42", OutputSlot: 6},
					{Slot: 3, Color: "#5E43B7", Used: true, OutputSlot: 5},
				},
				Output: []threemf.OutputColorSlot{
					{Slot: 1, Color: "#00FFFF", Base: true},
					{Slot: 2, Color: "#FF00FF", Base: true},
					{Slot: 3, Color: "#FFFF00", Base: true},
					{Slot: 4, Color: "#808080", Base: true},
					{Slot: 5, Color: "#5B47AB", Mixed: true, Editable: true, Mode: threemf.MixModeMatch, MaterialIDs: []int{1, 3}},
					{Slot: 6, Color: "#30A45B", Mixed: true, Editable: true, Mode: threemf.MixModeMatch, MaterialIDs: []int{2}},
				},
			}
			return threemf.ColorSequence{}, &threemf.FullSpectrumRequiredError{ColorCount: 2, NonBaseCount: 2, Sequence: sequence}
		},
	)
	if status != 0 || calls != 1 {
		t.Fatalf("status = %d, calls = %d, stderr = %s", status, calls, readOutput(t, stderr))
	}
}

func TestRejectingAutomaticMixingPreservesAllMaterialSlots(t *testing.T) {
	stdout, stderr := tempOutputs(t)
	calls := 0
	previews := 0
	reviews := 0
	status := runWithPrompts(
		[]string{"-o", "output.3mf", "-c", "bmcy", "source.3mf"},
		stdout,
		stderr,
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.Report, error) {
			calls++
			if request.FullSpectrum || !request.PreserveMaterialSlots {
				t.Fatalf("conversion request = %+v", request)
			}
			wantPalette, err := threemf.ParsePalette("bmcy")
			if err != nil {
				t.Fatal(err)
			}
			if request.Palette != wantPalette {
				t.Fatalf("retry palette = %v, want %v", request.Palette, wantPalette)
			}
			return threemf.Report{Mode: "material-slots", Output: request.Output, Colors: "#000000,#FF0000,#0000FF,#FFFF00,#FCE300,#FB0207,#161616,#FFFFFF,#5E43B7,#00AE42"}, nil
		},
		func(context.Context, *os.File, string) (bool, error) { return false, nil },
		func(_ context.Context, _ *os.File, sources []progressui.ColorSourceRow, outputs []progressui.ColorOutputRow, _ []progressui.MixModeOption, _ progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
			reviews++
			if len(sources) != 2 || len(outputs) != 6 || outputs[4].Mixed || !outputs[4].Editable {
				t.Fatalf("unexpected preserved review: %v %v", sources, outputs)
			}
			return progressui.ColorPlanResult{Colors: []progressui.ColorPlanSelection{
				{OutputSlot: 5, ReplacementSlot: 5},
				{OutputSlot: 6, ReplacementSlot: 6},
			}}, nil
		},
		func(_ context.Context, request threemf.Request, _ threemf.ProgressFunc) (threemf.ColorSequence, error) {
			previews++
			if !request.PreserveMaterialSlots {
				return threemf.ColorSequence{}, &threemf.FullSpectrumRequiredError{ColorCount: 6, NonBaseCount: 2}
			}
			return testPreservedColorSequence(), nil
		},
	)
	if status != 0 || calls != 1 || previews != 2 || reviews != 1 {
		t.Fatalf("status = %d, calls = %d, previews = %d, reviews = %d, stderr = %s", status, calls, previews, reviews, readOutput(t, stderr))
	}
	if got := readOutput(t, stdout); !strings.Contains(got, "Required U1 colors: #000000,#FF0000,#0000FF,#FFFF00,#FCE300,#FB0207,#161616,#FFFFFF,#5E43B7,#00AE42") {
		t.Fatalf("stdout = %q", got)
	}
}

func unexpectedColorPlanReview(context.Context, *os.File, []progressui.ColorSourceRow, []progressui.ColorOutputRow, []progressui.MixModeOption, progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
	return progressui.ColorPlanResult{}, errors.New("color plan review was not expected")
}

func unexpectedColorPlanPreview(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.ColorSequence, error) {
	return threemf.ColorSequence{}, errors.New("color plan preview was not expected")
}

func emptyColorPlanPreview(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.ColorSequence, error) {
	return threemf.ColorSequence{}, nil
}

func testMixedColorSequence() threemf.ColorSequence {
	return threemf.ColorSequence{
		Source: []threemf.SourceColorSlot{{Slot: 1, Color: "#5E43B7", Used: true, OutputSlot: 5}},
		Output: []threemf.OutputColorSlot{
			{Slot: 1, Color: "#00FFFF", Base: true},
			{Slot: 2, Color: "#FF00FF", Base: true},
			{Slot: 3, Color: "#FFFF00", Base: true},
			{Slot: 4, Color: "#808080", Base: true},
			{Slot: 5, Color: "#5B47AB", Mixed: true, Editable: true, Mode: threemf.MixModeRatio, MaterialIDs: []int{1}},
		},
	}
}

func testPreservedColorSequence() threemf.ColorSequence {
	return threemf.ColorSequence{
		Source: []threemf.SourceColorSlot{
			{Slot: 1, Color: "#5E43B7", Used: true, OutputSlot: 5},
			{Slot: 2, Color: "#00AE42", Used: true, OutputSlot: 6},
		},
		Output: []threemf.OutputColorSlot{
			{Slot: 1, Color: "#000000", Base: true},
			{Slot: 2, Color: "#FF0000", Base: true},
			{Slot: 3, Color: "#00FFFF", Base: true},
			{Slot: 4, Color: "#FFFF00", Base: true},
			{Slot: 5, Color: "#5E43B7", Editable: true, MaterialIDs: []int{1}},
			{Slot: 6, Color: "#00AE42", Editable: true, MaterialIDs: []int{2}},
		},
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
