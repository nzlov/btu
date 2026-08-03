package progressui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Progress struct {
	Current int
	Total   int
	Stage   string
	Detail  string
}

type Job[T any] func(report func(Progress)) (T, error)

type resultMsg[T any] struct {
	value T
	err   error
}

type model[T any] struct {
	ctx      context.Context
	events   <-chan any
	progress Progress
	result   resultMsg[T]
	done     bool
}

func waitForEvent[T any](ctx context.Context, events <-chan any) tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-events:
			return event
		case <-ctx.Done():
			return resultMsg[T]{err: ctx.Err()}
		}
	}
}

func (m model[T]) Init() tea.Cmd {
	return waitForEvent[T](m.ctx, m.events)
}

func (m model[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			m.result.err = context.Canceled
			m.done = true
			return m, tea.Quit
		}
	case Progress:
		m.progress = msg
		return m, waitForEvent[T](m.ctx, m.events)
	case resultMsg[T]:
		m.result = msg
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model[T]) View() string {
	if m.done {
		return ""
	}
	p := m.progress
	if p.Total <= 0 {
		return "Preparing...\n"
	}
	width := 28
	filled := p.Current * width / p.Total
	if filled > width {
		filled = width
	}
	percent := p.Current * 100 / p.Total
	detail := ""
	if p.Detail != "" {
		detail = "  " + p.Detail
	}
	return fmt.Sprintf("[%s%s] %3d%%  %s%s\n", strings.Repeat("#", filled), strings.Repeat("-", width-filled), percent, p.Stage, detail)
}

func Run[T any](ctx context.Context, output *os.File, job Job[T]) (T, error) {
	if !isTerminal(output) {
		return runPlain(output, job)
	}

	events := make(chan any)
	go func() {
		value, err := job(func(progress Progress) {
			events <- progress
		})
		events <- resultMsg[T]{value: value, err: err}
	}()

	initial := model[T]{ctx: ctx, events: events}
	program := tea.NewProgram(initial, tea.WithInput(nil), tea.WithOutput(output))
	final, err := program.Run()
	if err != nil {
		var zero T
		return zero, err
	}
	result := final.(model[T]).result
	return result.value, result.err
}

func runPlain[T any](output io.Writer, job Job[T]) (T, error) {
	lastCurrent := -1
	lastStage := ""
	return job(func(progress Progress) {
		if progress.Current == lastCurrent && progress.Stage == lastStage {
			return
		}
		lastCurrent = progress.Current
		lastStage = progress.Stage
		fmt.Fprintf(output, "[%d/%d] %s", progress.Current, progress.Total, progress.Stage)
		if progress.Detail != "" {
			fmt.Fprintf(output, ": %s", progress.Detail)
		}
		fmt.Fprintln(output)
	})
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
