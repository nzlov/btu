package threemf

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type MixMode string

const (
	MixModeRatio    MixMode = "ratio"
	MixModeCycle    MixMode = "cycle"
	MixModeMatch    MixMode = "match"
	MixModeGradient MixMode = "gradient"
)

func ParseMixMode(value string) (MixMode, error) {
	mode := MixMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case MixModeRatio, MixModeCycle, MixModeMatch, MixModeGradient:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported mix mode %q (allowed: ratio, cycle, match, gradient)", value)
	}
}

func (mode MixMode) String() string {
	return string(mode)
}

func normalizeMixMode(mode MixMode) (MixMode, error) {
	if mode == "" {
		return MixModeRatio, nil
	}
	return ParseMixMode(string(mode))
}

func makeCyclePattern(components, weights []int) (string, int, error) {
	if len(components) < 2 || len(components) > 4 || len(weights) != len(components) {
		return "", 0, fmt.Errorf("cycle mixing requires two to four weighted components")
	}
	total := 0
	for index := range components {
		if components[index] < 1 || components[index] > 9 || weights[index] <= 0 {
			return "", 0, fmt.Errorf("cycle components and weights must be positive single-digit values")
		}
		total += weights[index]
	}

	counts := append([]int(nil), weights...)
	common := counts[0]
	for _, count := range counts[1:] {
		common = greatestCommonDivisor(common, count)
	}
	for index := range counts {
		counts[index] /= common
	}
	cycleLength := 0
	for _, count := range counts {
		cycleLength += count
	}
	if cycleLength > 48 {
		counts = scaleCycleCounts(weights, total, 48)
		common = counts[0]
		for _, count := range counts[1:] {
			common = greatestCommonDivisor(common, count)
		}
		for index := range counts {
			counts[index] /= common
		}
	}

	cycleLength = 0
	for _, count := range counts {
		cycleLength += count
	}
	emitted := make([]int, len(counts))
	var pattern strings.Builder
	for position := 0; position < cycleLength; position++ {
		best := 0
		bestScore := math.Inf(-1)
		for index, count := range counts {
			score := float64((position+1)*count)/float64(cycleLength) - float64(emitted[index])
			if score > bestScore {
				best = index
				bestScore = score
			}
		}
		pattern.WriteString(strconv.Itoa(components[best]))
		emitted[best]++
	}

	componentBCount := 0
	for index, component := range components {
		if component == 2 {
			componentBCount += counts[index]
		}
	}
	mixB := int(math.Round(float64(componentBCount) * 100 / float64(cycleLength)))
	return pattern.String(), mixB, nil
}

func scaleCycleCounts(weights []int, total, target int) []int {
	counts := make([]int, len(weights))
	exact := make([]float64, len(weights))
	allocated := 0
	for index, weight := range weights {
		exact[index] = float64(weight) * float64(target) / float64(total)
		counts[index] = max(1, int(math.Round(exact[index])))
		allocated += counts[index]
	}
	for allocated < target {
		best := 0
		for index := 1; index < len(counts); index++ {
			if exact[index]-float64(counts[index]) > exact[best]-float64(counts[best]) {
				best = index
			}
		}
		counts[best]++
		allocated++
	}
	for allocated > target {
		best := -1
		for index := range counts {
			if counts[index] <= 1 {
				continue
			}
			if best < 0 || float64(counts[index])-exact[index] > float64(counts[best])-exact[best] {
				best = index
			}
		}
		if best < 0 {
			break
		}
		counts[best]--
		allocated--
	}
	return counts
}

func greatestCommonDivisor(first, second int) int {
	for second != 0 {
		first, second = second, first%second
	}
	return max(1, first)
}
