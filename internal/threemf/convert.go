package threemf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	projectSettingsName     = "Metadata/project_settings.config"
	processSettingsName     = "Metadata/process_settings_1.config"
	modelSettingsName       = "Metadata/model_settings.config"
	mainModelName           = "3D/3dmodel.model"
	sliceInfoName           = "Metadata/slice_info.config"
	projectProcessPreset    = "btu"
	projectProcessVersion   = "2.2.53.2"
	projectSettingSlotCount = 6
)

var (
	extruderPattern = regexp.MustCompile(`(<metadata\s+key="extruder"\s+value=")(\d+)(")`)
	paintPattern    = regexp.MustCompile(`(paint_color=")([0-9A-Fa-f]+)(")`)
	applicationRE   = regexp.MustCompile(`(?s)<metadata name="Application">.*?</metadata>`)
	platePattern    = regexp.MustCompile(`^Metadata/plate_\d+\.json$`)
	filamentPattern = regexp.MustCompile(`^Metadata/filament_settings_\d+\.config$`)
	processPattern  = regexp.MustCompile(`^Metadata/process_settings_\d+\.config$`)
)

type LocalZSettings struct {
	LayerHeight  bool
	Infill       bool
	WholeObjects bool
}

type Request struct {
	Source                string
	Template              string
	Output                string
	Replace               bool
	Nozzle                string
	Palette               Palette
	FullSpectrum          bool
	LocalZ                LocalZSettings
	PreserveMaterialSlots bool
	MixMode               MixMode
	MaterialMixModes      map[int]MixMode
	MaterialReplacements  map[int]MaterialReplacement
}

type OutputExistsError struct {
	Path string
}

func (err *OutputExistsError) Error() string {
	return fmt.Sprintf("output already exists: %s", err.Path)
}

type Progress struct {
	Current     int
	Total       int
	Stage       string
	Detail      string
	ItemCurrent int
	ItemTotal   int
}

type ProgressFunc func(Progress)

type itemProgressFunc func(current, total int, detail string)

func reportItemProgress(progress itemProgressFunc, current, total int, detail string) {
	if progress != nil {
		progress(current, total, detail)
	}
}

type Report struct {
	Mode            string
	Output          string
	Colors          string
	PhysicalMapping map[int]int
	VirtualMixes    int
	Plates          []PlateReport
}

type archive struct {
	reader *zip.ReadCloser
	files  map[string]*zip.File
}

type conversionAnalysis struct {
	sourceSettings map[string]any
	usage          projectMaterialUsage
	baseline       u1Baseline
	plan           materialPlan
}

func PreviewColorPlan(ctx context.Context, request Request, progress ProgressFunc) (ColorSequence, error) {
	if progress == nil {
		progress = func(Progress) {}
	}
	if request.Source == "" {
		return ColorSequence{}, fmt.Errorf("source path is required")
	}
	palette, err := request.Palette.normalized()
	if err != nil {
		return ColorSequence{}, err
	}
	progress(Progress{Current: 0, Total: 3, Stage: "Open source", Detail: request.Source})
	source, err := openArchive(request.Source)
	if err != nil {
		return ColorSequence{}, fmt.Errorf("open source: %w", err)
	}
	defer source.reader.Close()

	analysisTotal := analyzeConversionWorkTotal(source)
	progress(Progress{Current: 1, Total: 3, Stage: "Analyze color plan", ItemTotal: analysisTotal})
	analysis, err := analyzeConversion(ctx, source, request, palette, func(current, total int, detail string) {
		progress(Progress{
			Current: 1, Total: 3, Stage: "Analyze color plan", Detail: detail,
			ItemCurrent: current, ItemTotal: total,
		})
	})
	if err != nil {
		var required *FullSpectrumRequiredError
		if errors.As(err, &required) {
			progress(Progress{Current: 2, Total: 3, Stage: "Build color editor"})
			progress(Progress{Current: 3, Total: 3, Stage: "Color plan ready", Detail: fmt.Sprintf("%d source colors", len(required.Sequence.Source))})
		}
		return ColorSequence{}, err
	}
	progress(Progress{Current: 2, Total: 3, Stage: "Build color editor"})
	colors := stringSlice(analysis.sourceSettings["filament_colour"])
	sequence, err := makeColorSequence(colors, analysis.usage.Total, analysis.plan)
	if err != nil {
		return ColorSequence{}, err
	}
	progress(Progress{Current: 3, Total: 3, Stage: "Color plan ready", Detail: fmt.Sprintf("%d source colors", len(sequence.Source))})
	return sequence, nil
}

func Convert(ctx context.Context, request Request, progress ProgressFunc) (Report, error) {
	if progress == nil {
		progress = func(Progress) {}
	}
	if request.Source == "" || request.Output == "" {
		return Report{}, fmt.Errorf("source and output paths are required")
	}
	palette, err := request.Palette.normalized()
	if err != nil {
		return Report{}, err
	}
	if samePath(request.Source, request.Output) {
		return Report{}, fmt.Errorf("output must differ from source")
	}
	if request.Template != "" && samePath(request.Template, request.Output) {
		return Report{}, fmt.Errorf("output must differ from template")
	}
	if info, err := os.Stat(request.Output); err == nil {
		if info.IsDir() {
			return Report{}, fmt.Errorf("output path is a directory: %s", request.Output)
		}
		if !request.Replace {
			return Report{}, &OutputExistsError{Path: request.Output}
		}
	} else if !os.IsNotExist(err) {
		return Report{}, fmt.Errorf("check output: %w", err)
	}

	progress(Progress{Current: 0, Total: 6, Stage: "Open source", Detail: request.Source})
	source, err := openArchive(request.Source)
	if err != nil {
		return Report{}, fmt.Errorf("open source: %w", err)
	}
	defer source.reader.Close()

	analysisTotal := analyzeConversionWorkTotal(source)
	progress(Progress{Current: 1, Total: 6, Stage: "Analyze materials", ItemTotal: analysisTotal})
	analysis, err := analyzeConversion(ctx, source, request, palette, func(current, total int, detail string) {
		progress(Progress{
			Current: 1, Total: 6, Stage: "Analyze materials", Detail: detail,
			ItemCurrent: current, ItemTotal: total,
		})
	})
	if err != nil {
		return Report{}, err
	}
	sourceSettings := analysis.sourceSettings
	baseline := analysis.baseline
	plan := analysis.plan

	progress(Progress{Current: 2, Total: 6, Stage: "Encode project settings", Detail: fmt.Sprintf("%d virtual materials", plan.virtualMixes)})
	mergedSettings, hasProjectProcessSettings := mergeProjectSettings(sourceSettings, baseline.projectSettings, plan, request.FullSpectrum, request.LocalZ)
	var processData []byte
	if hasProjectProcessSettings {
		processSettings := buildProjectProcessSettings(mergedSettings, baseline.projectSettings)
		processData, err = json.MarshalIndent(processSettings, "", "    ")
		if err != nil {
			return Report{}, fmt.Errorf("encode process settings: %w", err)
		}
		processData = append(processData, '\n')
	}
	projectData, err := json.MarshalIndent(mergedSettings, "", "    ")
	if err != nil {
		return Report{}, fmt.Errorf("encode project settings: %w", err)
	}
	projectData = append(projectData, '\n')
	modelSettings, err := readMember(source.files[modelSettingsName])
	if err != nil {
		return Report{}, fmt.Errorf("read model settings: %w", err)
	}
	modelSettings, err = remapModelSettings(modelSettings, plan.allMapping)
	if err != nil {
		return Report{}, err
	}

	writeTotal := writeArchiveWorkTotal(source, len(processData) > 0)
	progress(Progress{Current: 3, Total: 6, Stage: "Rewrite 3MF package", ItemTotal: writeTotal})
	temporary, err := os.CreateTemp(filepath.Dir(request.Output), "."+filepath.Base(request.Output)+".*.tmp")
	if err != nil {
		return Report{}, fmt.Errorf("create temporary output: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()

	if err := writeArchive(ctx, temporary, source, baseline, projectData, processData, modelSettings, plan, func(current, total int, detail string) {
		progress(Progress{
			Current: 3, Total: 6, Stage: "Rewrite 3MF package", Detail: detail,
			ItemCurrent: current, ItemTotal: total,
		})
	}); err != nil {
		return Report{}, err
	}
	if err := temporary.Close(); err != nil {
		return Report{}, fmt.Errorf("close temporary output: %w", err)
	}

	progress(Progress{Current: 4, Total: 6, Stage: "Verify output"})
	if err := verifyArchive(temporaryName, plan, hasProjectProcessSettings, func(current, total int, detail string) {
		progress(Progress{
			Current: 4, Total: 6, Stage: "Verify output", Detail: detail,
			ItemCurrent: current, ItemTotal: total,
		})
	}); err != nil {
		return Report{}, err
	}
	progress(Progress{Current: 5, Total: 6, Stage: "Publish output", Detail: request.Output})
	if err := os.Chmod(temporaryName, 0o644); err != nil {
		return Report{}, fmt.Errorf("set output permissions: %w", err)
	}
	if err := os.Rename(temporaryName, request.Output); err != nil {
		return Report{}, fmt.Errorf("publish output: %w", err)
	}
	committed = true
	progress(Progress{Current: 6, Total: 6, Stage: "Complete", Detail: request.Output})

	return Report{
		Mode:            plan.mode,
		Output:          request.Output,
		Colors:          plan.colorSummary(),
		PhysicalMapping: plan.physicalMapping,
		VirtualMixes:    plan.virtualMixes,
		Plates:          plan.plates,
	}, nil
}

func analyzeConversionWorkTotal(source archive) int {
	return materialUsageWorkTotal(source) + 3
}

func analyzeConversion(ctx context.Context, source archive, request Request, palette Palette, progress itemProgressFunc) (conversionAnalysis, error) {
	total := analyzeConversionWorkTotal(source)
	current := 0
	if err := ctx.Err(); err != nil {
		return conversionAnalysis{}, err
	}
	sourceSettings, err := readJSONMap(source.files[projectSettingsName])
	if err != nil {
		return conversionAnalysis{}, fmt.Errorf("source project settings: %w", err)
	}
	current++
	reportItemProgress(progress, current, total, projectSettingsName)
	usageTotal := materialUsageWorkTotal(source)
	usage, err := analyzeMaterialUsage(source, func(usageCurrent, _ int, detail string) {
		reportItemProgress(progress, current+usageCurrent, total, detail)
	})
	if err != nil {
		return conversionAnalysis{}, err
	}
	current += usageTotal
	baseline, err := loadRequestedU1Baseline(request.Template, request.Nozzle, sourceSettings)
	if err != nil {
		return conversionAnalysis{}, err
	}
	current++
	reportItemProgress(progress, current, total, "U1 baseline")
	plan, err := planMaterials(sourceSettings, baseline.projectSettings, palette, materialPlanOptions{
		fullSpectrum:    request.FullSpectrum,
		preserveSlots:   request.PreserveMaterialSlots,
		mixMode:         request.MixMode,
		materialMixMode: request.MaterialMixModes,
		replacements:    request.MaterialReplacements,
	}, usage)
	current++
	reportItemProgress(progress, current, total, "Material mapping")
	if err != nil {
		return conversionAnalysis{}, err
	}
	return conversionAnalysis{sourceSettings: sourceSettings, usage: usage, baseline: baseline, plan: plan}, nil
}

func openArchive(path string) (archive, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return archive{}, err
	}
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if _, duplicate := files[file.Name]; duplicate {
			reader.Close()
			return archive{}, fmt.Errorf("duplicate member %q", file.Name)
		}
		files[file.Name] = file
	}
	for _, required := range []string{projectSettingsName, modelSettingsName, mainModelName} {
		if files[required] == nil {
			reader.Close()
			return archive{}, fmt.Errorf("missing required member %q", required)
		}
	}
	return archive{reader: reader, files: files}, nil
}

func skipSourceMember(file *zip.File) bool {
	return filamentPattern.MatchString(file.Name) || processPattern.MatchString(file.Name)
}

func writeArchiveWorkTotal(source archive, writeProcessSettings bool) int {
	total := 0
	hasSliceInfo := false
	for _, file := range source.reader.File {
		if skipSourceMember(file) {
			continue
		}
		total++
		hasSliceInfo = hasSliceInfo || file.Name == sliceInfoName
	}
	if !hasSliceInfo {
		total++
	}
	if writeProcessSettings {
		total++
	}
	return total
}

func writeArchive(ctx context.Context, output *os.File, source archive, baseline u1Baseline, projectData, processData, modelSettings []byte, plan materialPlan, progress itemProgressFunc) error {
	writer := zip.NewWriter(output)
	writtenSliceInfo := false
	total := writeArchiveWorkTotal(source, len(processData) > 0)
	current := 0
	reportItemProgress(progress, current, total, "Prepare output archive")
	for _, file := range source.reader.File {
		if err := ctx.Err(); err != nil {
			writer.Close()
			return err
		}
		if skipSourceMember(file) {
			continue
		}

		var data []byte
		modified := true
		switch {
		case file.Name == projectSettingsName:
			data = projectData
		case file.Name == modelSettingsName:
			data = modelSettings
		case file.Name == sliceInfoName:
			writtenSliceInfo = true
			data = baseline.sliceInfo
		case file.Name == mainModelName:
			var err error
			data, err = readMember(file)
			if err != nil {
				writer.Close()
				return err
			}
			data, err = replaceApplication(data, baseline.application)
			if err != nil {
				writer.Close()
				return err
			}
			data, err = remapPaintAttributes(data, plan.allMapping)
			if err != nil {
				writer.Close()
				return fmt.Errorf("%s: %w", file.Name, err)
			}
		case strings.HasSuffix(file.Name, ".model"):
			var err error
			data, err = readMember(file)
			if err != nil {
				writer.Close()
				return err
			}
			data, err = remapPaintAttributes(data, plan.allMapping)
			if err != nil {
				writer.Close()
				return fmt.Errorf("%s: %w", file.Name, err)
			}
		case platePattern.MatchString(file.Name):
			var err error
			data, err = rewritePlate(file, baseline.projectSettings, plan.allMapping)
			if err != nil {
				writer.Close()
				return fmt.Errorf("%s: %w", file.Name, err)
			}
		default:
			modified = false
		}

		if !modified {
			if err := writer.Copy(file); err != nil {
				writer.Close()
				return fmt.Errorf("copy %s: %w", file.Name, err)
			}
		} else {
			if err := writeModifiedMember(writer, file.FileHeader, data); err != nil {
				writer.Close()
				return fmt.Errorf("write %s: %w", file.Name, err)
			}
		}
		current++
		reportItemProgress(progress, current, total, file.Name)
	}

	if !writtenSliceInfo {
		header := zip.FileHeader{Name: sliceInfoName, Method: zip.Deflate}
		if err := writeModifiedMember(writer, header, baseline.sliceInfo); err != nil {
			writer.Close()
			return fmt.Errorf("write slice info: %w", err)
		}
		current++
		reportItemProgress(progress, current, total, sliceInfoName)
	}
	if len(processData) > 0 {
		processHeader := zip.FileHeader{Name: processSettingsName, Method: zip.Deflate}
		if err := writeModifiedMember(writer, processHeader, processData); err != nil {
			writer.Close()
			return fmt.Errorf("write process settings: %w", err)
		}
		current++
		reportItemProgress(progress, current, total, processSettingsName)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalize output archive: %w", err)
	}
	return nil
}

func writeModifiedMember(writer *zip.Writer, source zip.FileHeader, data []byte) error {
	source.CRC32 = 0
	source.CompressedSize = 0
	source.CompressedSize64 = 0
	source.UncompressedSize = 0
	source.UncompressedSize64 = 0
	destination, err := writer.CreateHeader(&source)
	if err != nil {
		return err
	}
	_, err = destination.Write(data)
	return err
}

func mergeProjectSettings(source, template map[string]any, plan materialPlan, fullSpectrum bool, localZSettings LocalZSettings) (map[string]any, bool) {
	merged := make(map[string]any, len(template)+8)
	for key, value := range template {
		merged[key] = value
	}
	mergeMaterialSettings(merged, source, plan)
	for _, key := range []string{"layer_height", "initial_layer_print_height"} {
		if value, ok := source[key]; ok {
			merged[key] = value
		}
	}
	merged["mixed_filament_definitions"] = plan.definitions
	merged["filament_colour"] = plan.outputColors()
	localZ := "0"
	infill := "0"
	wholeObjects := "0"
	if fullSpectrum {
		merged["prime_volume"] = "20"
		if localZSettings.LayerHeight {
			localZ = "1"
		}
		if localZSettings.Infill {
			infill = "1"
		}
		if localZSettings.WholeObjects {
			wholeObjects = "1"
		}
	} else if !plan.preserveSlots && source["enable_mixed_color_sublayer"] == "1" {
		localZ = "1"
		infill = "1"
	}
	merged["dithering_local_z_mode"] = localZ
	merged["dithering_local_z_infill"] = infill
	merged["dithering_local_z_whole_objects"] = wholeObjects
	merged["dithering_step_painted_zones_only"] = "1"
	directMulticolor := "0"
	if localZ == "1" && plan.hasMultiColor {
		directMulticolor = "1"
	}
	merged["dithering_local_z_direct_multicolor"] = directMulticolor
	merged["mixed_filament_gradient_mode"] = "0"
	hasProjectProcessSettings := mergeProjectSettingOverrides(merged, template,
		"layer_height",
		"initial_layer_print_height",
		"mixed_filament_definitions",
		"prime_volume",
		"dithering_local_z_mode",
		"dithering_local_z_infill",
		"dithering_local_z_whole_objects",
		"dithering_step_painted_zones_only",
		"mixed_filament_gradient_mode",
	)
	return merged, hasProjectProcessSettings
}

func mergeProjectSettingOverrides(settings, baseline map[string]any, keys ...string) bool {
	slots := stringSlice(settings["different_settings_to_system"])
	overrides := make(map[string]struct{})
	if len(slots) > 0 {
		for _, key := range strings.Split(slots[0], ";") {
			if key != "" {
				overrides[key] = struct{}{}
			}
		}
	}
	changed := false
	for _, key := range keys {
		value, exists := settings[key]
		baselineValue, baselineExists := baseline[key]
		if exists != baselineExists || !reflect.DeepEqual(value, baselineValue) {
			overrides[key] = struct{}{}
			changed = true
		}
	}
	if !changed {
		return false
	}
	if len(slots) < projectSettingSlotCount {
		slots = append(slots, make([]string, projectSettingSlotCount-len(slots))...)
	}
	processOverrides := make([]string, 0, len(overrides))
	for key := range overrides {
		processOverrides = append(processOverrides, key)
	}
	slices.Sort(processOverrides)
	slots[0] = strings.Join(processOverrides, ";")
	settings["different_settings_to_system"] = slots
	return true
}

func buildProjectProcessSettings(settings, baseline map[string]any) map[string]any {
	basePreset, _ := baseline["print_settings_id"].(string)
	process := map[string]any{
		"from":              "project",
		"inherits":          basePreset,
		"name":              projectProcessPreset,
		"print_settings_id": projectProcessPreset,
		"version":           projectProcessVersion,
	}
	overrideSlots := stringSlice(settings["different_settings_to_system"])
	if len(overrideSlots) > 0 {
		for _, key := range strings.Split(overrideSlots[0], ";") {
			if value, exists := settings[key]; exists {
				process[key] = value
			}
		}
	}
	inheritsGroup := append([]string(nil), stringSlice(settings["inherits_group"])...)
	if len(inheritsGroup) < projectSettingSlotCount {
		inheritsGroup = append(inheritsGroup, make([]string, projectSettingSlotCount-len(inheritsGroup))...)
	}
	inheritsGroup[0] = basePreset
	settings["inherits_group"] = inheritsGroup
	settings["print_settings_id"] = projectProcessPreset
	return process
}

func remapModelSettings(data []byte, mapping map[int]int) ([]byte, error) {
	return replaceNumericReferences(data, extruderPattern, mapping, "model settings")
}

func remapPaintAttributes(data []byte, mapping map[int]int) ([]byte, error) {
	matches := paintPattern.FindAllSubmatchIndex(data, -1)
	if len(matches) == 0 {
		return data, nil
	}
	var output bytes.Buffer
	position := 0
	for _, match := range matches {
		output.Write(data[position:match[4]])
		mapped, err := remapPaintEncoding(string(data[match[4]:match[5]]), mapping)
		if err != nil {
			return nil, err
		}
		output.WriteString(mapped)
		position = match[5]
	}
	output.Write(data[position:])
	return output.Bytes(), nil
}

func replaceNumericReferences(data []byte, pattern *regexp.Regexp, mapping map[int]int, contextName string) ([]byte, error) {
	matches := pattern.FindAllSubmatchIndex(data, -1)
	var output bytes.Buffer
	position := 0
	for _, match := range matches {
		value, _ := strconv.Atoi(string(data[match[4]:match[5]]))
		mapped, ok := mapping[value]
		if !ok {
			return nil, fmt.Errorf("%s references unknown material T%d", contextName, value)
		}
		output.Write(data[position:match[4]])
		output.WriteString(strconv.Itoa(mapped))
		position = match[5]
	}
	output.Write(data[position:])
	return output.Bytes(), nil
}

func replaceApplication(source, application []byte) ([]byte, error) {
	if len(application) == 0 || applicationRE.Find(source) == nil {
		return nil, fmt.Errorf("Application metadata is missing")
	}
	location := applicationRE.FindIndex(source)
	result := make([]byte, 0, len(source)-location[1]+location[0]+len(application))
	result = append(result, source[:location[0]]...)
	result = append(result, application...)
	result = append(result, source[location[1]:]...)
	return result, nil
}

func rewritePlate(file *zip.File, baselineSettings map[string]any, mapping map[int]int) ([]byte, error) {
	data, err := readJSONMap(file)
	if err != nil {
		return nil, err
	}
	if raw, ok := data["first_extruder"].(float64); ok {
		sourceID := int(raw)
		if sourceID > 0 {
			if mapped, exists := mapping[sourceID]; exists {
				data["first_extruder"] = mapped
			}
		}
	}
	nozzles := stringSlice(baselineSettings["nozzle_diameter"])
	if len(nozzles) > 0 {
		if nozzle, err := strconv.ParseFloat(nozzles[0], 64); err == nil {
			data["nozzle_diameter"] = nozzle
		}
	}
	encoded, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func verifyArchive(path string, plan materialPlan, hasProjectProcessSettings bool, progress itemProgressFunc) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("verify output: %w", err)
	}
	defer reader.Close()
	total := len(reader.File) + 1
	current := 0
	reportItemProgress(progress, current, total, "Open output archive")
	found := make(map[string]bool)
	for _, file := range reader.File {
		member, err := file.Open()
		if err != nil {
			return fmt.Errorf("verify %s: %w", file.Name, err)
		}
		_, copyErr := io.Copy(io.Discard, member)
		closeErr := member.Close()
		if copyErr != nil {
			return fmt.Errorf("verify %s: %w", file.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("verify %s: %w", file.Name, closeErr)
		}
		found[file.Name] = true
		current++
		reportItemProgress(progress, current, total, file.Name)
	}
	for _, required := range []string{projectSettingsName, modelSettingsName, mainModelName} {
		if !found[required] {
			return fmt.Errorf("verify output: missing %s", required)
		}
	}
	if found[processSettingsName] != hasProjectProcessSettings {
		return fmt.Errorf("verify output: project process settings presence is %t, want %t", found[processSettingsName], hasProjectProcessSettings)
	}
	settings, err := readJSONMap(findFile(reader.File, projectSettingsName))
	if err != nil {
		return fmt.Errorf("verify project settings: %w", err)
	}
	definitions, _ := settings["mixed_filament_definitions"].(string)
	if definitions != plan.definitions {
		return fmt.Errorf("verify project settings: mixed definitions changed")
	}
	if colors := stringSlice(settings["filament_colour"]); !slices.Equal(colors, plan.outputColors()) {
		return fmt.Errorf("verify project settings: filament colors changed")
	}
	if plan.preserveSlots {
		flags := stringSlice(settings["filament_is_mixed"])
		if len(flags) != len(plan.slotColors) || slices.ContainsFunc(flags, func(flag string) bool { return flag != "0" }) {
			return fmt.Errorf("verify project settings: preserved material slots became mixed")
		}
		if settings["dithering_local_z_mode"] != "0" {
			return fmt.Errorf("verify project settings: Local-Z mixing remained enabled")
		}
	}
	current++
	reportItemProgress(progress, current, total, projectSettingsName)
	return nil
}

func readJSONMap(file *zip.File) (map[string]any, error) {
	if file == nil {
		return nil, fmt.Errorf("member is missing")
	}
	data, err := readMember(file)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func readMember(file *zip.File) ([]byte, error) {
	if file == nil {
		return nil, fmt.Errorf("member is missing")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func findFile(files []*zip.File, name string) *zip.File {
	for _, file := range files {
		if file.Name == name {
			return file
		}
	}
	return nil
}

func samePath(a, b string) bool {
	aPath, errA := filepath.Abs(a)
	bPath, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(aPath) == filepath.Clean(bPath)
}
