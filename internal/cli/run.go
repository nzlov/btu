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

	"github.com/nzlov/btu/internal/progressui"
	"github.com/nzlov/btu/internal/threemf"
)

type convertFunc func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.Report, error)
type confirmFunc func(context.Context, *os.File, string) (bool, error)
type selectMixModesFunc func(context.Context, *os.File, []progressui.MixModeRow, []progressui.MixModeOption) ([]string, error)

func Run(args []string, stdout, stderr *os.File) int {
	return runWithPrompts(
		args,
		stdout,
		stderr,
		threemf.Convert,
		func(ctx context.Context, output *os.File, prompt string) (bool, error) {
			return progressui.Confirm(ctx, os.Stdin, output, prompt)
		},
		func(ctx context.Context, output *os.File, rows []progressui.MixModeRow, options []progressui.MixModeOption) ([]string, error) {
			return progressui.SelectMixModes(ctx, os.Stdin, output, rows, options)
		},
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
		func(context.Context, *os.File, []progressui.MixModeRow, []progressui.MixModeOption) ([]string, error) {
			return nil, errors.New("mix mode selection was not expected")
		},
	)
}

// GLUE: keeps Bubble Tea prompts injectable without leaking terminal concerns into conversion.
func runWithPrompts(args []string, stdout, stderr *os.File, convert convertFunc, confirm confirmFunc, selectMixModes selectMixModesFunc) int {
	command := newCommand(stdout, stderr, convert, confirm, selectMixModes)
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

func newCommand(stdout, stderr *os.File, convert convertFunc, confirm confirmFunc, selectMixModes selectMixModesFunc) *urfavecli.Command {
	nozzleFlag := &urfavecli.StringFlag{
		Name:     "nozzle",
		Aliases:  []string{"n"},
		Usage:    "use the built-in U1 baseline for `DIAMETER_MM` instead of the source nozzle (allowed: 0.2, 0.4, 0.6, 0.8)",
		OnlyOnce: true,
		Validator: func(value string) error {
			_, err := threemf.ParseNozzleSize(value)
			return err
		},
	}
	templateFlag := &urfavecli.StringFlag{
		Name:      "template",
		Aliases:   []string{"t"},
		Usage:     "override the built-in U1 baseline with `FILE`",
		OnlyOnce:  true,
		TakesFile: true,
	}
	mixModeFlag := &urfavecli.StringFlag{
		Name:             "mix-mode",
		Aliases:          []string{"m"},
		Usage:            "use `MODE` for generated mixtures (ratio, cycle, match, or gradient)",
		Value:            threemf.MixModeRatio.String(),
		OnlyOnce:         true,
		ValidateDefaults: true,
		Validator: func(value string) error {
			_, err := threemf.ParseMixMode(value)
			return err
		},
	}
	return &urfavecli.Command{
		Name:      "btu",
		Usage:     "convert Bambu Studio 3MF materials for Snapmaker U1",
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
				Usage:     "write to `FILE` instead of SOURCE-btu.3mf",
				OnlyOnce:  true,
				TakesFile: true,
			},
			&urfavecli.BoolFlag{
				Name:     "replace",
				Aliases:  []string{"r"},
				Usage:    "replace the output file when it already exists",
				OnlyOnce: true,
			},
			&urfavecli.StringFlag{
				Name:             "colors",
				Aliases:          []string{"c"},
				Usage:            "keep CMY in slots 1-3 and set preview T4 with `ORDER` (cmyg, cmyw, or cmyb)",
				Value:            "cmyg",
				OnlyOnce:         true,
				ValidateDefaults: true,
				Validator: func(value string) error {
					_, err := threemf.ParsePalette(value)
					return err
				},
			},
			&urfavecli.BoolFlag{
				Name:     "full-spectrum",
				Aliases:  []string{"f"},
				Usage:    "synthesize source colors from the four loaded filaments",
				OnlyOnce: true,
			},
			mixModeFlag,
			nozzleFlag,
			templateFlag,
		},
		Action: func(ctx context.Context, command *urfavecli.Command) error {
			if command.NArg() != 1 {
				return urfavecli.Exit("expected exactly one source 3MF", 2)
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
			convertRequest := func() (threemf.Report, error) {
				return progressui.Run(ctx, stderr, func(progress func(progressui.Progress)) (threemf.Report, error) {
					return convert(ctx, request, func(event threemf.Progress) {
						progress(progressui.Progress(event))
					})
				})
			}
			var report threemf.Report
			for {
				report, err = convertRequest()
				if err == nil {
					break
				}

				var outputExists *threemf.OutputExistsError
				if errors.As(err, &outputExists) && !request.Replace {
					accepted, confirmErr := confirm(ctx, stderr, fmt.Sprintf("Output %s already exists. Replace it?", outputExists.Path))
					if confirmErr != nil {
						return urfavecli.Exit(fmt.Errorf("%w; rerun with --replace", confirmErr), 1)
					}
					if !accepted {
						return urfavecli.Exit(fmt.Errorf("output was not replaced: %s", outputExists.Path), 1)
					}
					request.Replace = true
					continue
				}

				var required *threemf.FullSpectrumRequiredError
				if errors.As(err, &required) && !request.FullSpectrum && !request.PreserveMaterialSlots {
					accepted, confirmErr := confirm(ctx, stderr, fmt.Sprintf("Source declares %d colors; %d need mixing. Generate them with full-spectrum mixing?", required.ColorCount, required.NonBaseCount))
					if confirmErr != nil {
						return urfavecli.Exit(fmt.Errorf("%w; rerun with --full-spectrum", confirmErr), 1)
					}
					if !accepted {
						request.PreserveMaterialSlots = true
						continue
					}
					request.FullSpectrum = true
					if len(required.Colors) == 0 {
						continue
					}
					rows, options := mixModeRows(required.Colors, request.MixMode)
					selected, selectionErr := selectMixModes(ctx, stderr, rows, options)
					if selectionErr != nil {
						return urfavecli.Exit(fmt.Errorf("%w; rerun with --full-spectrum --mix-mode %s", selectionErr, request.MixMode), 1)
					}
					if len(selected) != len(required.Colors) {
						return urfavecli.Exit(fmt.Errorf("mix mode selection returned %d rows for %d source colors", len(selected), len(required.Colors)), 1)
					}
					request.MaterialMixModes = make(map[int]threemf.MixMode)
					for index, color := range required.Colors {
						mode, parseErr := threemf.ParseMixMode(selected[index])
						if parseErr != nil {
							return urfavecli.Exit(parseErr, 1)
						}
						for _, material := range color.MaterialIDs {
							request.MaterialMixModes[material] = mode
						}
					}
					continue
				}

				return urfavecli.Exit(err, 1)
			}

			fmt.Fprintf(stdout, "Converted %s -> %s (%s", source, report.Output, report.Mode)
			if report.VirtualMixes > 0 {
				fmt.Fprintf(stdout, ", %d mixed materials", report.VirtualMixes)
			}
			fmt.Fprintln(stdout, ")")
			if report.Colors != "" {
				fmt.Fprintf(stdout, "  U1 colors: %s\n", report.Colors)
			}
			keys := make([]int, 0, len(report.PhysicalMapping))
			for sourceID := range report.PhysicalMapping {
				keys = append(keys, sourceID)
			}
			sort.Ints(keys)
			for _, sourceID := range keys {
				fmt.Fprintf(stdout, "  source T%d -> U1 T%d\n", sourceID, report.PhysicalMapping[sourceID])
			}
			if len(report.Plates) > 0 {
				printPlateSteps(stdout, report.Plates, request.Palette.Slots[3])
			}
			return nil
		},
	}
}

func mixModeRows(colors []threemf.MixModeColor, defaultMode threemf.MixMode) ([]progressui.MixModeRow, []progressui.MixModeOption) {
	options := []progressui.MixModeOption{
		{Label: "Ratio", Value: threemf.MixModeRatio.String()},
		{Label: "Cycle", Value: threemf.MixModeCycle.String()},
		{Label: "Match", Value: threemf.MixModeMatch.String()},
		{Label: "Gradient", Value: threemf.MixModeGradient.String()},
	}
	selected := 0
	for index, option := range options {
		if option.Value == defaultMode.String() {
			selected = index
			break
		}
	}
	rows := make([]progressui.MixModeRow, len(colors))
	for index, color := range colors {
		ids := make([]string, len(color.MaterialIDs))
		for idIndex, material := range color.MaterialIDs {
			ids[idIndex] = fmt.Sprintf("T%d", material)
		}
		state := "unused"
		if color.Used {
			state = "used"
		}
		rows[index] = progressui.MixModeRow{
			Label:    fmt.Sprintf("%s (%s)", strings.Join(ids, "/"), state),
			Color:    color.Color,
			Selected: selected,
		}
	}
	return rows, options
}

type plateGroup struct {
	neutral threemf.ColorRole
	colors  string
	plates  []threemf.PlateReport
}

func printPlateSteps(output *os.File, plates []threemf.PlateReport, initial threemf.ColorRole) {
	groups := groupPlatesForPrinting(plates, initial)
	changes := 0
	current := initial
	for _, group := range groups {
		if group.neutral != current {
			changes++
			current = group.neutral
		}
	}

	label := "changes"
	if changes == 1 {
		label = "change"
	}
	fmt.Fprintf(output, "Printing steps (recommended order, %d T4 %s):\n", changes, label)
	current = initial
	for _, group := range groups {
		if group.neutral == current {
			fmt.Fprintf(output, "  keep T4 %s (%s): ", group.neutral, group.colors)
		} else {
			fmt.Fprintf(output, "  replace T4 %s -> %s (%s): ", current, group.neutral, group.colors)
			current = group.neutral
		}
		for index, plate := range group.plates {
			if index > 0 {
				fmt.Fprint(output, ", ")
			}
			fmt.Fprintf(output, "plate %d", plate.Number)
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
