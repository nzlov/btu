package threemf

import "strings"

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

func mergeMaterialSlotSettings(target, source map[string]any, slotCount int) {
	for key, targetValue := range target {
		if !isMaterialSlotSetting(key) {
			continue
		}
		if sourceValue, exists := source[key]; exists {
			target[key] = sourceValue
			continue
		}
		target[key] = resizeFourSlotValue(targetValue, slotCount)
	}
	for key, value := range source {
		if isMaterialSlotSetting(key) {
			target[key] = value
		}
	}

	target["filament_is_mixed"] = repeatedSlotValue("0", slotCount)
	for _, key := range []string{
		"filament_mixed_components",
		"filament_mixed_gradient_range",
		"filament_mixed_sublayer_ratios",
	} {
		target[key] = repeatedSlotValue("", slotCount)
	}
	target["filament_mixed_gradient"] = repeatedSlotValue("0", slotCount)
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
