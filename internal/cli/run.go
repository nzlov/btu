package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	urfavecli "github.com/urfave/cli/v3"

	"github.com/nzlov/btu/internal/progressui"
	"github.com/nzlov/btu/internal/threemf"
)

type convertFunc func(context.Context, threemf.Request, threemf.ProgressFunc) (threemf.Report, error)
type confirmFunc func(context.Context, *os.File, string) (bool, error)
type mapColorsFunc func(context.Context, *os.File, []progressui.ColorMappingRow, []progressui.ColorOption) ([]string, error)

func Run(args []string, stdout, stderr *os.File) int {
	return runWithPrompts(
		args,
		stdout,
		stderr,
		threemf.Convert,
		func(ctx context.Context, output *os.File, prompt string) (bool, error) {
			return progressui.Confirm(ctx, os.Stdin, output, prompt)
		},
		func(ctx context.Context, output *os.File, rows []progressui.ColorMappingRow, options []progressui.ColorOption) ([]string, error) {
			return progressui.MapColors(ctx, os.Stdin, output, rows, options)
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
		func(context.Context, *os.File, []progressui.ColorMappingRow, []progressui.ColorOption) ([]string, error) {
			return nil, errors.New("color mapping was not expected")
		},
	)
}

// GLUE: keeps both Bubble Tea prompts injectable without leaking terminal concerns into conversion.
func runWithPrompts(args []string, stdout, stderr *os.File, convert convertFunc, confirm confirmFunc, mapColors mapColorsFunc) int {
	command := newCommand(stdout, stderr, convert, confirm, mapColors)
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

func newCommand(stdout, stderr *os.File, convert convertFunc, confirm confirmFunc, mapColors mapColorsFunc) *urfavecli.Command {
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
				Usage:            "set preferred slot 1-4 colors using four `CMYGWB` characters",
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
				if errors.As(err, &required) && !request.FullSpectrum && len(request.ColorMapping) == 0 && len(required.Mappings) > 0 {
					accepted, confirmErr := confirm(ctx, stderr, fmt.Sprintf("Source declares %d colors; %d need mixing. Keep them with full-spectrum?", required.ColorCount, required.NonBaseCount))
					if confirmErr != nil {
						return urfavecli.Exit(fmt.Errorf("%w; rerun with --full-spectrum", confirmErr), 1)
					}
					rows, options := colorMappingRows(required.Mappings, accepted)
					selected, mappingErr := mapColors(ctx, stderr, rows, options)
					if mappingErr != nil {
						return urfavecli.Exit(mappingErr, 1)
					}
					if len(selected) != len(required.Mappings) {
						return urfavecli.Exit(fmt.Errorf("color mapping returned %d rows for %d source colors", len(selected), len(required.Mappings)), 1)
					}
					request.ColorMapping = make(map[int]string)
					for index, mapping := range required.Mappings {
						for _, material := range mapping.MaterialIDs {
							request.ColorMapping[material] = selected[index]
						}
					}
					request.FullSpectrum = accepted
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
			return nil
		},
	}
}

func colorMappingRows(mappings []threemf.ColorMapping, keepMixed bool) ([]progressui.ColorMappingRow, []progressui.ColorOption) {
	baseColors := []struct {
		role  threemf.ColorRole
		label string
		color string
	}{
		{role: threemf.ColorCyan, label: "cyan (c)", color: "#0000FF"},
		{role: threemf.ColorMagenta, label: "magenta (m)", color: "#FF0000"},
		{role: threemf.ColorYellow, label: "yellow (y)", color: "#FFFF00"},
		{role: threemf.ColorGray, label: "gray (g)", color: "#808080"},
		{role: threemf.ColorWhite, label: "white (w)", color: "#FFFFFF"},
		{role: threemf.ColorBlack, label: "black (b)", color: "#000000"},
	}
	options := make([]progressui.ColorOption, 0, len(baseColors)+len(mappings))
	roleOption := make(map[threemf.ColorRole]int, len(baseColors))
	colorOption := make(map[string]int, len(baseColors)+len(mappings))
	for _, base := range baseColors {
		roleOption[base.role] = len(options)
		colorOption[base.color] = len(options)
		options = append(options, progressui.ColorOption{Label: base.label, Color: base.color})
	}
	for _, mapping := range mappings {
		if mapping.Base {
			continue
		}
		if _, exists := colorOption[mapping.Color]; exists {
			continue
		}
		colorOption[mapping.Color] = len(options)
		options = append(options, progressui.ColorOption{Label: "mixed", Color: mapping.Color})
	}

	rows := make([]progressui.ColorMappingRow, len(mappings))
	for index, mapping := range mappings {
		ids := make([]string, len(mapping.MaterialIDs))
		for idIndex, material := range mapping.MaterialIDs {
			ids[idIndex] = "T" + strconv.Itoa(material)
		}
		state := "unused"
		if mapping.Used {
			state = "used"
		}
		selected := roleOption[mapping.Suggested]
		if keepMixed && !mapping.Base {
			selected = colorOption[mapping.Color]
		}
		rows[index] = progressui.ColorMappingRow{
			Label:    strings.Join(ids, "/") + " (" + state + ")",
			Color:    mapping.Color,
			Selected: selected,
		}
	}
	return rows, options
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
