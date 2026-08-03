package threemf

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"strings"
)

//go:embed assets/*/project_settings.config assets/*/slice_info.config assets/*/3dmodel.model
var baselineAssets embed.FS

var builtInNozzleSizes = [...]string{"0.2", "0.4", "0.6", "0.8"}

type u1Baseline struct {
	projectSettings map[string]any
	sliceInfo       []byte
	application     []byte
}

func ParseNozzleSize(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	normalized = strings.TrimSpace(strings.TrimSuffix(normalized, "mm"))
	switch normalized {
	case "0.2", "0.4", "0.6", "0.8":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported nozzle size %q; use 0.2, 0.4, 0.6, or 0.8", value)
	}
}

func nozzleSizeFromSettings(settings map[string]any) (string, error) {
	values := stringSlice(settings["nozzle_diameter"])
	if len(values) == 0 {
		return "", fmt.Errorf("source project has no nozzle_diameter; use --nozzle to select 0.2, 0.4, 0.6, or 0.8")
	}
	selected, err := ParseNozzleSize(values[0])
	if err != nil {
		return "", fmt.Errorf("source nozzle_diameter: %w", err)
	}
	for _, value := range values[1:] {
		candidate, err := ParseNozzleSize(value)
		if err != nil {
			return "", fmt.Errorf("source nozzle_diameter: %w", err)
		}
		if candidate != selected {
			return "", fmt.Errorf("source project uses multiple nozzle sizes; use --nozzle to select one U1 baseline")
		}
	}
	return selected, nil
}

func loadRequestedU1Baseline(templatePath, requestedNozzle string, sourceSettings map[string]any) (u1Baseline, error) {
	if templatePath != "" {
		baseline, err := loadU1BaselineFrom3MF(templatePath)
		if err != nil || requestedNozzle == "" {
			return baseline, err
		}
		return applyNozzleOverride(baseline, requestedNozzle)
	}

	nozzle := requestedNozzle
	var err error
	if nozzle == "" {
		nozzle, err = nozzleSizeFromSettings(sourceSettings)
	} else {
		nozzle, err = ParseNozzleSize(nozzle)
	}
	if err != nil {
		return u1Baseline{}, err
	}
	return loadU1Baseline(nozzle)
}

func loadU1Baseline(nozzle string) (u1Baseline, error) {
	nozzle, err := ParseNozzleSize(nozzle)
	if err != nil {
		return u1Baseline{}, err
	}
	directory := path.Join("assets", nozzle)
	projectSettings, err := baselineAssets.ReadFile(path.Join(directory, "project_settings.config"))
	if err != nil {
		return u1Baseline{}, fmt.Errorf("read built-in U1 %s mm project settings: %w", nozzle, err)
	}
	sliceInfo, err := baselineAssets.ReadFile(path.Join(directory, "slice_info.config"))
	if err != nil {
		return u1Baseline{}, fmt.Errorf("read built-in U1 %s mm slice info: %w", nozzle, err)
	}
	model, err := baselineAssets.ReadFile(path.Join(directory, "3dmodel.model"))
	if err != nil {
		return u1Baseline{}, fmt.Errorf("read built-in U1 %s mm model: %w", nozzle, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(projectSettings, &settings); err != nil {
		return u1Baseline{}, fmt.Errorf("decode built-in U1 %s mm project settings: %w", nozzle, err)
	}
	application := applicationRE.Find(model)
	if application == nil {
		return u1Baseline{}, fmt.Errorf("built-in U1 %s mm Application metadata is missing", nozzle)
	}
	return u1Baseline{
		projectSettings: settings,
		sliceInfo:       sliceInfo,
		application:     application,
	}, nil
}

func applyNozzleOverride(baseline u1Baseline, nozzle string) (u1Baseline, error) {
	nozzle, err := ParseNozzleSize(nozzle)
	if err != nil {
		return u1Baseline{}, err
	}
	variants := make([]map[string]any, 0, len(builtInNozzleSizes))
	var selected map[string]any
	allKeys := make(map[string]struct{})
	for _, size := range builtInNozzleSizes {
		variant, err := loadU1Baseline(size)
		if err != nil {
			return u1Baseline{}, err
		}
		variants = append(variants, variant.projectSettings)
		if size == nozzle {
			selected = variant.projectSettings
		}
		for key := range variant.projectSettings {
			allKeys[key] = struct{}{}
		}
	}

	// Overlay only settings that vary across nozzle baselines so unrelated
	// template customizations remain intact.
	for key := range allKeys {
		reference, referenceExists := variants[0][key]
		varies := false
		for _, variant := range variants[1:] {
			value, exists := variant[key]
			if exists != referenceExists || !reflect.DeepEqual(value, reference) {
				varies = true
				break
			}
		}
		if !varies {
			continue
		}
		if value, exists := selected[key]; exists {
			baseline.projectSettings[key] = value
		} else {
			delete(baseline.projectSettings, key)
		}
	}
	return baseline, nil
}

func loadU1BaselineFrom3MF(path string) (u1Baseline, error) {
	template, err := openArchive(path)
	if err != nil {
		return u1Baseline{}, fmt.Errorf("open template: %w", err)
	}
	defer template.reader.Close()
	settings, err := readJSONMap(template.files[projectSettingsName])
	if err != nil {
		return u1Baseline{}, fmt.Errorf("template project settings: %w", err)
	}
	sliceInfo, err := readMember(template.files[sliceInfoName])
	if err != nil {
		return u1Baseline{}, fmt.Errorf("template slice info: %w", err)
	}
	model, err := readMember(template.files[mainModelName])
	if err != nil {
		return u1Baseline{}, fmt.Errorf("template main model: %w", err)
	}
	application := applicationRE.Find(model)
	if application == nil {
		return u1Baseline{}, fmt.Errorf("template Application metadata is missing")
	}
	return u1Baseline{
		projectSettings: settings,
		sliceInfo:       sliceInfo,
		application:     application,
	}, nil
}
