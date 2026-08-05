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
		for _, candidate := range paletteCandidates(preferred) {
			score, err := scorePlatePalette(colors, plate.Materials, candidate, modes)
			if err != nil {
				return nil, fmt.Errorf("plate %d: %w", plate.ID, err)
			}
			if betterPlateScore(score, bestScore) {
				bestScore = score
				bestNeutral = candidate.Neutral()
			}
		}
		if bestNeutral == "" {
			return nil, fmt.Errorf("plate %d has no valid CMY plus neutral palette", plate.ID)
		}
		plans = append(plans, platePlan{
			id: plate.ID, name: plate.Name, neutral: bestNeutral, usage: plate.Materials,
		})
	}
	return plans, nil
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

func selectProjectRecipes(colors [][3]int, usage projectMaterialUsage, plans []platePlan, preferred Palette, modes []MixMode) ([]colorRecipe, error) {
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
