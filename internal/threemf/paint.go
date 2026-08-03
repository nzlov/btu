package threemf

import (
	"fmt"
	"strconv"
	"strings"
)

func remapPaintEncoding(encoded string, mapping map[int]int) (string, error) {
	output := make([]int, 0, len(encoded))
	err := processPaintEncoding(encoded, &output, func(state int) (int, error) {
		if state == 0 {
			return 0, nil
		}
		mapped, ok := mapping[state]
		if !ok {
			return 0, fmt.Errorf("paint references unknown material T%d", state)
		}
		return mapped, nil
	})
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for index := len(output) - 1; index >= 0; index-- {
		builder.WriteString(strings.ToUpper(strconv.FormatInt(int64(output[index]), 16)))
	}
	return builder.String(), nil
}

func paintStates(encoded string) ([]int, error) {
	states := make([]int, 0, 1)
	err := processPaintEncoding(encoded, nil, func(state int) (int, error) {
		states = append(states, state)
		return state, nil
	})
	return states, err
}

func processPaintEncoding(encoded string, output *[]int, transform func(int) (int, error)) error {
	nibbles := make([]int, 0, len(encoded))
	for index := len(encoded) - 1; index >= 0; index-- {
		nibble, err := strconv.ParseUint(encoded[index:index+1], 16, 4)
		if err != nil {
			return fmt.Errorf("invalid paint encoding %q", encoded)
		}
		nibbles = append(nibbles, int(nibble))
	}

	position := 0
	var visit func() error
	visit = func() error {
		if position >= len(nibbles) {
			return fmt.Errorf("truncated paint encoding %q", encoded)
		}
		code := nibbles[position]
		position++
		splitSides := code & 3
		if splitSides > 0 {
			if output != nil {
				*output = append(*output, code)
			}
			for child := 0; child < splitSides+1; child++ {
				if err := visit(); err != nil {
					return err
				}
			}
			return nil
		}

		state := code >> 2
		if code&12 == 12 {
			state = 3
			for {
				if position >= len(nibbles) {
					return fmt.Errorf("truncated extended paint state in %q", encoded)
				}
				extra := nibbles[position]
				position++
				state += extra
				if extra < 15 {
					break
				}
			}
		}
		state, err := transform(state)
		if err != nil {
			return err
		}
		if output != nil {
			appendPaintLeaf(output, state)
		}
		return nil
	}

	if err := visit(); err != nil {
		return err
	}
	if position != len(nibbles) {
		return fmt.Errorf("paint encoding %q has %d trailing nibbles", encoded, len(nibbles)-position)
	}
	return nil
}

func appendPaintLeaf(output *[]int, state int) {
	if state < 3 {
		*output = append(*output, state<<2)
		return
	}
	*output = append(*output, 12)
	remaining := state - 3
	for remaining >= 15 {
		*output = append(*output, 15)
		remaining -= 15
	}
	*output = append(*output, remaining)
}
