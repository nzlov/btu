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

func Run(args []string, stdout, stderr *os.File) int {
	return runWithConfirm(args, stdout, stderr, threemf.Convert, func(ctx context.Context, output *os.File, prompt string) (bool, error) {
		return progressui.Confirm(ctx, os.Stdin, output, prompt)
	})
}

func run(args []string, stdout, stderr *os.File, convert convertFunc) int {
	return runWithConfirm(args, stdout, stderr, convert, func(context.Context, *os.File, string) (bool, error) {
		return false, errors.New("confirmation was not expected")
	})
}

// GLUE: keeps the terminal prompt injectable at the CLI/UI boundary for deterministic tests.
func runWithConfirm(args []string, stdout, stderr *os.File, convert convertFunc, confirm confirmFunc) int {
	command := newCommand(stdout, stderr, convert, confirm)
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

func newCommand(stdout, stderr *os.File, convert convertFunc, confirm confirmFunc) *urfavecli.Command {
	nozzleFlag := &urfavecli.StringFlag{
		Name:     "nozzle",
		Aliases:  []string{"n"},
		Usage:    "use the built-in U1 baseline for `SIZE` mm instead of the source nozzle",
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
			&urfavecli.StringFlag{
				Name:             "colors",
				Aliases:          []string{"c"},
				Usage:            "set preferred slot 1-4 colors using four `WRBYK` characters",
				Value:            "bryk",
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
			report, err := convertRequest()
			var required *threemf.FullSpectrumRequiredError
			if errors.As(err, &required) && !request.FullSpectrum {
				accepted, confirmErr := confirm(ctx, stderr, fmt.Sprintf("Source uses %d colors. Enable full-spectrum mixing?", required.ColorCount))
				if confirmErr != nil {
					return urfavecli.Exit(confirmErr, 1)
				}
				if !accepted {
					return urfavecli.Exit(fmt.Errorf("full-spectrum mixing was not enabled for %d source colors", required.ColorCount), 1)
				}
				request.FullSpectrum = true
				report, err = convertRequest()
			}
			if err != nil {
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
