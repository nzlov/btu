package progressui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ColorOption struct {
	Label string
	Color string
}

type ColorMappingRow struct {
	Label    string
	Color    string
	Selected int
}

type mappingModel struct {
	rows    []ColorMappingRow
	options []ColorOption
	cursor  int
	done    bool
	err     error
}

func (model mappingModel) Init() tea.Cmd {
	return nil
}

func (model mappingModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch key.String() {
	case "up", "k":
		if model.cursor > 0 {
			model.cursor--
		}
	case "down", "j":
		if model.cursor+1 < len(model.rows) {
			model.cursor++
		}
	case "left", "h":
		model.moveSelection(-1)
	case "right", "l":
		model.moveSelection(1)
	case "enter":
		model.done = true
		return model, tea.Quit
	case "ctrl+c", "q", "esc":
		model.err = context.Canceled
		model.done = true
		return model, tea.Quit
	}
	return model, nil
}

func (model *mappingModel) moveSelection(delta int) {
	if len(model.rows) == 0 || len(model.options) == 0 {
		return
	}
	selected := model.rows[model.cursor].Selected + delta
	if selected < 0 {
		selected = len(model.options) - 1
	}
	if selected >= len(model.options) {
		selected = 0
	}
	model.rows[model.cursor].Selected = selected
}

func (model mappingModel) View() string {
	if model.done {
		return ""
	}
	var output strings.Builder
	output.WriteString("Map source colors\n\n")
	for index, row := range model.rows {
		cursor := "  "
		if index == model.cursor {
			cursor = "> "
		}
		option := model.options[row.Selected]
		fmt.Fprintf(&output, "%s%s %s -> %s %s\n", cursor, row.Label, row.Color, option.Label, option.Color)
	}
	return output.String()
}

func MapColors(ctx context.Context, input, output *os.File, rows []ColorMappingRow, options []ColorOption) ([]string, error) {
	if len(rows) == 0 || len(options) == 0 {
		return nil, fmt.Errorf("color mapping requires at least one row and target")
	}
	for _, row := range rows {
		if row.Selected < 0 || row.Selected >= len(options) {
			return nil, fmt.Errorf("color mapping selection is out of range")
		}
	}
	if !isTerminal(input) || !isTerminal(output) {
		return nil, fmt.Errorf("cannot configure color mapping without an interactive terminal; rerun with --full-spectrum to keep all detected colors")
	}
	program := tea.NewProgram(
		mappingModel{rows: rows, options: options},
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	)
	final, err := program.Run()
	if err != nil {
		return nil, err
	}
	result := final.(mappingModel)
	if result.err != nil {
		return nil, result.err
	}
	colors := make([]string, len(result.rows))
	for index, row := range result.rows {
		colors[index] = result.options[row.Selected].Color
	}
	return colors, nil
}
