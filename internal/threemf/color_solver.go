package threemf

import (
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

type cachedColorRecipe struct {
	recipe colorRecipe
	ok     bool
}

var colorRecipeCache sync.Map

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

func bestColorRecipeForMode(target [3]int, palette Palette, mode MixMode) (colorRecipe, bool) {
	mode, err := normalizeMixMode(mode)
	if err != nil {
		return colorRecipe{}, false
	}
	key := palette.String() + "/" + canonicalColor(target) + "/" + mode.String()
	if cached, ok := colorRecipeCache.Load(key); ok {
		result := cached.(cachedColorRecipe)
		return result.recipe, result.ok
	}
	if role, base := baseColorRole(target); base {
		if slot := palette.slot(role); slot > 0 {
			recipe := directColorRecipe(target, role, slot)
			colorRecipeCache.Store(key, cachedColorRecipe{recipe: recipe, ok: true})
			return recipe, true
		}
	}
	var recipe colorRecipe
	var ok bool
	switch mode {
	case MixModeRatio:
		recipe, ok = searchColorRecipeWithLimit(target, palette, []int{1, 2, 3, 4}, 3)
	case MixModeGradient:
		recipe, ok = searchGradientRecipe(target, palette, []int{1, 2, 3, 4})
	default:
		recipe, ok = searchColorRecipe(target, palette, []int{1, 2, 3, 4})
	}
	colorRecipeCache.Store(key, cachedColorRecipe{recipe: recipe, ok: ok})
	return recipe, ok
}

func calculateBestColorRecipe(target [3]int, palette Palette) (colorRecipe, bool) {
	if role, base := baseColorRole(target); base {
		if slot := palette.slot(role); slot > 0 {
			return directColorRecipe(target, role, slot), true
		}
	}
	return searchColorRecipe(target, palette, []int{1, 2, 3, 4})
}

func bestCMYRecipe(target [3]int) (colorRecipe, bool) {
	palette := DefaultPalette()
	if role, base := baseColorRole(target); base {
		if slot := palette.slot(role); slot > 0 && slot <= 3 {
			return directColorRecipe(target, role, slot), true
		}
	}
	return searchColorRecipe(target, palette, []int{1, 2, 3})
}

func bestCMYRecipeForMode(target [3]int, mode MixMode) (colorRecipe, bool) {
	palette := DefaultPalette()
	if role, base := baseColorRole(target); base {
		if slot := palette.slot(role); slot > 0 && slot <= 3 {
			return directColorRecipe(target, role, slot), true
		}
	}
	if mode == MixModeGradient {
		return searchGradientRecipe(target, palette, []int{1, 2, 3})
	}
	return searchColorRecipeWithLimit(target, palette, []int{1, 2, 3}, 3)
}

func searchColorRecipe(target [3]int, palette Palette, slots []int) (colorRecipe, bool) {
	return searchColorRecipeWithLimit(target, palette, slots, len(slots))
}

func searchColorRecipeWithLimit(target [3]int, palette Palette, slots []int, maxComponents int) (colorRecipe, bool) {
	best := colorRecipe{error: math.Inf(1)}
	for componentCount := 2; componentCount <= min(len(slots), maxComponents); componentCount++ {
		for _, components := range componentCombinations(slots, componentCount) {
			recipe := searchRecipeWeights(target, palette, components)
			if recipe.error < best.error {
				best = recipe
			}
		}
	}
	return best, !math.IsInf(best.error, 1)
}

func searchGradientRecipe(target [3]int, palette Palette, slots []int) (colorRecipe, bool) {
	best := colorRecipe{error: math.Inf(1)}
	for _, components := range componentCombinations(slots, 2) {
		best = chooseRecipe(best, evaluateRecipe(target, palette, components, []int{50, 50}))
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
