package progressui

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type confirmModel struct {
	prompt   string
	accepted bool
	done     bool
	err      error
}

func (model confirmModel) Init() tea.Cmd {
	return nil
}

func (model confirmModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch key.String() {
	case "y", "Y":
		model.accepted = true
		model.done = true
		return model, tea.Quit
	case "n", "N", "enter", "esc":
		model.done = true
		return model, tea.Quit
	case "ctrl+c", "q":
		model.err = context.Canceled
		model.done = true
		return model, tea.Quit
	default:
		return model, nil
	}
}

func (model confirmModel) View() string {
	if model.done {
		return ""
	}
	return fmt.Sprintf("%s [y/N] ", model.prompt)
}

func Confirm(ctx context.Context, input, output *os.File, prompt string) (bool, error) {
	if !isTerminal(input) || !isTerminal(output) {
		return false, fmt.Errorf("cannot ask for full-spectrum confirmation without an interactive terminal; rerun with --full-spectrum")
	}
	program := tea.NewProgram(
		confirmModel{prompt: prompt},
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	)
	final, err := program.Run()
	if err != nil {
		return false, err
	}
	result := final.(confirmModel)
	return result.accepted, result.err
}
