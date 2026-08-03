package threemf

import (
	"fmt"
	"sort"
	"strings"
)

type ColorRole string

const (
	ColorWhite  ColorRole = "white"
	ColorRed    ColorRole = "red"
	ColorBlue   ColorRole = "blue"
	ColorYellow ColorRole = "yellow"
	ColorBlack  ColorRole = "black"
)

var colorRoles = [...]ColorRole{ColorWhite, ColorRed, ColorBlue, ColorYellow, ColorBlack}

type Palette struct {
	Slots [4]ColorRole
}

func DefaultPalette() Palette {
	return Palette{Slots: [4]ColorRole{ColorBlue, ColorRed, ColorYellow, ColorBlack}}
}

func ParsePalette(order string) (Palette, error) {
	order = strings.ToLower(strings.TrimSpace(order))
	if len(order) != 4 {
		return Palette{}, fmt.Errorf("colors requires exactly four characters from wrbyk")
	}
	palette := Palette{}
	seen := make(map[ColorRole]bool, len(palette.Slots))
	for index := range order {
		role, ok := roleFromCode(order[index])
		if !ok {
			return Palette{}, fmt.Errorf("unsupported color code %q; use w, r, b, y, or k", order[index:index+1])
		}
		if seen[role] {
			return Palette{}, fmt.Errorf("color code %q appears more than once", order[index:index+1])
		}
		seen[role] = true
		palette.Slots[index] = role
	}
	return palette, nil
}

func (palette Palette) normalized() (Palette, error) {
	allEmpty := true
	for _, role := range palette.Slots {
		if role != "" {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		return DefaultPalette(), nil
	}
	seen := make(map[ColorRole]bool, len(colorRoles))
	for _, role := range palette.Slots {
		if !isColorRole(role) || seen[role] {
			return Palette{}, fmt.Errorf("palette must contain four different colors from wrbyk")
		}
		seen[role] = true
	}
	return palette, nil
}

func (palette Palette) validateFullSpectrum() error {
	for _, role := range []ColorRole{ColorRed, ColorYellow, ColorBlue} {
		if palette.slot(role) == 0 {
			return fmt.Errorf("--full-spectrum colors must contain r, y, and b")
		}
	}
	hasWhite := palette.slot(ColorWhite) > 0
	hasBlack := palette.slot(ColorBlack) > 0
	if hasWhite == hasBlack {
		return fmt.Errorf("--full-spectrum colors must contain exactly one of w or k")
	}
	return nil
}

func (palette Palette) neutralRole() ColorRole {
	if palette.slot(ColorWhite) > 0 {
		return ColorWhite
	}
	return ColorBlack
}

func (palette Palette) slot(role ColorRole) int {
	for index, candidate := range palette.Slots {
		if candidate == role {
			return index + 1
		}
	}
	return 0
}

func (palette Palette) outputColors() []string {
	colors := make([]string, len(palette.Slots))
	for index, role := range palette.Slots {
		colors[index] = roleHex(role)
	}
	return colors
}

func (palette Palette) matchDistance(source [3]int, role ColorRole) int {
	distance := colorDistance(source, roleRGB(role))
	if role == ColorBlack && palette.slot(ColorWhite) == 0 {
		whiteDistance := colorDistance(source, roleRGB(ColorWhite))
		if whiteDistance < distance {
			return whiteDistance
		}
	}
	return distance
}

type colorComponent struct {
	role   ColorRole
	weight float64
}

func decomposeRYB(color [3]int, palette Palette) []colorComponent {
	r := float64(color[0]) / 255
	g := float64(color[1]) / 255
	b := float64(color[2]) / 255
	if r == 0 && g == 0 && b == 0 {
		if palette.slot(ColorBlack) > 0 {
			return []colorComponent{{role: ColorBlack, weight: 1}}
		}
		return []colorComponent{
			{role: ColorRed, weight: 1},
			{role: ColorYellow, weight: 1},
			{role: ColorBlue, weight: 1},
		}
	}

	white := min(r, g, b)
	r -= white
	g -= white
	b -= white
	maxGreen := max(r, g, b)
	yellow := min(r, g)
	r -= yellow
	g -= yellow
	if b > 0 && g > 0 {
		b /= 2
		g /= 2
	}
	yellow += g
	b += g
	maxYellow := max(r, yellow, b)
	if maxYellow > 0 {
		normalize := maxGreen / maxYellow
		r *= normalize
		yellow *= normalize
		b *= normalize
	}

	components := []colorComponent{
		{role: palette.neutralRole(), weight: white},
		{role: ColorRed, weight: r},
		{role: ColorYellow, weight: yellow},
		{role: ColorBlue, weight: b},
	}
	total := white + r + yellow + b
	filtered := components[:0]
	for _, component := range components {
		if component.weight > total*0.005 {
			filtered = append(filtered, component)
		}
	}
	if len(filtered) > 3 {
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].weight > filtered[j].weight
		})
		filtered = filtered[:3]
		sort.SliceStable(filtered, func(i, j int) bool {
			return mixRoleIndex(filtered[i].role) < mixRoleIndex(filtered[j].role)
		})
	}
	return filtered
}

func normalizedPercentages(values []float64) []int {
	total := 0.0
	for _, value := range values {
		total += value
	}
	percentages := make([]int, len(values))
	remainders := make([]float64, len(values))
	allocated := 0
	for index, value := range values {
		exact := value * 100 / total
		percentages[index] = int(exact)
		remainders[index] = exact - float64(percentages[index])
		allocated += percentages[index]
	}
	for allocated < 100 {
		best := 0
		for index := 1; index < len(remainders); index++ {
			if remainders[index] > remainders[best] {
				best = index
			}
		}
		percentages[best]++
		remainders[best] = -1
		allocated++
	}
	return percentages
}

func isColorRole(role ColorRole) bool {
	return role == ColorWhite || role == ColorRed || role == ColorBlue || role == ColorYellow || role == ColorBlack
}

func mixRoleIndex(role ColorRole) int {
	switch role {
	case ColorWhite, ColorBlack:
		return 0
	case ColorRed:
		return 1
	case ColorYellow:
		return 2
	case ColorBlue:
		return 3
	}
	return len(colorRoles)
}

func roleFromCode(code byte) (ColorRole, bool) {
	switch code {
	case 'w':
		return ColorWhite, true
	case 'r':
		return ColorRed, true
	case 'b':
		return ColorBlue, true
	case 'y':
		return ColorYellow, true
	case 'k':
		return ColorBlack, true
	default:
		return "", false
	}
}

func roleHex(role ColorRole) string {
	switch role {
	case ColorWhite:
		return "#FFFFFF"
	case ColorRed:
		return "#FF0000"
	case ColorYellow:
		return "#FFFF00"
	case ColorBlue:
		return "#0000FF"
	case ColorBlack:
		return "#000000"
	default:
		return ""
	}
}

func roleRGB(role ColorRole) [3]int {
	color, _ := parseColor(roleHex(role))
	return color
}
