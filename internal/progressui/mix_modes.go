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

type ColorSourceRow struct {
	Slot       int
	Color      string
	Used       bool
	OutputSlot int
}

type ColorOutputRow struct {
	Slot            int
	Color           string
	Base            bool
	Mixed           bool
	Editable        bool
	Mode            int
	ReplacementSlot int
}

type ColorPlanSelection struct {
	OutputSlot      int
	Mode            string
	ReplacementSlot int
}

type LocalZSelection struct {
	LayerHeight  bool
	Infill       bool
	WholeObjects bool
}

type ColorPlanResult struct {
	Colors []ColorPlanSelection
	LocalZ LocalZSelection
}

type colorPlanFocus int

const (
	colorPlanModeFocus colorPlanFocus = iota
	colorPlanReplacementFocus
)

type colorPlanModel struct {
	sources []ColorSourceRow
	outputs []ColorOutputRow
	options []MixModeOption
	localZ  LocalZSelection
	cursor  int
	focus   colorPlanFocus
	width   int
	height  int
	done    bool
	err     error
}

func newColorPlanModel(sources []ColorSourceRow, outputs []ColorOutputRow, options []MixModeOption, localZ LocalZSelection) colorPlanModel {
	model := colorPlanModel{
		sources: append([]ColorSourceRow(nil), sources...),
		outputs: append([]ColorOutputRow(nil), outputs...),
		options: append([]MixModeOption(nil), options...),
		localZ:  localZ,
	}
	model.normalizeFocus()
	return model
}

func (model colorPlanModel) Init() tea.Cmd {
	return nil
}

func (model colorPlanModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		return model, nil
	case tea.KeyMsg:
		switch message.String() {
		case "up", "k":
			model.moveCursor(-1)
		case "down", "j":
			model.moveCursor(1)
		case "tab", "shift+tab":
			if index := model.selectedOutputIndex(); index >= 0 && model.outputs[index].Mixed {
				if model.focus == colorPlanModeFocus {
					model.focus = colorPlanReplacementFocus
				} else {
					model.focus = colorPlanModeFocus
				}
			}
		case "left", "h":
			model.moveSelection(-1)
		case "right", "l":
			model.moveSelection(1)
		case " ":
			model.toggleLocalZ()
		case "enter":
			model.done = true
			return model, tea.Quit
		case "ctrl+c", "q", "esc":
			model.err = context.Canceled
			model.done = true
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model *colorPlanModel) moveCursor(delta int) {
	selected := model.cursor + delta
	if selected >= 0 && selected < model.selectionCount() {
		model.cursor = selected
		model.normalizeFocus()
	}
}

func (model *colorPlanModel) normalizeFocus() {
	index := model.selectedOutputIndex()
	if index >= 0 && !model.outputs[index].Mixed {
		model.focus = colorPlanReplacementFocus
	}
}

func (model *colorPlanModel) moveSelection(delta int) {
	index := model.selectedOutputIndex()
	if index < 0 {
		model.toggleLocalZ()
		return
	}
	row := &model.outputs[index]
	if row.Mixed && model.focus == colorPlanModeFocus {
		row.Mode = wrappedIndex(row.Mode+delta, len(model.options))
		return
	}
	current := model.outputIndex(row.ReplacementSlot)
	if current < 0 {
		current = index
	}
	row.ReplacementSlot = model.outputs[wrappedIndex(current+delta, len(model.outputs))].Slot
}

func (model colorPlanModel) editableCount() int {
	count := 0
	for _, output := range model.outputs {
		if output.Editable {
			count++
		}
	}
	return count
}

func (model colorPlanModel) selectionCount() int {
	return model.editableCount() + 3
}

func (model colorPlanModel) selectedOutputIndex() int {
	if model.cursor >= model.editableCount() {
		return -1
	}
	selected := 0
	for index, output := range model.outputs {
		if !output.Editable {
			continue
		}
		if selected == model.cursor {
			return index
		}
		selected++
	}
	return -1
}

func (model colorPlanModel) selectedSettingIndex() int {
	index := model.cursor - model.editableCount()
	if index < 0 || index >= 3 {
		return -1
	}
	return index
}

func (model *colorPlanModel) toggleLocalZ() {
	switch model.selectedSettingIndex() {
	case 0:
		model.localZ.LayerHeight = !model.localZ.LayerHeight
	case 1:
		model.localZ.Infill = !model.localZ.Infill
	case 2:
		model.localZ.WholeObjects = !model.localZ.WholeObjects
	}
}

func wrappedIndex(index, count int) int {
	if count == 0 {
		return 0
	}
	index %= count
	if index < 0 {
		index += count
	}
	return index
}

func (model colorPlanModel) outputIndex(slot int) int {
	for index, output := range model.outputs {
		if output.Slot == slot {
			return index
		}
	}
	return -1
}

func (model colorPlanModel) effectiveOutput(slot int) int {
	index := model.outputIndex(slot)
	if index < 0 {
		return slot
	}
	return model.outputs[index].ReplacementSlot
}

func (model colorPlanModel) View() string {
	if model.done {
		return ""
	}
	compact := model.width > 0 && model.width < 92
	leftWidth := 42
	if compact {
		leftWidth = 27
	}

	var output strings.Builder
	colorRows := max(len(model.sources), len(model.outputs))
	settingsStart := colorRows
	if colorRows > 0 {
		output.WriteString("Review color plan\n\n")
		output.WriteString(padVisible("Original order", leftWidth))
		output.WriteString("   New order / mode / replacement\n")
		settingsStart++
	} else {
		output.WriteString("Configure full-spectrum printing\n\n")
	}
	rowCount := settingsStart + 3
	start, end := model.visibleRange(rowCount)
	for index := start; index < end; index++ {
		if index == colorRows && colorRows > 0 {
			output.WriteString("Local-Z settings\n")
			continue
		}
		if index >= settingsStart {
			output.WriteString(model.localZView(index - settingsStart))
			output.WriteByte('\n')
			continue
		}
		left := ""
		if index < len(model.sources) {
			left = model.sourceView(model.sources[index], compact)
		}
		output.WriteString(padVisible(left, leftWidth))
		output.WriteString("   ")
		if index < len(model.outputs) {
			output.WriteString(model.outputView(index, compact))
		}
		output.WriteByte('\n')
	}
	return output.String()
}

func (model colorPlanModel) visibleRange(rowCount int) (int, int) {
	available := rowCount
	if model.height > 0 {
		available = max(1, model.height-4)
	}
	if available >= rowCount {
		return 0, rowCount
	}
	start := model.cursorDisplayRow() - available/2
	start = max(0, min(start, rowCount-available))
	return start, start + available
}

func (model colorPlanModel) cursorDisplayRow() int {
	if index := model.selectedOutputIndex(); index >= 0 {
		return index
	}
	settingsStart := max(len(model.sources), len(model.outputs))
	if settingsStart > 0 {
		settingsStart++
	}
	return settingsStart + model.selectedSettingIndex()
}

func (model colorPlanModel) sourceView(row ColorSourceRow, compact bool) string {
	target := model.effectiveOutput(row.OutputSlot)
	if compact {
		return fmt.Sprintf("T%d %s %s -> T%d", row.Slot, colorSwatch(row.Color), row.Color, target)
	}
	state := "unused"
	if row.Used {
		state = "used"
	}
	return fmt.Sprintf("T%d %s %s %-6s -> T%d", row.Slot, colorSwatch(row.Color), row.Color, state, target)
}

func (model colorPlanModel) outputView(index int, compact bool) string {
	row := model.outputs[index]
	cursor := "  "
	if index == model.selectedOutputIndex() {
		cursor = "> "
	}
	prefix := fmt.Sprintf("%sT%d %s %s", cursor, row.Slot, colorSwatch(row.Color), row.Color)
	replacement := "Keep"
	if row.ReplacementSlot != row.Slot {
		target := model.outputs[model.outputIndex(row.ReplacementSlot)]
		if compact {
			replacement = fmt.Sprintf("T%d", target.Slot)
		} else {
			replacement = fmt.Sprintf("T%d %s", target.Slot, target.Color)
		}
	}
	replacement = colorPlanControl(replacement, index == model.selectedOutputIndex() && model.focus == colorPlanReplacementFocus)
	kind := "Material"
	if row.Base {
		kind = "Base"
	}
	if !row.Editable {
		if row.Mixed {
			return fmt.Sprintf("%s  %-8s [%s]", prefix, "Mixed", model.options[row.Mode].Label)
		}
		return fmt.Sprintf("%s  %s", prefix, kind)
	}
	if !row.Mixed {
		return fmt.Sprintf("%s  %-8s %s", prefix, kind, replacement)
	}
	mode := colorPlanControl(model.options[row.Mode].Label, index == model.selectedOutputIndex() && model.focus == colorPlanModeFocus)
	return fmt.Sprintf("%s  %-8s %s %s", prefix, "Mixed", mode, replacement)
}

func (model colorPlanModel) localZView(index int) string {
	labels := [...]string{
		"Layer-height subdivision",
		"Subdivide infill",
		"Subdivide whole objects",
	}
	values := [...]bool{model.localZ.LayerHeight, model.localZ.Infill, model.localZ.WholeObjects}
	cursor := "  "
	selected := index == model.selectedSettingIndex()
	if selected {
		cursor = "> "
	}
	value := "Disabled"
	if values[index] {
		value = "Enabled"
	}
	return fmt.Sprintf("%s%-28s %s", cursor, labels[index], colorPlanControl(value, selected))
}

func colorPlanControl(label string, focused bool) string {
	if focused {
		return "\x1b[7m " + label + " \x1b[0m"
	}
	return "[" + label + "]"
}

func padVisible(value string, width int) string {
	padding := width - visibleWidth(value)
	if padding < 1 {
		padding = 1
	}
	return value + strings.Repeat(" ", padding)
}

func visibleWidth(value string) int {
	width := 0
	escape := false
	for _, character := range value {
		if escape {
			if character == 'm' {
				escape = false
			}
			continue
		}
		if character == '\x1b' {
			escape = true
			continue
		}
		width++
	}
	return width
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

func SelectColorPlan(ctx context.Context, input, output *os.File, sources []ColorSourceRow, outputs []ColorOutputRow, options []MixModeOption, localZ LocalZSelection) (ColorPlanResult, error) {
	if err := validateColorPlan(sources, outputs, options); err != nil {
		return ColorPlanResult{}, err
	}
	if !isTerminal(input) || !isTerminal(output) {
		return ColorPlanResult{}, fmt.Errorf("cannot review the color plan without an interactive terminal")
	}
	program := tea.NewProgram(
		newColorPlanModel(sources, outputs, options, localZ),
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	)
	final, err := program.Run()
	if err != nil {
		return ColorPlanResult{}, err
	}
	result := final.(colorPlanModel)
	if result.err != nil {
		return ColorPlanResult{}, result.err
	}
	return result.selection(), nil
}

func (model colorPlanModel) selection() ColorPlanResult {
	selections := make([]ColorPlanSelection, 0, model.editableCount())
	for _, row := range model.outputs {
		if !row.Editable {
			continue
		}
		mode := ""
		if row.Mixed {
			mode = model.options[row.Mode].Value
		}
		selections = append(selections, ColorPlanSelection{
			OutputSlot: row.Slot, Mode: mode, ReplacementSlot: row.ReplacementSlot,
		})
	}
	return ColorPlanResult{Colors: selections, LocalZ: model.localZ}
}

func validateColorPlan(sources []ColorSourceRow, outputs []ColorOutputRow, options []MixModeOption) error {
	if len(outputs) == 0 {
		if len(sources) != 0 || len(options) != 0 {
			return fmt.Errorf("color order review requires source colors, output colors, and mix modes")
		}
		return nil
	}
	if len(sources) == 0 {
		return fmt.Errorf("color order review requires source and output colors")
	}
	mixed := 0
	for index, row := range outputs {
		if row.Slot != index+1 || row.Color == "" {
			return fmt.Errorf("output color sequence is invalid")
		}
		if row.Base != (index < 4) || row.Base && row.Mixed {
			return fmt.Errorf("output color sequence must keep physical colors in T1-T4")
		}
		if row.Mixed {
			mixed++
		}
		if row.Mixed && (row.Mode < 0 || row.Mode >= len(options)) {
			return fmt.Errorf("mix mode selection is out of range")
		}
		if row.Editable && (row.ReplacementSlot < 1 || row.ReplacementSlot > len(outputs)) {
			return fmt.Errorf("color replacement is out of range")
		}
	}
	if mixed > 0 && len(options) == 0 {
		return fmt.Errorf("mixed color review requires mix modes")
	}
	for index, row := range sources {
		if row.Slot != index+1 || row.Color == "" || row.OutputSlot < 1 || row.OutputSlot > len(outputs) {
			return fmt.Errorf("source color sequence is invalid")
		}
	}
	return nil
}
