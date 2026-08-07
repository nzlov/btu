package threemf

import (
	"reflect"
	"strconv"
	"strings"
)

var materialSlotSettingNames = map[string]struct{}{
	"activate_air_filtration":                  {},
	"additional_cooling_fan_speed":             {},
	"additional_fan_full_speed_layer":          {},
	"chamber_temperature":                      {},
	"chamber_temperatures":                     {},
	"circle_compensation_speed":                {},
	"close_additional_fan_first_x_layers":      {},
	"close_fan_the_first_x_layers":             {},
	"complete_print_exhaust_fan_speed":         {},
	"cool_plate_temp":                          {},
	"cool_plate_temp_initial_layer":            {},
	"cooling_perimeter_transition_distance":    {},
	"cooling_slowdown_logic":                   {},
	"counter_coef_1":                           {},
	"counter_coef_2":                           {},
	"counter_coef_3":                           {},
	"counter_limit_max":                        {},
	"counter_limit_min":                        {},
	"diameter_limit":                           {},
	"during_print_exhaust_fan_speed":           {},
	"enable_overhang_bridge_fan":               {},
	"enable_pressure_advance":                  {},
	"eng_plate_temp":                           {},
	"eng_plate_temp_initial_layer":             {},
	"fan_cooling_layer_time":                   {},
	"fan_max_speed":                            {},
	"fan_min_speed":                            {},
	"first_x_layer_fan_speed":                  {},
	"first_x_layer_part_fan_speed":             {},
	"flush_volumes_matrix":                     {},
	"flush_volumes_vector":                     {},
	"full_fan_speed_layer":                     {},
	"graphic_effect_plate_temp":                {},
	"graphic_effect_plate_temp_initial_layer":  {},
	"hole_coef_1":                              {},
	"hole_coef_2":                              {},
	"hole_coef_3":                              {},
	"hole_limit_max":                           {},
	"hole_limit_min":                           {},
	"hot_plate_temp":                           {},
	"hot_plate_temp_initial_layer":             {},
	"idle_temperature":                         {},
	"impact_strength_z":                        {},
	"internal_bridge_fan_speed":                {},
	"ironing_fan_speed":                        {},
	"no_slow_down_for_cooling_on_outwalls":     {},
	"nozzle_temperature":                       {},
	"nozzle_temperature_initial_layer":         {},
	"nozzle_temperature_range_high":            {},
	"nozzle_temperature_range_low":             {},
	"overhang_fan_speed":                       {},
	"overhang_fan_threshold":                   {},
	"overhang_threshold_participating_cooling": {},
	"pre_start_fan_time":                       {},
	"pressure_advance":                         {},
	"reduce_fan_stop_start_freq":               {},
	"required_nozzle_HRC":                      {},
	"slow_down_for_layer_cooling":              {},
	"slow_down_layer_time":                     {},
	"slow_down_min_speed":                      {},
	"supertack_plate_temp":                     {},
	"supertack_plate_temp_initial_layer":       {},
	"support_material_interface_fan_speed":     {},
	"temperature_vitrification":                {},
	"textured_cool_plate_temp":                 {},
	"textured_cool_plate_temp_initial_layer":   {},
	"textured_plate_temp":                      {},
	"textured_plate_temp_initial_layer":        {},
}

var u1CoupledMaterialSettingNames = map[string]struct{}{
	"enable_pressure_advance":       {},
	"filament_end_gcode":            {},
	"filament_max_volumetric_speed": {},
	"filament_start_gcode":          {},
	"pressure_advance":              {},
}

var mappedMaterialSettingNames = map[string]struct{}{
	"filament_colour":           {},
	"filament_colour_mode":      {},
	"filament_extruder_variant": {},
	"filament_map":              {},
	"filament_multi_colors":     {},
	"filament_nozzle_map":       {},
	"filament_self_index":       {},
	"filament_volume_map":       {},
	"flush_volumes_matrix":      {},
	"flush_volumes_vector":      {},
}

var u1CoupledMaterialSettingPrefixes = [...]string{
	"filament_deretraction_",
	"filament_loading_",
	"filament_long_retractions_",
	"filament_multitool_",
	"filament_ramming_",
	"filament_retract",
	"filament_stamping_",
	"filament_toolchange_",
	"filament_unloading_",
	"filament_wipe",
	"filament_z_hop",
}

func mergeMaterialSettings(target, source map[string]any, plan materialPlan) {
	if plan.preserveSlots {
		mergePreservedMaterialSettings(target, source, plan)
		return
	}

	sourceSlotCount := len(stringSlice(source["filament_colour"]))
	targetSlotCount := len(plan.palette.Slots)
	for key := range source {
		if !isMappedSourceMaterialSetting(key) {
			continue
		}
		targetValue, supported := target[key]
		if !supported {
			continue
		}
		sourceValue, ok := sourceMaterialSettingValue(source, key, sourceSlotCount)
		if !ok {
			continue
		}
		mapped, ok := remapMaterialSettingValue(sourceValue, targetValue, sourceSlotCount, targetSlotCount, plan.sourceSlots)
		if ok {
			target[key] = mapped
		}
	}
}

func sourceMaterialSettingValue(source map[string]any, key string, slotCount int) (any, bool) {
	value := source[key]
	values, isSlice := anySlice(value)
	if !isSlice || len(values) == slotCount {
		return value, true
	}

	indexes := stringSlice(source["filament_self_index"])
	if len(indexes) != len(values) {
		return nil, false
	}
	variants := stringSlice(source["filament_extruder_variant"])
	if len(variants) != len(values) {
		return nil, false
	}
	selected := make([]any, slotCount)
	found := make([]bool, slotCount)
	for index, rawSlot := range indexes {
		slot, err := strconv.Atoi(rawSlot)
		if err != nil || slot < 1 || slot > slotCount {
			continue
		}
		if found[slot-1] || variants[index] != sourceFilamentVariant(source, slot-1) {
			continue
		}
		selected[slot-1] = values[index]
		found[slot-1] = true
	}
	for _, exists := range found {
		if !exists {
			return nil, false
		}
	}
	return selected, true
}

func sourceFilamentVariant(source map[string]any, slot int) string {
	volumeTypes := map[int]string{
		0: "Standard",
		1: "High Flow",
		2: "Hybrid",
		3: "TPU High Flow",
		5: "E3D High Flow",
	}
	volumeType := intAt(source["filament_volume_map"], slot, 0)
	extruder := intAt(source["filament_map"], slot, 1) - 1
	extruderTypes := stringSlice(source["extruder_type"])
	if extruder < 0 || extruder >= len(extruderTypes) {
		return "Direct Drive " + volumeTypes[volumeType]
	}
	return extruderTypes[extruder] + " " + volumeTypes[volumeType]
}

func intAt(value any, index, fallback int) int {
	values := stringSlice(value)
	if index < 0 || index >= len(values) {
		return fallback
	}
	parsed, err := strconv.Atoi(values[index])
	if err != nil {
		return fallback
	}
	return parsed
}

func mergePreservedMaterialSettings(target, source map[string]any, plan materialPlan) {
	sourceSlotCount := len(stringSlice(source["filament_colour"]))
	targetSlotCount := len(plan.slotColors)
	baseSlotCount := len(plan.palette.Slots)
	for key, targetValue := range target {
		if !isMaterialSlotSetting(key) {
			continue
		}
		if sourceValue, exists := source[key]; exists && isSourceMaterialSetting(key) {
			if key != "flush_volumes_matrix" && key != "flush_volumes_vector" {
				var supported bool
				sourceValue, supported = sourceMaterialSettingValue(source, key, sourceSlotCount)
				if !supported {
					target[key] = resizeFourSlotValue(targetValue, targetSlotCount)
					continue
				}
			}
			if combined, ok := combinePreservedMaterialSetting(key, targetValue, sourceValue, baseSlotCount, sourceSlotCount, plan.preservedSources); ok {
				target[key] = combined
				continue
			}
		}
		target[key] = resizeFourSlotValue(targetValue, targetSlotCount)
	}

	target["filament_is_mixed"] = repeatedSlotValue("0", targetSlotCount)
	for _, key := range []string{
		"filament_mixed_components",
		"filament_mixed_gradient_range",
		"filament_mixed_sublayer_ratios",
	} {
		target[key] = repeatedSlotValue("", targetSlotCount)
	}
	target["filament_mixed_gradient"] = repeatedSlotValue("0", targetSlotCount)
}

func combinePreservedMaterialSetting(key string, targetValue, sourceValue any, baseSlotCount, sourceSlotCount int, preservedSources []int) (any, bool) {
	targetValues, targetIsSlice := anySlice(targetValue)
	sourceValues, sourceIsSlice := anySlice(sourceValue)
	if !sourceIsSlice {
		return sourceValue, true
	}
	if !targetIsSlice {
		return nil, false
	}
	if key == "flush_volumes_matrix" {
		return combineFlushVolumeMatrices(targetValues, sourceValues, baseSlotCount, sourceSlotCount, preservedSources)
	}
	selected, ok := selectPreservedSourceValues(key, sourceValues, sourceSlotCount, preservedSources)
	if !ok {
		return nil, false
	}
	return append(append([]any(nil), targetValues...), selected...), true
}

func selectPreservedSourceValues(key string, source []any, sourceSlotCount int, preservedSources []int) ([]any, bool) {
	width := 1
	if key == "flush_volumes_vector" {
		width = 2
	}
	if len(source) != sourceSlotCount*width {
		return nil, false
	}
	selected := make([]any, 0, len(preservedSources)*width)
	for _, material := range preservedSources {
		if material < 1 || material > sourceSlotCount {
			return nil, false
		}
		start := (material - 1) * width
		selected = append(selected, source[start:start+width]...)
	}
	return selected, true
}

func combineFlushVolumeMatrices(base, source []any, baseSlotCount, sourceSlotCount int, preservedSources []int) ([]any, bool) {
	if len(base) != baseSlotCount*baseSlotCount || len(source) != sourceSlotCount*sourceSlotCount {
		return nil, false
	}
	cross := largestFlushVolume(base, source)
	total := baseSlotCount + len(preservedSources)
	result := make([]any, total*total)
	for row := 0; row < total; row++ {
		for column := 0; column < total; column++ {
			switch {
			case row < baseSlotCount && column < baseSlotCount:
				result[row*total+column] = base[row*baseSlotCount+column]
			case row >= baseSlotCount && column >= baseSlotCount:
				sourceRow := preservedSources[row-baseSlotCount] - 1
				sourceColumn := preservedSources[column-baseSlotCount] - 1
				if sourceRow < 0 || sourceRow >= sourceSlotCount || sourceColumn < 0 || sourceColumn >= sourceSlotCount {
					return nil, false
				}
				result[row*total+column] = source[sourceRow*sourceSlotCount+sourceColumn]
			default:
				result[row*total+column] = cross
			}
		}
	}
	return result, true
}

func largestFlushVolume(groups ...[]any) any {
	best := any("0")
	bestValue := 0.0
	for _, values := range groups {
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				continue
			}
			parsed, err := strconv.ParseFloat(text, 64)
			if err == nil && parsed > bestValue {
				best = value
				bestValue = parsed
			}
		}
	}
	return best
}

func remapMaterialSettingValue(sourceValue, targetValue any, sourceSlotCount, targetSlotCount int, sourceSlots map[int][]int) (any, bool) {
	sourceValues, sourceIsSlice := anySlice(sourceValue)
	if !sourceIsSlice {
		return sourceValue, true
	}
	if len(sourceValues) != sourceSlotCount || sourceSlotCount == 0 || targetSlotCount == 0 {
		return nil, false
	}

	targetValues, targetIsSlice := anySlice(targetValue)
	if targetIsSlice && len(targetValues) != targetSlotCount {
		return nil, false
	}
	result := make([]any, targetSlotCount)
	assigned := make([]bool, targetSlotCount)
	conflict := make([]bool, targetSlotCount)
	// Several source materials can share a U1 component. Keep its baseline value
	// when their material profiles disagree instead of choosing one arbitrarily.
	for sourceSlot, targetSlots := range sourceSlots {
		if sourceSlot < 1 || sourceSlot > len(sourceValues) {
			continue
		}
		for _, targetSlot := range targetSlots {
			if targetSlot < 1 || targetSlot > targetSlotCount {
				continue
			}
			index := targetSlot - 1
			if assigned[index] && !reflect.DeepEqual(result[index], sourceValues[sourceSlot-1]) {
				conflict[index] = true
				continue
			}
			result[index] = sourceValues[sourceSlot-1]
			assigned[index] = true
		}
	}

	for index := range result {
		if assigned[index] && !conflict[index] {
			continue
		}
		if !targetIsSlice {
			return nil, false
		}
		result[index] = targetValues[index]
	}
	return result, true
}

func anySlice(value any) ([]any, bool) {
	switch values := value.(type) {
	case []any:
		return values, true
	case []string:
		result := make([]any, len(values))
		for index, value := range values {
			result[index] = value
		}
		return result, true
	default:
		return nil, false
	}
}

func isMaterialSlotSetting(key string) bool {
	if strings.HasPrefix(key, "filament_mixed") || key == "filament_is_mixed" {
		return false
	}
	if strings.HasPrefix(key, "filament_") {
		return true
	}
	_, exists := materialSlotSettingNames[key]
	return exists
}

func isSourceMaterialSetting(key string) bool {
	if !isMaterialSlotSetting(key) {
		return false
	}
	if _, coupled := u1CoupledMaterialSettingNames[key]; coupled {
		return false
	}
	for _, prefix := range u1CoupledMaterialSettingPrefixes {
		if strings.HasPrefix(key, prefix) {
			return false
		}
	}
	return true
}

func isMappedSourceMaterialSetting(key string) bool {
	if !isSourceMaterialSetting(key) {
		return false
	}
	_, derived := mappedMaterialSettingNames[key]
	return !derived
}

func resizeFourSlotValue(value any, slotCount int) any {
	switch values := value.(type) {
	case []any:
		if len(values) != 4 {
			return value
		}
		resized := make([]any, slotCount)
		for index := range resized {
			resized[index] = values[min(index, len(values)-1)]
		}
		return resized
	case []string:
		if len(values) != 4 {
			return value
		}
		resized := make([]string, slotCount)
		for index := range resized {
			resized[index] = values[min(index, len(values)-1)]
		}
		return resized
	default:
		return value
	}
}

func repeatedSlotValue(value string, count int) []string {
	result := make([]string, count)
	for index := range result {
		result[index] = value
	}
	return result
}
