package colormix

import "testing"

func TestBlendMatchesFullSpectrumReference(t *testing.T) {
	got := Blend(RGB{R: 0, G: 33, B: 133}, RGB{R: 252, G: 211, B: 0}, 0.5)
	want := (RGB{R: 47, G: 141, B: 56})
	if got != want {
		t.Fatalf("blend = %+v, want %+v", got, want)
	}
}

func TestBlendWeightedUsesSlotOrder(t *testing.T) {
	colors := []RGB{{B: 255}, {R: 255}, {R: 255, G: 255, B: 255}}
	weights := []int{55, 13, 32}
	want := Blend(Blend(colors[0], colors[1], 13.0/68.0), colors[2], 0.32)
	if got := BlendWeighted(colors, weights); got != want {
		t.Fatalf("blend = %+v, want %+v", got, want)
	}
}
