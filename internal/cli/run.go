package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	urfavecli "github.com/urfave/cli/v3"

	"github.com/nzlov/btu/internal/i18n"
	"github.com/nzlov/btu/internal/progressui"
	"github.com/nzlov/btu/internal/threemf"
)

type convertFunc func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.Report, error)
type previewColorPlanFunc func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.ColorSequence, error)
type confirmFunc func(context.Context, *os.File, string) (bool, error)
type selectColorPlanFunc func(context.Context, *os.File, []progressui.ColorSourceRow, []progressui.ColorOutputRow, []progressui.MixModeOption, progressui.LocalZSelection) (progressui.ColorPlanResult, error)

func Run(args []string, stdout, stderr *os.File) int {
	localizer := i18n.FromLANG(os.Getenv("LANG"))
	return runWithPromptsLocalized(
		args,
		stdout,
		stderr,
		localizer,
		threemf.Convert,
		func(ctx context.Context, output *os.File, prompt string) (bool, error) {
			return progressui.Confirm(ctx, os.Stdin, output, localizer, prompt)
		},
		func(ctx context.Context, output *os.File, sources []progressui.ColorSourceRow, colors []progressui.ColorOutputRow, options []progressui.MixModeOption, localZ progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
			return progressui.SelectColorPlan(ctx, os.Stdin, output, localizer, sources, colors, options, localZ)
		},
		threemf.PreviewColorPlan,
	)
}

func run(args []string, stdout, stderr *os.File, convert convertFunc) int {
	return runWithPrompts(
		args,
		stdout,
		stderr,
		convert,
		func(context.Context, *os.File, string) (bool, error) {
			return false, errors.New("confirmation was not expected")
		},
		func(context.Context, *os.File, []progressui.ColorSourceRow, []progressui.ColorOutputRow, []progressui.MixModeOption, progressui.LocalZSelection) (progressui.ColorPlanResult, error) {
			return progressui.ColorPlanResult{}, nil
		},
		func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.ColorSequence, error) {
			return threemf.ColorSequence{}, nil
		},
	)
}

// GLUE: keeps Bubble Tea prompts injectable without leaking terminal concerns into conversion.
func runWithPrompts(args []string, stdout, stderr *os.File, convert convertFunc, confirm confirmFunc, selectColorPlan selectColorPlanFunc, previewColorPlan previewColorPlanFunc) int {
	return runWithPromptsLocalized(args, stdout, stderr, i18n.FromLANG(os.Getenv("LANG")), convert, confirm, selectColorPlan, previewColorPlan)
}

func runWithPromptsLocalized(args []string, stdout, stderr *os.File, localizer i18n.Localizer, convert convertFunc, confirm confirmFunc, selectColorPlan selectColorPlanFunc, previewColorPlan previewColorPlanFunc) int {
	command := newCommand(stdout, stderr, localizer, convert, confirm, selectColorPlan, previewColorPlan)
	osArgs := append([]string{"btu"}, args...)
	if err := command.Run(context.Background(), osArgs); err != nil {
		fmt.Fprintf(stderr, "btu: %v\n", err)
		var exitCoder urfavecli.ExitCoder
		if errors.As(err, &exitCoder) {
			return exitCoder.ExitCode()
		}
		return 2
	}
	return 0
}

func newCommand(stdout, stderr *os.File, localizer i18n.Localizer, convert convertFunc, confirm confirmFunc, selectColorPlan selectColorPlanFunc, previewColorPlan previewColorPlanFunc) *urfavecli.Command {
	nozzleFlag := &urfavecli.StringFlag{
		Name:     "nozzle",
		Aliases:  []string{"n"},
		Usage:    localizer.Text(i18n.FlagNozzleUsage),
		OnlyOnce: true,
		Validator: func(value string) error {
			_, err := threemf.ParseNozzleSize(value)
			return localizedNozzleError(localizer, value, err)
		},
	}
	templateFlag := &urfavecli.StringFlag{
		Name:      "template",
		Aliases:   []string{"t"},
		Usage:     localizer.Text(i18n.FlagTemplateUsage),
		OnlyOnce:  true,
		TakesFile: true,
	}
	mixModeFlag := &urfavecli.StringFlag{
		Name:             "mix-mode",
		Aliases:          []string{"m"},
		Usage:            localizer.Text(i18n.FlagMixModeUsage),
		Value:            threemf.MixModeRatio.String(),
		OnlyOnce:         true,
		ValidateDefaults: true,
		HideDefault:      localizer.IsChinese(),
		Validator: func(value string) error {
			_, err := threemf.ParseMixMode(value)
			return localizedMixModeError(localizer, value, err)
		},
	}
	subdivideLayerHeightFlag := &urfavecli.BoolWithInverseFlag{
		Name:        "subdivide-layer-height",
		Usage:       localizer.Text(i18n.FlagSubdivideLayerHeightUsage),
		OnlyOnce:    true,
		HideDefault: localizer.IsChinese(),
	}
	command := &urfavecli.Command{
		Name:      "btu",
		Usage:     localizer.Text(i18n.AppUsage),
		ArgsUsage: "SOURCE.3mf",
		Writer:    stdout,
		ErrWriter: stderr,
		Suggest:   true,
		ExitErrHandler: func(context.Context, *urfavecli.Command, error) {
			// Run returns errors so main and tests can choose the process exit code.
		},
		Flags: []urfavecli.Flag{
			&urfavecli.StringFlag{
				Name:      "output",
				Aliases:   []string{"o"},
				Usage:     localizer.Text(i18n.FlagOutputUsage),
				OnlyOnce:  true,
				TakesFile: true,
			},
			&urfavecli.BoolFlag{
				Name:     "replace",
				Aliases:  []string{"r"},
				Usage:    localizer.Text(i18n.FlagReplaceUsage),
				OnlyOnce: true,
			},
			&urfavecli.StringFlag{
				Name:             "colors",
				Aliases:          []string{"c"},
				Usage:            localizer.Text(i18n.FlagColorsUsage),
				Value:            "cmyg",
				OnlyOnce:         true,
				ValidateDefaults: true,
				HideDefault:      localizer.IsChinese(),
				Validator: func(value string) error {
					_, err := threemf.ParsePalette(value)
					return localizedPaletteError(localizer, value, err)
				},
			},
			&urfavecli.BoolFlag{
				Name:     "full-spectrum",
				Aliases:  []string{"f"},
				Usage:    localizer.Text(i18n.FlagFullSpectrumUsage),
				OnlyOnce: true,
			},
			mixModeFlag,
			subdivideLayerHeightFlag,
			nozzleFlag,
			templateFlag,
		},
		Action: func(ctx context.Context, command *urfavecli.Command) error {
			if command.NArg() != 1 {
				return urfavecli.Exit(localizer.Text(i18n.ErrorExpectedSource), 2)
			}
			palette, err := threemf.ParsePalette(command.String("colors"))
			if err != nil {
				return urfavecli.Exit(err, 2)
			}
			mixMode, err := threemf.ParseMixMode(command.String("mix-mode"))
			if err != nil {
				return urfavecli.Exit(err, 2)
			}
			source := command.Args().First()
			output := command.String("output")
			if output == "" {
				output = defaultOutputPath(source)
			}
			request := threemf.Request{
				Source:       source,
				Template:     command.String("template"),
				Output:       output,
				Replace:      command.Bool("replace"),
				Nozzle:       command.String("nozzle"),
				Palette:      palette,
				FullSpectrum: command.Bool("full-spectrum"),
				MixMode:      mixMode,
			}
			localZSelected := command.IsSet("subdivide-layer-height")
			if localZSelected {
				enabled := command.Bool("subdivide-layer-height")
				request.LocalZ = threemf.LocalZSettings{
					LayerHeight: enabled, Infill: enabled, WholeObjects: enabled,
				}
			}
			convertRequest := func() (threemf.Report, error) {
				return progressui.Run(ctx, stderr, localizer, func(progress func(progressui.Progress)) (threemf.Report, error) {
					return convert(ctx, request, func(event threemf.Progress) {
						progress(localizedProgress(localizer, event))
					})
				})
			}
			previewRequest := func() (threemf.ColorSequence, error) {
				return progressui.Run(ctx, stderr, localizer, func(progress func(progressui.Progress)) (threemf.ColorSequence, error) {
					return previewColorPlan(ctx, request, func(event threemf.Progress) {
						progress(localizedProgress(localizer, event))
					})
				})
			}
			reviewSequence := func(sequence threemf.ColorSequence) error {
				if len(sequence.Output) == 0 {
					return nil
				}
				sources, outputs, options := colorPlanRows(localizer, sequence, request.MixMode)
				selected, selectionErr := selectColorPlan(ctx, stderr, sources, outputs, options, localZSelection(request.LocalZ))
				if selectionErr != nil {
					return fmt.Errorf("%w; %s", selectionErr, localizer.Text(i18n.ErrorColorPlanInteractiveRequired))
				}
				request.LocalZ = localZSettings(selected.LocalZ)
				localZSelected = true
				return applyColorPlanSelection(&request, sequence, selected)
			}

			if request.FullSpectrum {
				if !localZSelected {
					selection, selectionErr := selectColorPlan(ctx, stderr, nil, nil, nil, localZSelection(request.LocalZ))
					if selectionErr != nil {
						return urfavecli.Exit(fmt.Errorf("%w; %s", selectionErr, localizer.Text(i18n.ErrorRerunLocalZ)), 1)
					}
					request.LocalZ = localZSettings(selection.LocalZ)
					localZSelected = true
				}
			} else {
				sequence, previewErr := previewRequest()
				var required *threemf.FullSpectrumRequiredError
				if errors.As(previewErr, &required) {
					accepted, confirmErr := confirm(ctx, stderr, localizer.Format(i18n.PromptGenerateFullSpectrum, required.ColorCount, required.NonBaseCount))
					if confirmErr != nil {
						return urfavecli.Exit(fmt.Errorf("%w; %s", confirmErr, localizer.Text(i18n.ErrorRerunFullSpectrum)), 1)
					}
					if accepted {
						request.FullSpectrum = true
						sequence = required.Sequence
						previewErr = nil
					} else {
						request.PreserveMaterialSlots = true
						sequence, previewErr = previewRequest()
					}
				} else if previewErr != nil {
					return urfavecli.Exit(localizer.Wrap(i18n.ErrorPreviewFailed, previewErr), 1)
				}
				if request.FullSpectrum && len(sequence.Output) == 0 {
					sequence, previewErr = previewRequest()
				}
				if previewErr != nil {
					return urfavecli.Exit(localizer.Wrap(i18n.ErrorPreviewFailed, previewErr), 1)
				}
				if reviewErr := reviewSequence(sequence); reviewErr != nil {
					return urfavecli.Exit(localizer.Wrap(i18n.ErrorColorPlanReviewFailed, reviewErr), 1)
				}
			}

			var report threemf.Report
			for {
				report, err = convertRequest()
				if err == nil {
					break
				}

				var outputExists *threemf.OutputExistsError
				if errors.As(err, &outputExists) && !request.Replace {
					accepted, confirmErr := confirm(ctx, stderr, localizer.Format(i18n.PromptReplaceOutput, outputExists.Path))
					if confirmErr != nil {
						return urfavecli.Exit(fmt.Errorf("%w; %s", confirmErr, localizer.Text(i18n.ErrorRerunReplace)), 1)
					}
					if !accepted {
						return urfavecli.Exit(fmt.Errorf(localizer.Text(i18n.ErrorOutputNotReplaced), outputExists.Path), 1)
					}
					request.Replace = true
					continue
				}
				return urfavecli.Exit(localizer.Wrap(i18n.ErrorConversionFailed, err), 1)
			}

			if report.VirtualMixes > 0 {
				fmt.Fprintf(stdout, localizer.Text(i18n.OutputConvertedWithMixes), source, report.Output, report.Mode, report.VirtualMixes)
			} else {
				fmt.Fprintf(stdout, localizer.Text(i18n.OutputConverted), source, report.Output, report.Mode)
			}
			if report.Colors != "" {
				fmt.Fprintf(stdout, localizer.Text(i18n.OutputRequiredColors), report.Colors)
			}
			keys := make([]int, 0, len(report.PhysicalMapping))
			for sourceID := range report.PhysicalMapping {
				keys = append(keys, sourceID)
			}
			sort.Ints(keys)
			for _, sourceID := range keys {
				fmt.Fprintf(stdout, localizer.Text(i18n.OutputSourceMapping), sourceID, report.PhysicalMapping[sourceID])
			}
			if len(report.Plates) > 0 {
				printPalette, parseErr := threemf.ParsePalette(report.Colors)
				if parseErr != nil {
					return urfavecli.Exit(localizer.Wrap(i18n.ErrorInvalidRequiredColors, parseErr), 1)
				}
				printPlateSteps(stdout, localizer, report.Plates, printPalette)
			}
			return nil
		},
	}
	if localizer.IsChinese() {
		command.HideHelp = true
		command.CustomRootCommandHelpTemplate = localizedHelpTemplate(localizer)
		command.Flags = append(command.Flags, &urfavecli.BoolFlag{
			Name: "help", Aliases: []string{"h"}, Usage: localizer.Text(i18n.HelpShow), HideDefault: true, Local: true,
		})
		command.OnUsageError = func(_ context.Context, command *urfavecli.Command, err error, _ bool) error {
			err = localizedUsageError(localizer, command, err)
			fmt.Fprintf(stderr, localizer.Text(i18n.ErrorIncorrectUsage), err)
			_ = urfavecli.ShowRootCommandHelp(command)
			return err
		}
	}
	return command
}

func colorPlanRows(localizer i18n.Localizer, sequence threemf.ColorSequence, defaultMode threemf.MixMode) ([]progressui.ColorSourceRow, []progressui.ColorOutputRow, []progressui.MixModeOption) {
	options := []progressui.MixModeOption{
		{Label: localizer.Text(i18n.ModeRatio), Value: threemf.MixModeRatio.String()},
		{Label: localizer.Text(i18n.ModeCycle), Value: threemf.MixModeCycle.String()},
		{Label: localizer.Text(i18n.ModeMatch), Value: threemf.MixModeMatch.String()},
		{Label: localizer.Text(i18n.ModeGradient), Value: threemf.MixModeGradient.String()},
	}
	sources := make([]progressui.ColorSourceRow, len(sequence.Source))
	for index, source := range sequence.Source {
		sources[index] = progressui.ColorSourceRow{
			Slot: source.Slot, Color: source.Color, Used: source.Used, OutputSlot: source.OutputSlot,
		}
	}
	outputs := make([]progressui.ColorOutputRow, len(sequence.Output))
	for index, output := range sequence.Output {
		mode := output.Mode
		if mode == "" {
			mode = defaultMode
		}
		selected := 0
		for optionIndex, option := range options {
			if option.Value == mode.String() {
				selected = optionIndex
				break
			}
		}
		outputs[index] = progressui.ColorOutputRow{
			Slot: output.Slot, Color: output.Color, Base: output.Base, Mixed: output.Mixed, Editable: output.Editable, Mode: selected,
			ReplacementSlot: output.Slot,
		}
	}
	hasMixed := false
	for _, output := range sequence.Output {
		if output.Mixed {
			hasMixed = true
			break
		}
	}
	if !hasMixed {
		options = nil
	}
	return sources, outputs, options
}

// GLUE: translates terminal output slots into planner references that survive output deduplication and renumbering.
func applyColorPlanSelection(request *threemf.Request, sequence threemf.ColorSequence, selected progressui.ColorPlanResult) error {
	outputBySlot := make(map[int]threemf.OutputColorSlot, len(sequence.Output))
	expected := 0
	for _, output := range sequence.Output {
		outputBySlot[output.Slot] = output
		if output.Editable {
			expected++
		}
	}
	if len(selected.Colors) != expected {
		return fmt.Errorf("color order review returned %d rows for %d editable outputs", len(selected.Colors), expected)
	}
	request.MaterialMixModes = make(map[int]threemf.MixMode)
	request.MaterialReplacements = make(map[int]threemf.MaterialReplacement)
	seen := make(map[int]bool, len(selected.Colors))
	for _, selection := range selected.Colors {
		color, found := outputBySlot[selection.OutputSlot]
		if !found || !color.Editable || seen[selection.OutputSlot] {
			return fmt.Errorf("color order review references unknown editable output T%d", selection.OutputSlot)
		}
		seen[selection.OutputSlot] = true
		if color.Mixed {
			mode, err := threemf.ParseMixMode(selection.Mode)
			if err != nil {
				return err
			}
			for _, material := range color.MaterialIDs {
				request.MaterialMixModes[material] = mode
			}
		}
		if selection.ReplacementSlot == selection.OutputSlot {
			continue
		}
		target, found := outputBySlot[selection.ReplacementSlot]
		if !found {
			return fmt.Errorf("color order review references unknown replacement T%d", selection.ReplacementSlot)
		}
		replacement := threemf.MaterialReplacement{}
		if target.Base {
			replacement.BaseSlot = target.Slot
		} else if len(target.MaterialIDs) > 0 {
			replacement.SourceMaterial = target.MaterialIDs[0]
		} else {
			return fmt.Errorf("replacement output T%d has no source material", target.Slot)
		}
		for _, material := range color.MaterialIDs {
			request.MaterialReplacements[material] = replacement
		}
	}
	return nil
}

// GLUE: translates the three UI toggles at the terminal/domain boundary.
func localZSelection(settings threemf.LocalZSettings) progressui.LocalZSelection {
	return progressui.LocalZSelection{
		LayerHeight: settings.LayerHeight, Infill: settings.Infill, WholeObjects: settings.WholeObjects,
	}
}

func localZSettings(selection progressui.LocalZSelection) threemf.LocalZSettings {
	return threemf.LocalZSettings{
		LayerHeight: selection.LayerHeight, Infill: selection.Infill, WholeObjects: selection.WholeObjects,
	}
}

type plateGroup struct {
	neutral threemf.ColorRole
	colors  string
	plates  []threemf.PlateReport
}

func printPlateSteps(output *os.File, localizer i18n.Localizer, plates []threemf.PlateReport, palette threemf.Palette) {
	initial := palette.Neutral()
	neutralSlot := palette.Slot(initial)
	groups := groupPlatesForPrinting(plates, initial)
	changes := 0
	current := initial
	for _, group := range groups {
		if group.neutral != current {
			changes++
			current = group.neutral
		}
	}

	if changes == 1 {
		fmt.Fprintf(output, localizer.Text(i18n.PrintingStepsOne), neutralSlot)
	} else {
		fmt.Fprintf(output, localizer.Text(i18n.PrintingStepsMany), changes, neutralSlot)
	}
	current = initial
	for _, group := range groups {
		if group.neutral == current {
			fmt.Fprintf(output, localizer.Text(i18n.PrintingStepKeep), neutralSlot, localizedColorRole(localizer, group.neutral), group.colors)
		} else {
			fmt.Fprintf(output, localizer.Text(i18n.PrintingStepReplace), neutralSlot, localizedColorRole(localizer, current), localizedColorRole(localizer, group.neutral), group.colors)
			current = group.neutral
		}
		for index, plate := range group.plates {
			if index > 0 {
				fmt.Fprint(output, ", ")
			}
			fmt.Fprintf(output, localizer.Text(i18n.PrintingPlate), plate.Number)
			if plate.Name != "" {
				fmt.Fprintf(output, " - %s", plate.Name)
			}
		}
		fmt.Fprintln(output)
	}
}

func groupPlatesForPrinting(plates []threemf.PlateReport, initial threemf.ColorRole) []plateGroup {
	groups := make([]plateGroup, 0, 3)
	indexes := make(map[threemf.ColorRole]int, 3)
	for _, plate := range plates {
		index, found := indexes[plate.Neutral]
		if !found {
			index = len(groups)
			indexes[plate.Neutral] = index
			groups = append(groups, plateGroup{neutral: plate.Neutral, colors: plate.Colors})
		}
		groups[index].plates = append(groups[index].plates, plate)
	}

	initialIndex, found := indexes[initial]
	if !found || initialIndex == 0 {
		return groups
	}
	optimized := make([]plateGroup, 0, len(groups))
	optimized = append(optimized, groups[initialIndex])
	optimized = append(optimized, groups[:initialIndex]...)
	optimized = append(optimized, groups[initialIndex+1:]...)
	return optimized
}
func defaultOutputPath(source string) string {
	directory := filepath.Dir(source)
	name := filepath.Base(source)
	extension := filepath.Ext(name)
	stem := strings.TrimSuffix(name, extension)
	if stem == "" {
		stem = name
	}
	return filepath.Join(directory, stem+"-btu.3mf")
}
