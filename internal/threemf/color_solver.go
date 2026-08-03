package threemf

import (
	"fmt"
	"math"
	"sync"

	"github.com/nzlov/btu/internal/colormix"
)

type colorRecipe struct {
	components []int
	weights    []int
	preview    [3]int
	error      float64
}

type paletteScore struct {
	maxError   float64
	totalError float64
	usedError  float64
	mixCost    float64
}

type cachedColorRecipe struct {
	recipe colorRecipe
	ok     bool
}

var colorRecipeCache sync.Map

func selectColorRecipes(colors [][3]int, preferred Palette, usage materialUsage) (Palette, []colorRecipe, error) {
	bestScore := paletteScore{
		maxError:   math.Inf(1),
		totalError: math.Inf(1),
		usedError:  math.Inf(1),
		mixCost:    math.Inf(1),
	}
	var bestPalette Palette
	var bestRecipes []colorRecipe
	for _, candidate := range paletteCandidates(preferred) {
		recipes := make([]colorRecipe, len(colors))
		score := paletteScore{}
		valid := true
		for index, color := range colors {
			recipe, ok := bestColorRecipe(color, candidate)
			if !ok {
				valid = false
				break
			}
			recipes[index] = recipe
			score.maxError = max(score.maxError, recipe.error)
			score.totalError += recipe.error
			if weight := usage[index+1]; weight > 0 {
				score.usedError += recipe.error * (1 + math.Log1p(weight))
				score.mixCost += float64(len(recipe.components)-1) * (1 + math.Log1p(weight))
			} else {
				score.mixCost += float64(len(recipe.components) - 1)
			}
		}
		if valid && betterPaletteScore(score, bestScore) {
			bestPalette = candidate
			bestRecipes = recipes
			bestScore = score
		}
	}
	if bestRecipes == nil {
		return Palette{}, nil, fmt.Errorf("mapped colors cannot be represented by any four-color palette from cmygwb")
	}
	return bestPalette, bestRecipes, nil
}

func betterPaletteScore(candidate, current paletteScore) bool {
	const tolerance = 1e-9
	for _, values := range [][2]float64{
		{candidate.maxError, current.maxError},
		{candidate.totalError, current.totalError},
		{candidate.usedError, current.usedError},
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

func bestColorRecipe(target [3]int, palette Palette) (colorRecipe, bool) {
	key := palette.String() + "/" + canonicalColor(target)
	if cached, ok := colorRecipeCache.Load(key); ok {
		result := cached.(cachedColorRecipe)
		return result.recipe, result.ok
	}
	recipe, ok := calculateBestColorRecipe(target, palette)
	colorRecipeCache.Store(key, cachedColorRecipe{recipe: recipe, ok: ok})
	return recipe, ok
}

func calculateBestColorRecipe(target [3]int, palette Palette) (colorRecipe, bool) {
	if role, base := baseColorRole(target); base {
		if slot := palette.slot(role); slot > 0 {
			return directColorRecipe(target, role, slot), true
		}
		switch role {
		case ColorBlack:
			return searchRequiredRecipe(target, palette, []ColorRole{ColorCyan, ColorMagenta, ColorYellow})
		case ColorGray:
			return searchRequiredRecipe(target, palette, []ColorRole{ColorWhite, ColorBlack})
		default:
			return colorRecipe{}, false
		}
	}

	slots := []int{1, 2, 3, 4}
	best := colorRecipe{error: math.Inf(1)}
	for componentCount := 2; componentCount <= len(slots); componentCount++ {
		for _, components := range componentCombinations(slots, componentCount) {
			recipe := searchRecipeWeights(target, palette, components)
			if recipe.error < best.error {
				best = recipe
			}
		}
	}
	return best, !math.IsInf(best.error, 1)
}

func directColorRecipe(target [3]int, role ColorRole, slot int) colorRecipe {
	preview := roleRGB(role)
	return colorRecipe{
		components: []int{slot},
		weights:    []int{100},
		preview:    preview,
		error:      recipeDistance(target, preview),
	}
}

func searchRequiredRecipe(target [3]int, palette Palette, roles []ColorRole) (colorRecipe, bool) {
	components := make([]int, len(roles))
	for index, role := range roles {
		components[index] = palette.slot(role)
		if components[index] == 0 {
			return colorRecipe{}, false
		}
	}
	sortInts(components)
	recipe := searchRecipeWeights(target, palette, components)
	return recipe, !math.IsInf(recipe.error, 1)
}

func searchRecipeWeights(target [3]int, palette Palette, components []int) colorRecipe {
	best := colorRecipe{error: math.Inf(1)}
	switch len(components) {
	case 2:
		for first := 1; first < 100; first++ {
			best = chooseRecipe(best, evaluateRecipe(target, palette, components, []int{first, 100 - first}))
		}
		return best
	case 3:
		for first := 1; first <= 98; first++ {
			for second := 1; first+second < 100; second++ {
				weights := []int{first, second, 100 - first - second}
				best = chooseRecipe(best, evaluateRecipe(target, palette, components, weights))
			}
		}
		return best
	}

	coarse := colorRecipe{error: math.Inf(1)}
	for first := 5; first <= 85; first += 5 {
		for second := 5; first+second <= 90; second += 5 {
			for third := 5; first+second+third < 100; third += 5 {
				weights := []int{first, second, third, 100 - first - second - third}
				coarse = chooseRecipe(coarse, evaluateRecipe(target, palette, components, weights))
			}
		}
	}
	best = coarse
	for first := max(1, coarse.weights[0]-5); first <= min(97, coarse.weights[0]+5); first++ {
		for second := max(1, coarse.weights[1]-5); second <= min(97, coarse.weights[1]+5); second++ {
			for third := max(1, coarse.weights[2]-5); third <= min(97, coarse.weights[2]+5); third++ {
				if first+second+third >= 100 {
					continue
				}
				weights := []int{first, second, third, 100 - first - second - third}
				best = chooseRecipe(best, evaluateRecipe(target, palette, components, weights))
			}
		}
	}
	return best
}

func evaluateRecipe(target [3]int, palette Palette, components, weights []int) colorRecipe {
	colors := make([]colormix.RGB, len(components))
	for index, slot := range components {
		color := roleRGB(palette.Slots[slot-1])
		colors[index] = colormix.RGB{R: uint8(color[0]), G: uint8(color[1]), B: uint8(color[2])}
	}
	mixed := colormix.BlendWeighted(colors, weights)
	preview := [3]int{int(mixed.R), int(mixed.G), int(mixed.B)}
	return colorRecipe{
		components: append([]int(nil), components...),
		weights:    append([]int(nil), weights...),
		preview:    preview,
		error:      recipeDistance(target, preview),
	}
}

func recipeDistance(first, second [3]int) float64 {
	return colormix.DistanceSquared(
		colormix.RGB{R: uint8(first[0]), G: uint8(first[1]), B: uint8(first[2])},
		colormix.RGB{R: uint8(second[0]), G: uint8(second[1]), B: uint8(second[2])},
	)
}

func chooseRecipe(current, candidate colorRecipe) colorRecipe {
	if candidate.error < current.error {
		return candidate
	}
	return current
}

func componentCombinations(slots []int, count int) [][]int {
	var result [][]int
	current := make([]int, 0, count)
	var visit func(int)
	visit = func(start int) {
		if len(current) == count {
			result = append(result, append([]int(nil), current...))
			return
		}
		for index := start; index <= len(slots)-(count-len(current)); index++ {
			current = append(current, slots[index])
			visit(index + 1)
			current = current[:len(current)-1]
		}
	}
	visit(0)
	return result
}

func sortInts(values []int) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && values[cursor] < values[cursor-1]; cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}
