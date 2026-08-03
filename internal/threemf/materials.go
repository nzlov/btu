package threemf

import (
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type materialPlan struct {
	mode            string
	physicalMapping map[int]int
	allMapping      map[int]int
	definitions     string
	virtualMixes    int
	hasMultiColor   bool
	forceLocalZ     bool
	palette         Palette
}

type ColorMapping struct {
	MaterialIDs []int
	Color       string
	Used        bool
	Base        bool
	Suggested   ColorRole
}

type FullSpectrumRequiredError struct {
	ColorCount   int
	NonBaseCount int
	Mappings     []ColorMapping
}

func (err *FullSpectrumRequiredError) Error() string {
	if err.NonBaseCount > 0 {
		return fmt.Sprintf("source declares %d colors including %d non-base colors; color mapping is required", err.ColorCount, err.NonBaseCount)
	}
	return fmt.Sprintf("source uses %d colors; full-spectrum mixing is required", err.ColorCount)
}

func planMaterials(source, template map[string]any, palette Palette, fullSpectrum bool, usage materialUsage, targets map[int]string) (materialPlan, error) {
	for _, key := range []string{"filament_mixed_gradient_curve", "filament_mixed_gradient_per_part"} {
		if hasEnabledValue(source[key]) {
			return materialPlan{}, fmt.Errorf("%s is not representable by U1", key)
		}
	}
	flags := stringSlice(source["filament_is_mixed"])
	firstMixed := -1
	for index, flag := range flags {
		if flag == "1" {
			firstMixed = index
			break
		}
	}
	if firstMixed < 0 {
		colors := stringSlice(source["filament_colour"])
		usage = normalizedPhysicalUsage(usage, len(colors))
		mappings, nonBaseCount, err := inspectColorMappings(colors, usage)
		if err != nil {
			return materialPlan{}, err
		}
		if nonBaseCount > 0 && !fullSpectrum && len(targets) == 0 {
			return materialPlan{}, &FullSpectrumRequiredError{
				ColorCount:   len(mappings),
				NonBaseCount: nonBaseCount,
				Mappings:     mappings,
			}
		}
		return planMappedMaterials(source, template, palette, usage, targets)
	}
	if fullSpectrum || len(targets) > 0 {
		return materialPlan{}, fmt.Errorf("color mapping cannot replace a project that already has native mixed materials")
	}

	for index := firstMixed; index < len(flags); index++ {
		if flags[index] != "1" {
			return materialPlan{}, fmt.Errorf("physical material T%d appears after mixed materials", index+1)
		}
	}
	physicalCount := firstMixed
	if physicalCount < 2 {
		return materialPlan{}, fmt.Errorf("native mixed project has only %d physical materials", physicalCount)
	}
	physicalUsage := make(materialUsage, physicalCount)
	for material := 1; material <= physicalCount; material++ {
		physicalUsage[material] = 1
	}
	palette, err := selectLayeredPalette(source, stringSlice(template["filament_type"]), palette, physicalUsage, physicalCount)
	if err != nil {
		return materialPlan{}, err
	}
	physicalMapping, err := mapPhysicalMaterials(source, template, physicalCount, palette, physicalUsage)
	if err != nil {
		return materialPlan{}, err
	}
	allMapping := cloneMapping(physicalMapping)
	targetPhysicalCount := len(stringSlice(template["filament_colour"]))
	components := stringSlice(source["filament_mixed_components"])
	ratios := stringSlice(source["filament_mixed_sublayer_ratios"])
	gradients := stringSlice(source["filament_mixed_gradient"])
	gradientRanges := stringSlice(source["filament_mixed_gradient_range"])
	definitions := make([]string, 0, len(flags)-physicalCount)
	hasMultiColor := false

	for index := physicalCount; index < len(flags); index++ {
		componentIDs, err := parseIDs(sliceAt(components, index))
		if err != nil {
			return materialPlan{}, fmt.Errorf("mixed material T%d components: %w", index+1, err)
		}
		if len(componentIDs) < 2 || len(componentIDs) > 3 {
			return materialPlan{}, fmt.Errorf("mixed material T%d has %d components; U1 supports two or three", index+1, len(componentIDs))
		}
		mappedComponents := make([]int, len(componentIDs))
		for componentIndex, id := range componentIDs {
			mapped, ok := physicalMapping[id]
			if !ok {
				return materialPlan{}, fmt.Errorf("mixed material T%d references non-physical material T%d", index+1, id)
			}
			mappedComponents[componentIndex] = mapped
		}
		weights, err := parseWeights(sliceAt(ratios, index), len(componentIDs))
		if err != nil {
			return materialPlan{}, fmt.Errorf("mixed material T%d ratios: %w", index+1, err)
		}
		stableID := index - physicalCount + 1
		gradient := sliceAt(gradients, index) == "1"
		definition, err := makeDefinition(mappedComponents, weights, gradient, sliceAt(gradientRanges, index), stableID)
		if err != nil {
			return materialPlan{}, fmt.Errorf("mixed material T%d: %w", index+1, err)
		}
		if len(componentIDs) == 3 {
			hasMultiColor = true
		}
		definitions = append(definitions, definition)
		allMapping[index+1] = targetPhysicalCount + stableID
	}

	return materialPlan{
		mode:            "native-mixed",
		physicalMapping: physicalMapping,
		allMapping:      allMapping,
		definitions:     strings.Join(definitions, ";"),
		virtualMixes:    len(definitions),
		hasMultiColor:   hasMultiColor,
		palette:         palette,
	}, nil
}

func planMappedMaterials(source, template map[string]any, palette Palette, usage materialUsage, targets map[int]string) (materialPlan, error) {
	colors := stringSlice(source["filament_colour"])
	if len(colors) == 0 {
		return materialPlan{}, fmt.Errorf("source has no filament colors")
	}
	if len(stringSlice(template["filament_colour"])) != len(palette.Slots) {
		return materialPlan{}, fmt.Errorf("U1 template must have exactly four physical material slots")
	}
	usage = normalizedPhysicalUsage(usage, len(colors))
	targetColors, err := resolveTargetColors(colors, targets)
	if err != nil {
		return materialPlan{}, err
	}
	palette, recipes, err := selectColorRecipes(targetColors, palette, usage)
	if err != nil {
		return materialPlan{}, err
	}

	mapping := make(map[int]int, len(colors))
	definitions := make([]string, 0, len(colors))
	mixedTargets := make(map[string]int)
	hasMultiColor := false
	for index, recipe := range recipes {
		if len(recipe.components) == 1 {
			mapping[index+1] = recipe.components[0]
			continue
		}

		signature := mixSignature(recipe.components, recipe.weights)
		if target, ok := mixedTargets[signature]; ok {
			mapping[index+1] = target
			continue
		}

		stableID := len(definitions) + 1
		definition, err := makeDefinition(recipe.components, recipe.weights, false, "", stableID)
		if err != nil {
			return materialPlan{}, fmt.Errorf("source T%d target %s: %w", index+1, canonicalColor(targetColors[index]), err)
		}
		if len(recipe.components) >= 3 {
			hasMultiColor = true
		}
		definitions = append(definitions, definition)
		target := len(palette.Slots) + stableID
		mixedTargets[signature] = target
		mapping[index+1] = target
	}
	mode := "layered"
	if len(definitions) > 0 {
		mode = "full-spectrum"
	}
	return materialPlan{
		mode:            mode,
		physicalMapping: cloneMapping(mapping),
		allMapping:      mapping,
		definitions:     strings.Join(definitions, ";"),
		virtualMixes:    len(definitions),
		hasMultiColor:   hasMultiColor,
		forceLocalZ:     len(definitions) > 0,
		palette:         palette,
	}, nil
}

func inspectColorMappings(colors []string, usage materialUsage) ([]ColorMapping, int, error) {
	mappings := make([]ColorMapping, 0, len(colors))
	byColor := make(map[string]int, len(colors))
	nonBaseCount := 0
	for index, value := range colors {
		color, err := parseColor(value)
		if err != nil {
			return nil, 0, fmt.Errorf("source T%d: %w", index+1, err)
		}
		canonical := canonicalColor(color)
		if mappingIndex, exists := byColor[canonical]; exists {
			mappings[mappingIndex].MaterialIDs = append(mappings[mappingIndex].MaterialIDs, index+1)
			if usage[index+1] > 0 {
				mappings[mappingIndex].Used = true
			}
			continue
		}
		suggested, base := baseColorRole(color)
		if !base {
			nonBaseCount++
			suggested = suggestedMixedReplacement(color)
		}
		byColor[canonical] = len(mappings)
		mappings = append(mappings, ColorMapping{
			MaterialIDs: []int{index + 1},
			Color:       canonical,
			Used:        usage[index+1] > 0,
			Base:        base,
			Suggested:   suggested,
		})
	}
	return mappings, nonBaseCount, nil
}

func suggestedMixedReplacement(color [3]int) ColorRole {
	parts := decomposeCMY(color, DefaultPalette())
	if len(parts) == 0 {
		role, _ := nearestColorRole(color)
		return role
	}
	best := parts[0]
	for _, part := range parts[1:] {
		if part.weight > best.weight {
			best = part
		}
	}
	return best.role
}

func resolveTargetColors(colors []string, targets map[int]string) ([][3]int, error) {
	for material := range targets {
		if material < 1 || material > len(colors) {
			return nil, fmt.Errorf("color mapping references unknown source T%d", material)
		}
	}
	result := make([][3]int, len(colors))
	for index, sourceColor := range colors {
		value := sourceColor
		if target := targets[index+1]; target != "" {
			value = target
		}
		color, err := parseColor(value)
		if err != nil {
			return nil, fmt.Errorf("source T%d target: %w", index+1, err)
		}
		result[index] = color
	}
	return result, nil
}

func paletteCandidates(preferred Palette) []Palette {
	result := make([]Palette, 0, 15)
	for first := 0; first < len(colorRoles)-3; first++ {
		for second := first + 1; second < len(colorRoles)-2; second++ {
			for third := second + 1; third < len(colorRoles)-1; third++ {
				for fourth := third + 1; fourth < len(colorRoles); fourth++ {
					result = append(result, arrangePalette(preferred, [4]ColorRole{
						colorRoles[first], colorRoles[second], colorRoles[third], colorRoles[fourth],
					}))
				}
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		leftChanges, leftPenalty := palettePreference(result[i], preferred)
		rightChanges, rightPenalty := palettePreference(result[j], preferred)
		if leftChanges != rightChanges {
			return leftChanges < rightChanges
		}
		if leftPenalty != rightPenalty {
			return leftPenalty < rightPenalty
		}
		return result[i].String() < result[j].String()
	})
	return result
}

func arrangePalette(preferred Palette, roles [4]ColorRole) Palette {
	selected := make(map[ColorRole]bool, len(roles))
	for _, role := range roles {
		selected[role] = true
	}
	result := Palette{}
	for index, role := range preferred.Slots {
		if selected[role] {
			result.Slots[index] = role
			delete(selected, role)
		}
	}
	missing := make([]ColorRole, 0, len(selected))
	for _, role := range colorRoles {
		if selected[role] {
			missing = append(missing, role)
		}
	}
	missingIndex := 0
	for index, role := range result.Slots {
		if role == "" {
			result.Slots[index] = missing[missingIndex]
			missingIndex++
		}
	}
	return result
}

func palettePreference(candidate, preferred Palette) (int, int) {
	changes := 0
	for _, role := range preferred.Slots {
		if candidate.slot(role) == 0 {
			changes++
		}
	}
	primaryPenalty := 0
	for _, role := range []ColorRole{ColorCyan, ColorMagenta, ColorYellow} {
		if candidate.slot(role) == 0 {
			primaryPenalty += 4
		}
	}
	if candidate.slot(ColorGray) == 0 {
		primaryPenalty++
	}
	return changes, primaryPenalty
}

func colorParts(color [3]int, palette Palette) ([]colorComponent, bool) {
	if role, base := baseColorRole(color); base {
		if palette.slot(role) > 0 {
			return []colorComponent{{role: role, weight: 1}}, true
		}
		if role == ColorBlack && palette.slot(ColorCyan) > 0 && palette.slot(ColorMagenta) > 0 && palette.slot(ColorYellow) > 0 {
			return []colorComponent{{role: ColorMagenta, weight: 1}, {role: ColorYellow, weight: 1}, {role: ColorCyan, weight: 1}}, true
		}
		if role == ColorGray && palette.slot(ColorWhite) > 0 && palette.slot(ColorBlack) > 0 {
			return []colorComponent{{role: ColorWhite, weight: 1}, {role: ColorBlack, weight: 1}}, true
		}
		return nil, false
	}
	parts := decomposeCMY(color, palette)
	if len(parts) == 0 {
		return nil, false
	}
	for _, part := range parts {
		if palette.slot(part.role) == 0 {
			return nil, false
		}
	}
	return parts, true
}

func canonicalColor(color [3]int) string {
	return fmt.Sprintf("#%02X%02X%02X", color[0], color[1], color[2])
}

func mixSignature(components, weights []int) string {
	componentText := make([]string, len(components))
	weightText := make([]string, len(weights))
	for index := range components {
		componentText[index] = strconv.Itoa(components[index])
		weightText[index] = strconv.Itoa(weights[index])
	}
	return strings.Join(componentText, "/") + ":" + strings.Join(weightText, "/")
}

func hasEnabledValue(value any) bool {
	switch value := value.(type) {
	case string:
		return value != "" && value != "0"
	case []any:
		for _, item := range value {
			if hasEnabledValue(item) {
				return true
			}
		}
	case []string:
		for _, item := range value {
			if hasEnabledValue(item) {
				return true
			}
		}
	}
	return false
}

func makeDefinition(components, weights []int, gradient bool, gradientRange string, stableID int) (string, error) {
	if gradient {
		if len(components) != 2 {
			return "", fmt.Errorf("gradient mixing with %d components cannot be represented by U1", len(components))
		}
		parts := strings.Split(gradientRange, ",")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid gradient range %q", gradientRange)
		}
		start, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return "", fmt.Errorf("invalid gradient start %q", parts[0])
		}
		end, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return "", fmt.Errorf("invalid gradient end %q", parts[1])
		}
		return fmt.Sprintf("%d,%d,1,1,50,0,g,w,m0,z2,xa0,xb0,d0,o0,u%d,cm3,r1/%.4f/%.4f", components[0], components[1], stableID, start, end), nil
	}

	if len(components) == 2 {
		return fmt.Sprintf("%d,%d,1,1,%d,0,g,w,m2,z0,xa0,xb0,d0,o0,u%d,cm0", components[0], components[1], weights[1], stableID), nil
	}
	componentText := make([]string, len(components))
	weightText := make([]string, len(weights))
	for index := range components {
		componentText[index] = strconv.Itoa(components[index])
		weightText[index] = strconv.Itoa(weights[index])
	}
	return fmt.Sprintf("%d,%d,1,1,50,0,g%s,w%s,m0,z0,xa0,xb0,d0,o0,u%d,cm0", components[0], components[1], strings.Join(componentText, ""), strings.Join(weightText, "/"), stableID), nil
}

func parseIDs(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid material ID %q", part)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseWeights(value string, count int) ([]int, error) {
	var positive []float64
	for _, part := range strings.Split(value, ",") {
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", part)
		}
		if number > 0 {
			positive = append(positive, number)
		}
	}
	if len(positive) != count {
		return nil, fmt.Errorf("found %d positive ratios for %d components", len(positive), count)
	}
	total := 0.0
	for _, number := range positive {
		total += number
	}
	type remainder struct {
		index int
		value float64
	}
	weights := make([]int, count)
	remainders := make([]remainder, count)
	allocated := 0
	for index, number := range positive {
		exact := number * 100 / total
		weights[index] = int(math.Floor(exact))
		allocated += weights[index]
		remainders[index] = remainder{index: index, value: exact - float64(weights[index])}
	}
	for allocated < 100 {
		best := 0
		for index := 1; index < len(remainders); index++ {
			if remainders[index].value > remainders[best].value {
				best = index
			}
		}
		weights[remainders[best].index]++
		remainders[best].value = -1
		allocated++
	}
	return weights, nil
}

type materialGroup struct {
	ids    []int
	color  [3]int
	typeID string
	weight float64
}

func normalizedPhysicalUsage(usage materialUsage, count int) materialUsage {
	result := make(materialUsage)
	for material, weight := range usage {
		if material > 0 && material <= count && weight > 0 {
			result.add(material, weight)
		}
	}
	if len(result) == 0 {
		for material := range usage {
			if material > 0 && material <= count {
				result[material] = 1
			}
		}
		if len(result) == 0 {
			for material := 1; material <= count; material++ {
				result[material] = 1
			}
		}
	}
	return result
}

func groupUsedMaterials(source map[string]any, count int, usage materialUsage) ([]materialGroup, error) {
	sourceColors := stringSlice(source["filament_colour"])
	sourceTypes := stringSlice(source["filament_type"])
	if count == 0 || len(sourceColors) < count || len(sourceTypes) < count {
		return nil, fmt.Errorf("source physical material metadata is incomplete")
	}
	groups := make([]materialGroup, 0, len(usage))
	groupByKey := make(map[string]int, len(usage))
	for material := 1; material <= count; material++ {
		weight, used := usage[material]
		if !used {
			continue
		}
		color, err := parseColor(sourceColors[material-1])
		if err != nil {
			return nil, fmt.Errorf("source T%d: %w", material, err)
		}
		key := fmt.Sprintf("%d/%d/%d/%s", color[0], color[1], color[2], strings.ToLower(sourceTypes[material-1]))
		if index, exists := groupByKey[key]; exists {
			groups[index].ids = append(groups[index].ids, material)
			groups[index].weight += weight
			continue
		}
		groupByKey[key] = len(groups)
		groups = append(groups, materialGroup{
			ids:    []int{material},
			color:  color,
			typeID: sourceTypes[material-1],
			weight: weight,
		})
	}
	return groups, nil
}

func mapPhysicalMaterials(source, template map[string]any, count int, palette Palette, usage materialUsage) (map[int]int, error) {
	sourceColors := stringSlice(source["filament_colour"])
	sourceTypes := stringSlice(source["filament_type"])
	templateColors := stringSlice(template["filament_colour"])
	templateTypes := stringSlice(template["filament_type"])
	if count == 0 || len(sourceColors) < count || len(sourceTypes) < count {
		return nil, fmt.Errorf("source physical material metadata is incomplete")
	}
	if len(templateColors) != len(palette.Slots) || len(templateTypes) < len(palette.Slots) {
		return nil, fmt.Errorf("U1 template must have exactly four physical material slots")
	}
	usage = normalizedPhysicalUsage(usage, count)
	groups, err := groupUsedMaterials(source, count, usage)
	if err != nil {
		return nil, err
	}
	if len(groups) > len(palette.Slots) {
		return nil, fmt.Errorf("native mixed project uses %d physical colors; U1 supports four", len(groups))
	}

	best, _, err := assignMaterialGroups(groups, templateTypes, palette)
	if err != nil {
		return nil, err
	}
	mapping := make(map[int]int, count)
	for index, target := range best {
		for _, material := range groups[index].ids {
			mapping[material] = target
		}
	}
	for material := 1; material <= count; material++ {
		if mapping[material] > 0 {
			continue
		}
		color, err := parseColor(sourceColors[material-1])
		if err != nil {
			return nil, fmt.Errorf("source T%d: %w", material, err)
		}
		bestSlot := 0
		if role, neutral := neutralColorRole(color); neutral {
			switch role {
			case ColorWhite:
				bestSlot = palette.slot(ColorBlack)
			case ColorBlack:
				bestSlot = palette.slot(ColorWhite)
			}
		}
		if bestSlot > 0 && !strings.EqualFold(sourceTypes[material-1], templateTypes[bestSlot-1]) {
			bestSlot = 0
		}
		bestDistance := math.MaxInt
		for slot := range templateColors {
			if bestSlot > 0 {
				break
			}
			if !strings.EqualFold(sourceTypes[material-1], templateTypes[slot]) {
				continue
			}
			distance := palette.matchDistance(color, palette.Slots[slot])
			if distance < bestDistance {
				bestDistance = distance
				bestSlot = slot + 1
			}
		}
		if bestSlot == 0 {
			for slot := range templateColors {
				distance := palette.matchDistance(color, palette.Slots[slot])
				if distance < bestDistance {
					bestDistance = distance
					bestSlot = slot + 1
				}
			}
		}
		mapping[material] = bestSlot
	}
	return mapping, nil
}

func assignMaterialGroups(groups []materialGroup, templateTypes []string, palette Palette) ([]int, float64, error) {
	bestCost := math.Inf(1)
	best := make([]int, len(groups))
	current := make([]int, len(groups))
	if len(templateTypes) != len(palette.Slots) {
		return nil, 0, fmt.Errorf("U1 template must have exactly four physical material slots")
	}
	used := make([]bool, len(templateTypes))
	var search func(int, float64)
	search = func(sourceIndex int, cost float64) {
		if cost >= bestCost {
			return
		}
		if sourceIndex == len(groups) {
			bestCost = cost
			copy(best, current)
			return
		}
		for templateIndex := range templateTypes {
			if used[templateIndex] || !strings.EqualFold(groups[sourceIndex].typeID, templateTypes[templateIndex]) {
				continue
			}
			distance := float64(palette.matchDistance(groups[sourceIndex].color, palette.Slots[templateIndex])) * groups[sourceIndex].weight
			used[templateIndex] = true
			current[sourceIndex] = templateIndex + 1
			search(sourceIndex+1, cost+distance)
			used[templateIndex] = false
		}
	}
	search(0, 0)
	if math.IsInf(bestCost, 1) {
		return nil, 0, fmt.Errorf("cannot match source material types to template slots")
	}
	return best, bestCost, nil
}

func selectLayeredPalette(source map[string]any, templateTypes []string, palette Palette, usage materialUsage, count int) (Palette, error) {
	groups, err := groupUsedMaterials(source, count, usage)
	if err != nil {
		return Palette{}, err
	}
	bestCost := math.Inf(1)
	best := Palette{}
	for _, candidate := range paletteCandidates(palette) {
		_, cost, candidateErr := assignMaterialGroups(groups, templateTypes, candidate)
		if candidateErr != nil {
			continue
		}
		if cost < bestCost {
			best = candidate
			bestCost = cost
		}
	}
	if math.IsInf(bestCost, 1) {
		return Palette{}, fmt.Errorf("cannot match source material types to template slots")
	}
	return best, nil
}

func parseColor(value string) ([3]int, error) {
	var result [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 && len(value) != 8 {
		return result, fmt.Errorf("invalid color %q", value)
	}
	decoded, err := hex.DecodeString(value[:6])
	if err != nil {
		return result, fmt.Errorf("invalid color %q", value)
	}
	for index := range result {
		result[index] = int(decoded[index])
	}
	return result, nil
}

func colorDistance(a, b [3]int) int {
	dr := a[0] - b[0]
	dg := a[1] - b[1]
	db := a[2] - b[2]
	return dr*dr + dg*dg + db*db
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return strings
		}
		return nil
	}
	result := make([]string, len(items))
	for index, item := range items {
		result[index], _ = item.(string)
	}
	return result
}

func sliceAt(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return ""
}

func cloneMapping(mapping map[int]int) map[int]int {
	result := make(map[int]int, len(mapping))
	for source, target := range mapping {
		result[source] = target
	}
	return result
}
