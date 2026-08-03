package threemf

import "testing"

func TestRemapPaintEncoding(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		mapping map[int]int
		want    string
	}{
		{name: "extended virtual material", input: "2C", mapping: map[int]int{5: 7}, want: "4C"},
		{name: "simple to extended", input: "4", mapping: map[int]int{1: 4}, want: "1C"},
		{name: "large identity", input: "5FC", mapping: map[int]int{23: 23}, want: "5FC"},
		{name: "split tree", input: "2C41", mapping: map[int]int{1: 4, 5: 7}, want: "4C1C1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := remapPaintEncoding(test.input, test.mapping)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("remapPaintEncoding(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
