package threemf

import (
	"fmt"
	"sort"
	"strings"
)

type ColorRole string

const (
	ColorCyan    ColorRole = "cyan"
	ColorMagenta ColorRole = "magenta"
	ColorYellow  ColorRole = "yellow"
	ColorGray    ColorRole = "gray"
	ColorWhite   ColorRole = "white"
	ColorBlack   ColorRole = "black"
)

var colorRoles = [...]ColorRole{ColorCyan, ColorMagenta, ColorYellow, ColorGray, ColorWhite, ColorBlack}

type Palette struct {
	Slots [4]ColorRole
}

func DefaultPalette() Palette {
	return Palette{Slots: [4]ColorRole{ColorCyan, ColorMagenta, ColorYellow, ColorGray}}
}

func ParsePalette(order string) (Palette, error) {
	order = strings.ToLower(strings.TrimSpace(order))
	if len(order) != 4 {
		return Palette{}, fmt.Errorf("colors requires exactly four characters from cmygwb")
	}
	palette := Palette{}
	seen := make(map[ColorRole]bool, len(palette.Slots))
	for index := range order {
		role, ok := roleFromCode(order[index])
		if !ok {
			return Palette{}, fmt.Errorf("unsupported color code %q; use c, m, y, g, w, or b", order[index:index+1])
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
			return Palette{}, fmt.Errorf("palette must contain four different colors from cmygwb")
		}
		seen[role] = true
	}
	return palette, nil
}

func (palette Palette) neutralRole() ColorRole {
	if palette.slot(ColorGray) > 0 {
		return ColorGray
	}
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

func (palette Palette) String() string {
	var result strings.Builder
	for _, role := range palette.Slots {
		result.WriteByte(roleCode(role))
	}
	return result.String()
}

func exactColorRole(color [3]int) (ColorRole, bool) {
	for _, role := range colorRoles {
		if color == roleRGB(role) {
			return role, true
		}
	}
	return "", false
}

func baseColorRole(color [3]int) (ColorRole, bool) {
	if max(color[0], color[1], color[2])-min(color[0], color[1], color[2]) > 48 {
		role := ColorCyan
		distance := colorDistance(color, roleRGB(role))
		for _, candidate := range []ColorRole{ColorMagenta, ColorYellow} {
			candidateDistance := colorDistance(color, roleRGB(candidate))
			if candidateDistance < distance {
				role = candidate
				distance = candidateDistance
			}
		}
		return role, distance <= 3*72*72
	}
	role, distance := nearestColorRole(color)
	threshold := 3 * 64 * 64
	if role == ColorGray {
		threshold = 3 * 48 * 48
	}
	return role, distance <= threshold
}

func nearestColorRole(color [3]int) (ColorRole, int) {
	best := colorRoles[0]
	bestDistance := colorDistance(color, roleRGB(best))
	for _, role := range colorRoles[1:] {
		distance := colorDistance(color, roleRGB(role))
		if distance < bestDistance {
			best = role
			bestDistance = distance
		}
	}
	return best, bestDistance
}

func neutralColorRole(color [3]int) (ColorRole, bool) {
	grayDistance := colorDistance(color, roleRGB(ColorGray))
	whiteDistance := colorDistance(color, roleRGB(ColorWhite))
	blackDistance := colorDistance(color, roleRGB(ColorBlack))
	const neutralThreshold = 3 * 64 * 64
	if grayDistance <= 3*48*48 && grayDistance < whiteDistance && grayDistance < blackDistance {
		return ColorGray, true
	}
	if whiteDistance <= neutralThreshold && whiteDistance < blackDistance {
		return ColorWhite, true
	}
	if blackDistance <= neutralThreshold {
		return ColorBlack, true
	}
	return "", false
}

func (palette Palette) matchDistance(source [3]int, role ColorRole) int {
	distance := colorDistance(source, roleRGB(role))
	if classified, base := baseColorRole(source); base && classified != role {
		distance += 3 * 255 * 255
	}
	return distance
}

type colorComponent struct {
	role   ColorRole
	weight float64
}

func decomposeCMY(color [3]int, palette Palette) []colorComponent {
	r := float64(color[0]) / 255
	g := float64(color[1]) / 255
	b := float64(color[2]) / 255
	if r == 0 && g == 0 && b == 0 {
		if palette.slot(ColorBlack) > 0 {
			return []colorComponent{{role: ColorBlack, weight: 1}}
		}
		return []colorComponent{
			{role: ColorMagenta, weight: 1},
			{role: ColorYellow, weight: 1},
			{role: ColorCyan, weight: 1},
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
		{role: ColorMagenta, weight: r},
		{role: ColorYellow, weight: yellow},
		{role: ColorCyan, weight: b},
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
	return role == ColorCyan || role == ColorMagenta || role == ColorYellow || role == ColorGray || role == ColorWhite || role == ColorBlack
}

func mixRoleIndex(role ColorRole) int {
	switch role {
	case ColorGray, ColorWhite, ColorBlack:
		return 0
	case ColorMagenta:
		return 1
	case ColorYellow:
		return 2
	case ColorCyan:
		return 3
	}
	return len(colorRoles)
}

func roleFromCode(code byte) (ColorRole, bool) {
	switch code {
	case 'c':
		return ColorCyan, true
	case 'm':
		return ColorMagenta, true
	case 'y':
		return ColorYellow, true
	case 'g':
		return ColorGray, true
	case 'w':
		return ColorWhite, true
	case 'b':
		return ColorBlack, true
	default:
		return "", false
	}
}

func roleCode(role ColorRole) byte {
	switch role {
	case ColorCyan:
		return 'c'
	case ColorMagenta:
		return 'm'
	case ColorYellow:
		return 'y'
	case ColorGray:
		return 'g'
	case ColorWhite:
		return 'w'
	case ColorBlack:
		return 'b'
	default:
		return '?'
	}
}

func roleHex(role ColorRole) string {
	switch role {
	case ColorCyan:
		return "#0000FF"
	case ColorMagenta:
		return "#FF0000"
	case ColorYellow:
		return "#FFFF00"
	case ColorGray:
		return "#808080"
	case ColorWhite:
		return "#FFFFFF"
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
