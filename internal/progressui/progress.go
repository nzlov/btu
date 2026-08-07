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
	Current     int
	Total       int
	Stage       string
	Detail      string
	ItemCurrent int
	ItemTotal   int
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
	width    int
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
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
	current := max(0, min(p.Current, p.Total))
	barWidth := 28
	if m.width > 0 {
		stageWidth := len([]rune(p.Stage))
		barWidth = max(8, min(barWidth, m.width-stageWidth-9))
	}
	filled := current * barWidth / p.Total
	percent := current * 100 / p.Total
	line := fmt.Sprintf("[%s%s] %3d%%  %s", strings.Repeat("#", filled), strings.Repeat("-", barWidth-filled), percent, p.Stage)
	if p.ItemTotal <= 0 {
		if p.Detail != "" {
			line += "  " + p.Detail
		}
		return truncateProgressLine(line, m.width) + "\n"
	}
	itemCurrent := max(0, min(p.ItemCurrent, p.ItemTotal))
	itemPercent := itemCurrent * 100 / p.ItemTotal
	detail := fmt.Sprintf("  %d/%d (%d%%)", itemCurrent, p.ItemTotal, itemPercent)
	if p.Detail != "" {
		detail += "  " + p.Detail
	}
	return truncateProgressLine(line, m.width) + "\n" + truncateProgressLine(detail, m.width) + "\n"
}

func truncateProgressLine(value string, width int) string {
	characters := []rune(value)
	if width <= 3 || len(characters) <= width {
		return value
	}
	return string(characters[:width-3]) + "..."
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
	var last Progress
	hasLast := false
	return job(func(progress Progress) {
		if hasLast && progress == last {
			return
		}
		last = progress
		hasLast = true
		fmt.Fprintf(output, "[%d/%d] %s", progress.Current, progress.Total, progress.Stage)
		if progress.ItemTotal > 0 {
			fmt.Fprintf(output, " [%d/%d]", progress.ItemCurrent, progress.ItemTotal)
		}
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
