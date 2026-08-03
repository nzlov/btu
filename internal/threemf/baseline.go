package threemf

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed assets/project_settings.config
var baselineProjectSettings []byte

//go:embed assets/slice_info.config
var baselineSliceInfo []byte

//go:embed assets/3dmodel.model
var baselineModel []byte

type u1Baseline struct {
	projectSettings map[string]any
	sliceInfo       []byte
	application     []byte
}

func loadU1Baseline() (u1Baseline, error) {
	var settings map[string]any
	if err := json.Unmarshal(baselineProjectSettings, &settings); err != nil {
		return u1Baseline{}, fmt.Errorf("decode built-in U1 project settings: %w", err)
	}
	application := applicationRE.Find(baselineModel)
	if application == nil {
		return u1Baseline{}, fmt.Errorf("built-in U1 Application metadata is missing")
	}
	return u1Baseline{
		projectSettings: settings,
		sliceInfo:       baselineSliceInfo,
		application:     application,
	}, nil
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
