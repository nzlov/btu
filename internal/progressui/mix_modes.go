package progressui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type MixModeOption struct {
	Label string
	Value string
}

type MixModeRow struct {
	Label    string
	Color    string
	Selected int
}

type mixModeModel struct {
	rows    []MixModeRow
	options []MixModeOption
	cursor  int
	done    bool
	err     error
}

func (model mixModeModel) Init() tea.Cmd {
	return nil
}

func (model mixModeModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
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

func (model *mixModeModel) moveSelection(delta int) {
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

func (model mixModeModel) View() string {
	if model.done {
		return ""
	}
	labelWidth := 0
	for _, row := range model.rows {
		labelWidth = max(labelWidth, len(row.Label))
	}
	var output strings.Builder
	output.WriteString("Choose mix modes\n\n")
	for index, row := range model.rows {
		cursor := "  "
		if index == model.cursor {
			cursor = "> "
		}
		option := model.options[row.Selected]
		fmt.Fprintf(&output, "%s%-*s  %s %s  [%-8s]\n", cursor, labelWidth, row.Label, colorSwatch(row.Color), row.Color, option.Label)
	}
	return output.String()
}

func colorSwatch(color string) string {
	value := strings.TrimPrefix(color, "#")
	if len(value) != 6 {
		return "  "
	}
	rgb, err := strconv.ParseUint(value, 16, 24)
	if err != nil {
		return "  "
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm  \x1b[0m", rgb>>16, rgb>>8&0xff, rgb&0xff)
}

func SelectMixModes(ctx context.Context, input, output *os.File, rows []MixModeRow, options []MixModeOption) ([]string, error) {
	if len(rows) == 0 || len(options) == 0 {
		return nil, fmt.Errorf("mix mode selection requires at least one row and mode")
	}
	for _, row := range rows {
		if row.Selected < 0 || row.Selected >= len(options) {
			return nil, fmt.Errorf("mix mode selection is out of range")
		}
	}
	if !isTerminal(input) || !isTerminal(output) {
		return nil, fmt.Errorf("cannot select mix modes without an interactive terminal")
	}
	program := tea.NewProgram(
		mixModeModel{rows: rows, options: options},
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	)
	final, err := program.Run()
	if err != nil {
		return nil, err
	}
	result := final.(mixModeModel)
	if result.err != nil {
		return nil, result.err
	}
	modes := make([]string, len(result.rows))
	for index, row := range result.rows {
		modes[index] = result.options[row.Selected].Value
	}
	return modes, nil
}
