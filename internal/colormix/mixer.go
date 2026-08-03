package colormix

import "math"

type RGB struct {
	R uint8
	G uint8
	B uint8
}

// Blend matches the FilamentMixer polynomial model used by FullSpectrum previews.
func Blend(first, second RGB, ratio float64) RGB {
	if ratio <= 0 {
		return first
	}
	if ratio >= 1 {
		return second
	}
	input := [7]float64{
		float64(first.R), float64(first.G), float64(first.B),
		float64(second.R), float64(second.G), float64(second.B),
		ratio,
	}
	var features [330]float64
	for featureIndex, featurePowers := range powers {
		value := 1.0
		for inputIndex, exponent := range featurePowers {
			for range exponent {
				value *= input[inputIndex]
			}
		}
		features[featureIndex] = value
	}
	var channels [3]uint8
	for channel := range channels {
		value := intercept[channel]
		for featureIndex, feature := range features {
			value += feature * coefficients[featureIndex][channel]
		}
		channels[channel] = uint8(max(0, min(255, int(value))))
	}
	return RGB{R: channels[0], G: channels[1], B: channels[2]}
}

// BlendWeighted follows FullSpectrum's slot-ordered, pairwise multi-color preview.
func BlendWeighted(colors []RGB, weights []int) RGB {
	var result RGB
	accumulated := 0
	for index, color := range colors {
		if index >= len(weights) || weights[index] <= 0 {
			continue
		}
		if accumulated == 0 {
			result = color
			accumulated = weights[index]
			continue
		}
		result = Blend(result, color, float64(weights[index])/float64(accumulated+weights[index]))
		accumulated += weights[index]
	}
	return result
}

func DistanceSquared(first, second RGB) float64 {
	left := toLab(first)
	right := toLab(second)
	dl := left[0] - right[0]
	da := left[1] - right[1]
	db := left[2] - right[2]
	return dl*dl + da*da + db*db
}

func toLab(color RGB) [3]float64 {
	r := linear(float64(color.R) / 255)
	g := linear(float64(color.G) / 255)
	b := linear(float64(color.B) / 255)
	x := (0.4124564*r + 0.3575761*g + 0.1804375*b) / 0.95047
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := (0.0193339*r + 0.1191920*g + 0.9503041*b) / 1.08883
	fx := labCurve(x)
	fy := labCurve(y)
	fz := labCurve(z)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

func linear(value float64) float64 {
	if value <= 0.04045 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func labCurve(value float64) float64 {
	const delta = 6.0 / 29.0
	if value > delta*delta*delta {
		return math.Cbrt(value)
	}
	return value/(3*delta*delta) + 4.0/29.0
}
