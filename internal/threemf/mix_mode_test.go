package threemf

import "testing"

func TestParseMixMode(t *testing.T) {
	for _, value := range []string{"ratio", "cycle", "match", "gradient"} {
		mode, err := ParseMixMode(value)
		if err != nil {
			t.Fatalf("ParseMixMode(%q): %v", value, err)
		}
		if mode.String() != value {
			t.Fatalf("ParseMixMode(%q) = %q", value, mode)
		}
	}
	if _, err := ParseMixMode("blend"); err == nil {
		t.Fatal("ParseMixMode accepted an unknown mode")
	}
}

func TestMakeDefinitionsForAllMixModes(t *testing.T) {
	tests := []struct {
		name       string
		components []int
		weights    []int
		mode       MixMode
		want       string
	}{
		{
			name: "ratio", components: []int{1, 2}, weights: []int{50, 50}, mode: MixModeRatio,
			want: "1,2,1,1,50,0,g,w,m2,z0,xa0,xb0,d0,o0,u1,cm0",
		},
		{
			name: "cycle", components: []int{1, 2, 3}, weights: []int{33, 33, 34}, mode: MixModeCycle,
			want: "1,2,1,1,33,0,g,w,m2,z0,xa0,xb0,d0,o0,u1,cm1,123",
		},
		{
			name: "match", components: []int{1, 3, 2}, weights: []int{51, 25, 24}, mode: MixModeMatch,
			want: "1,3,1,1,50,0,g132,w51/25/24,m0,z0,xa0,xb0,d0,o0,u1,cm2",
		},
		{
			name: "gradient", components: []int{2, 4}, weights: []int{50, 50}, mode: MixModeGradient,
			want: "2,4,1,1,50,0,g,w,m0,z2,xa0,xb0,d0,o0,u1,cm3,r1/0.8000/0.2000",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := makeDefinition(test.components, test.weights, test.mode, "", 1)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("definition = %q, want %q", got, test.want)
			}
		})
	}
}
