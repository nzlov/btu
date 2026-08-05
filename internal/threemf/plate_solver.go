package threemf

import (
	"fmt"
	"math"
)

type platePlan struct {
	id      int
	name    string
	neutral ColorRole
	usage   materialUsage
}

type plateScore struct {
	maxError   float64
	totalError float64
	mixCost    float64
}

func normalizeProjectMaterialUsage(usage projectMaterialUsage, materialCount int) projectMaterialUsage {
	result := projectMaterialUsage{Total: normalizedPhysicalUsage(usage.Total, materialCount)}
	for _, plate := range usage.Plates {
		materials := make(materialUsage)
		for material, weight := range plate.Materials {
			if material > 0 && material <= materialCount && weight > 0 {
				materials.add(material, weight)
			}
		}
		if len(materials) == 0 {
			continue
		}
		result.Plates = append(result.Plates, plateMaterialUsage{
			ID: plate.ID, Name: plate.Name, Materials: materials,
		})
	}
	if len(result.Plates) == 0 {
		result.Plates = []plateMaterialUsage{{ID: 1, Materials: cloneUsage(result.Total)}}
	}
	return result
}

func cloneUsage(source materialUsage) materialUsage {
	result := make(materialUsage, len(source))
	for material, weight := range source {
		result[material] = weight
	}
	return result
}

func selectPlatePlans(colors [][3]int, usage projectMaterialUsage, preferred Palette, modes []MixMode) ([]platePlan, error) {
	plans := make([]platePlan, 0, len(usage.Plates))
	for _, plate := range usage.Plates {
		bestScore := plateScore{maxError: math.Inf(1), totalError: math.Inf(1), mixCost: math.Inf(1)}
		bestNeutral := ColorRole("")
		found := false
		for _, candidate := range paletteCandidates(preferred) {
			score, err := scorePlatePalette(colors, plate.Materials, candidate, modes)
			if err != nil {
				return nil, fmt.Errorf("plate %d: %w", plate.ID, err)
			}
			if betterPlateScore(score, bestScore) {
				bestScore = score
				bestNeutral = candidate.Neutral()
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("plate %d has no valid four-color palette", plate.ID)
		}
		plans = append(plans, platePlan{
			id: plate.ID, name: plate.Name, neutral: bestNeutral, usage: plate.Materials,
		})
	}
	return plans, nil
}

func selectProjectPalette(colors [][3]int, usage projectMaterialUsage, preferred Palette, modes []MixMode) (Palette, error) {
	best := Palette{}
	bestScore := plateScore{maxError: math.Inf(1), totalError: math.Inf(1), mixCost: math.Inf(1)}
	bestChanges := math.MaxInt
	for _, candidate := range projectPaletteCandidates(preferred) {
		plans, err := selectPlatePlans(colors, usage, candidate, modes)
		if err != nil {
			continue
		}
		recipes, err := selectProjectRecipes(colors, usage, plans, candidate, modes)
		if err != nil {
			continue
		}
		score := scoreProjectRecipes(recipes, usage.Total)
		changes := paletteChanges(preferred, candidate)
		if betterPlateScore(score, bestScore) || (equalPlateScore(score, bestScore) && changes < bestChanges) {
			best = candidate
			bestScore = score
			bestChanges = changes
		}
	}
	if best == (Palette{}) {
		return Palette{}, fmt.Errorf("source colors have no valid four-color palette")
	}
	return best, nil
}

func scoreProjectRecipes(recipes []colorRecipe, usage materialUsage) plateScore {
	score := plateScore{}
	for material, weight := range usage {
		if material <= 0 || material > len(recipes) || weight <= 0 {
			continue
		}
		recipe := recipes[material-1]
		weighted := 1 + math.Log1p(weight)
		score.maxError = max(score.maxError, recipe.error)
		score.totalError += recipe.error * weighted
		score.mixCost += float64(len(recipe.components)-1) * weighted
	}
	return score
}

func paletteChanges(first, second Palette) int {
	changes := 0
	for index := range first.Slots {
		if first.Slots[index] != second.Slots[index] {
			changes++
		}
	}
	return changes
}

func scorePlatePalette(colors [][3]int, usage materialUsage, palette Palette, modes []MixMode) (plateScore, error) {
	score := plateScore{}
	for material, weight := range usage {
		if material <= 0 || material > len(colors) || weight <= 0 {
			continue
		}
		recipe, ok := bestColorRecipeForMode(colors[material-1], palette, mixModeAt(modes, material))
		if !ok {
			return plateScore{}, fmt.Errorf("source T%d cannot be represented by %s", material, palette.String())
		}
		weighted := 1 + math.Log1p(weight)
		score.maxError = max(score.maxError, recipe.error)
		score.totalError += recipe.error * weighted
		score.mixCost += float64(len(recipe.components)-1) * weighted
	}
	return score, nil
}

func betterPlateScore(candidate, current plateScore) bool {
	const tolerance = 1e-9
	for _, values := range [][2]float64{
		{candidate.maxError, current.maxError},
		{candidate.totalError, current.totalError},
		{candidate.mixCost, current.mixCost},
	} {
		if values[0] < values[1]-tolerance {
			return true
		}
		if values[0] > values[1]+tolerance {
			return false
		}
	}
	return false
}

func equalPlateScore(first, second plateScore) bool {
	const tolerance = 1e-9
	return math.Abs(first.maxError-second.maxError) <= tolerance &&
		math.Abs(first.totalError-second.totalError) <= tolerance &&
		math.Abs(first.mixCost-second.mixCost) <= tolerance
}

func selectProjectRecipes(colors [][3]int, usage projectMaterialUsage, plans []platePlan, preferred Palette, modes []MixMode) ([]colorRecipe, error) {
	if !preferred.supportsDynamicNeutral() {
		recipes := make([]colorRecipe, len(colors))
		for index, color := range colors {
			recipe, ok := bestColorRecipeForMode(color, preferred, mixModeAt(modes, index+1))
			if !ok {
				return nil, fmt.Errorf("source T%d target %s has no compatible recipe", index+1, canonicalColor(color))
			}
			recipes[index] = recipe
		}
		return recipes, nil
	}
	materialNeutrals := make(map[int]map[ColorRole]bool)
	for _, plan := range plans {
		for material, weight := range plan.usage {
			if weight <= 0 {
				continue
			}
			if materialNeutrals[material] == nil {
				materialNeutrals[material] = make(map[ColorRole]bool)
			}
			materialNeutrals[material][plan.neutral] = true
		}
	}

	recipes := make([]colorRecipe, len(colors))
	for index, color := range colors {
		mode := mixModeAt(modes, index+1)
		neutrals := materialNeutrals[index+1]
		var recipe colorRecipe
		var ok bool
		switch len(neutrals) {
		case 0:
			recipe, ok = bestColorRecipeForMode(color, preferred, mode)
		case 1:
			for neutral := range neutrals {
				recipe, ok = bestColorRecipeForMode(color, preferred.withNeutral(neutral), mode)
			}
		default:
			recipe, ok = bestCMYRecipeForMode(color, preferred, mode)
		}
		if !ok {
			return nil, fmt.Errorf("source T%d target %s has no compatible recipe", index+1, canonicalColor(color))
		}
		recipes[index] = recipe
	}
	return recipes, nil
}

func mixModeAt(modes []MixMode, material int) MixMode {
	if material > 0 && material <= len(modes) && modes[material-1] != "" {
		return modes[material-1]
	}
	return MixModeRatio
}
