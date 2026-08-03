package threemf

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type materialPlan struct {
	mode            string
	physicalMapping map[int]int
	allMapping      map[int]int
	definitions     string
	virtualMixes    int
	hasThreeColor   bool
	forceLocalZ     bool
	palette         Palette
}

type FullSpectrumRequiredError struct {
	ColorCount int
}

func (err *FullSpectrumRequiredError) Error() string {
	return fmt.Sprintf("source uses %d colors; full-spectrum mixing is required", err.ColorCount)
}

func planMaterials(source, template map[string]any, palette Palette, fullSpectrum bool, usage materialUsage) (materialPlan, error) {
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
	if fullSpectrum {
		if firstMixed >= 0 {
			return materialPlan{}, fmt.Errorf("--full-spectrum cannot replace a project that already has native mixed materials")
		}
		palette = selectFullSpectrumPalette(source, palette, usage)
		if err := palette.validateFullSpectrum(); err != nil {
			return materialPlan{}, err
		}
		return planFullSpectrumMaterials(source, template, palette, usage)
	}
	if firstMixed < 0 {
		colors := stringSlice(source["filament_colour"])
		usage = normalizedPhysicalUsage(usage, len(colors))
		groups, err := groupUsedMaterials(source, len(colors), usage)
		if err != nil {
			return materialPlan{}, err
		}
		if len(groups) > len(palette.Slots) {
			return materialPlan{}, &FullSpectrumRequiredError{ColorCount: len(groups)}
		}
		palette, err = selectLayeredPalette(source, stringSlice(template["filament_type"]), palette, usage, len(colors))
		if err != nil {
			return materialPlan{}, err
		}
		mapping, err := mapPhysicalMaterials(source, template, len(colors), palette, usage)
		if err != nil {
			return materialPlan{}, err
		}
		return materialPlan{
			mode:            "layered",
			physicalMapping: mapping,
			allMapping:      cloneMapping(mapping),
			palette:         palette,
		}, nil
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
	hasThreeColor := false

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
			hasThreeColor = true
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
		hasThreeColor:   hasThreeColor,
		palette:         palette,
	}, nil
}

func planFullSpectrumMaterials(source, template map[string]any, palette Palette, usage materialUsage) (materialPlan, error) {
	colors := stringSlice(source["filament_colour"])
	if len(colors) == 0 {
		return materialPlan{}, fmt.Errorf("source has no filament colors")
	}
	if len(stringSlice(template["filament_colour"])) != len(palette.Slots) {
		return materialPlan{}, fmt.Errorf("U1 template must have exactly four physical material slots")
	}
	usage = normalizedPhysicalUsage(usage, len(colors))

	mapping := make(map[int]int, len(colors))
	definitions := make([]string, 0, len(colors))
	mixedTargets := make(map[string]int)
	hasThreeColor := false
	for index, value := range colors {
		if _, used := usage[index+1]; !used {
			continue
		}
		color, err := parseColor(value)
		if err != nil {
			return materialPlan{}, fmt.Errorf("source T%d: %w", index+1, err)
		}
		parts := decomposeRYB(color, palette)
		if len(parts) == 0 {
			return materialPlan{}, fmt.Errorf("source T%d color %s cannot be decomposed", index+1, value)
		}
		if len(parts) == 1 {
			mapping[index+1] = palette.slot(parts[0].role)
			continue
		}

		components := make([]int, len(parts))
		floatWeights := make([]float64, len(parts))
		for partIndex, part := range parts {
			components[partIndex] = palette.slot(part.role)
			floatWeights[partIndex] = part.weight
		}
		weights := normalizedPercentages(floatWeights)
		signature := mixSignature(components, weights)
		if target, ok := mixedTargets[signature]; ok {
			mapping[index+1] = target
			continue
		}

		stableID := len(definitions) + 1
		definition, err := makeDefinition(components, weights, false, "", stableID)
		if err != nil {
			return materialPlan{}, fmt.Errorf("source T%d color %s: %w", index+1, value, err)
		}
		if len(components) == 3 {
			hasThreeColor = true
		}
		definitions = append(definitions, definition)
		target := len(palette.Slots) + stableID
		mixedTargets[signature] = target
		mapping[index+1] = target
	}
	for index, value := range colors {
		if mapping[index+1] > 0 {
			continue
		}
		color, err := parseColor(value)
		if err != nil {
			return materialPlan{}, fmt.Errorf("source T%d: %w", index+1, err)
		}
		bestSlot := 1
		bestDistance := math.MaxInt
		for slot, role := range palette.Slots {
			distance := colorDistance(color, roleRGB(role))
			if distance < bestDistance {
				bestDistance = distance
				bestSlot = slot + 1
			}
		}
		mapping[index+1] = bestSlot
	}

	return materialPlan{
		mode:            "full-spectrum",
		physicalMapping: cloneMapping(mapping),
		allMapping:      mapping,
		definitions:     strings.Join(definitions, ";"),
		virtualMixes:    len(definitions),
		hasThreeColor:   hasThreeColor,
		forceLocalZ:     true,
		palette:         palette,
	}, nil
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
		return nil, &FullSpectrumRequiredError{ColorCount: len(groups)}
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
	_, bestCost, err := assignMaterialGroups(groups, templateTypes, palette)
	if err != nil {
		return Palette{}, err
	}
	best := palette
	bestReplacementRank := math.MaxInt
	replaced := false
	omitted := ColorRole("")
	for _, role := range colorRoles {
		if palette.slot(role) == 0 {
			omitted = role
			break
		}
	}
	for index, role := range palette.Slots {
		candidate := palette
		candidate.Slots[index] = omitted
		_, cost, candidateErr := assignMaterialGroups(groups, templateTypes, candidate)
		if candidateErr != nil {
			continue
		}
		rank := replacementRank(role)
		if cost < bestCost || replaced && cost == bestCost && rank < bestReplacementRank {
			best = candidate
			bestCost = cost
			bestReplacementRank = rank
			replaced = true
		}
	}
	return best, nil
}

func selectFullSpectrumPalette(source map[string]any, palette Palette, usage materialUsage) Palette {
	neutral := preferredNeutral(source, palette, usage)
	required := map[ColorRole]bool{ColorRed: true, ColorYellow: true, ColorBlue: true, neutral: true}
	missing := make([]ColorRole, 0, 1)
	for role := range required {
		if palette.slot(role) == 0 {
			missing = append(missing, role)
		}
	}
	result := palette
	for _, role := range missing {
		for index, current := range result.Slots {
			if !required[current] {
				result.Slots[index] = role
				break
			}
		}
	}
	return result
}

func preferredNeutral(source map[string]any, palette Palette, usage materialUsage) ColorRole {
	colors := stringSlice(source["filament_colour"])
	weights := map[ColorRole]float64{ColorWhite: 0, ColorBlack: 0}
	for material, weight := range usage {
		if material <= 0 || material > len(colors) {
			continue
		}
		color, err := parseColor(colors[material-1])
		if err != nil {
			continue
		}
		if role, ok := neutralColorRole(color); ok {
			weights[role] += weight
		}
	}
	if weights[ColorWhite] > weights[ColorBlack] {
		return ColorWhite
	}
	if weights[ColorBlack] > weights[ColorWhite] {
		return ColorBlack
	}
	if palette.slot(ColorWhite) > 0 {
		return ColorWhite
	}
	return ColorBlack
}

func replacementRank(role ColorRole) int {
	switch role {
	case ColorRed:
		return 0
	case ColorBlue:
		return 1
	case ColorYellow:
		return 2
	case ColorWhite, ColorBlack:
		return 3
	default:
		return 4
	}
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
