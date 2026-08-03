package threemf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	projectSettingsName = "Metadata/project_settings.config"
	modelSettingsName   = "Metadata/model_settings.config"
	mainModelName       = "3D/3dmodel.model"
	sliceInfoName       = "Metadata/slice_info.config"
)

var (
	extruderPattern = regexp.MustCompile(`(<metadata\s+key="extruder"\s+value=")(\d+)(")`)
	paintPattern    = regexp.MustCompile(`(paint_color=")([0-9A-Fa-f]+)(")`)
	applicationRE   = regexp.MustCompile(`(?s)<metadata name="Application">.*?</metadata>`)
	platePattern    = regexp.MustCompile(`^Metadata/plate_\d+\.json$`)
	filamentPattern = regexp.MustCompile(`^Metadata/filament_settings_\d+\.config$`)
)

type Request struct {
	Source       string
	Template     string
	Output       string
	Nozzle       string
	Palette      Palette
	FullSpectrum bool
}

type Progress struct {
	Current int
	Total   int
	Stage   string
	Detail  string
}

type ProgressFunc func(Progress)

type Report struct {
	Mode            string
	Output          string
	PhysicalMapping map[int]int
	VirtualMixes    int
}

type archive struct {
	reader *zip.ReadCloser
	files  map[string]*zip.File
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
	if _, err := os.Stat(request.Output); err == nil {
		return Report{}, fmt.Errorf("output already exists: %s", request.Output)
	} else if !os.IsNotExist(err) {
		return Report{}, fmt.Errorf("check output: %w", err)
	}

	progress(Progress{Current: 1, Total: 6, Stage: "Open source"})
	source, err := openArchive(request.Source)
	if err != nil {
		return Report{}, fmt.Errorf("open source: %w", err)
	}
	defer source.reader.Close()

	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	progress(Progress{Current: 2, Total: 6, Stage: "Analyze materials"})
	sourceSettings, err := readJSONMap(source.files[projectSettingsName])
	if err != nil {
		return Report{}, fmt.Errorf("source project settings: %w", err)
	}
	baseline, err := loadRequestedU1Baseline(request.Template, request.Nozzle, sourceSettings)
	if err != nil {
		return Report{}, err
	}
	plan, err := planMaterials(sourceSettings, baseline.projectSettings, palette, request.FullSpectrum)
	if err != nil {
		return Report{}, err
	}

	progress(Progress{Current: 3, Total: 6, Stage: "Translate mix definitions", Detail: fmt.Sprintf("%d virtual materials", plan.virtualMixes)})
	mergedSettings := mergeProjectSettings(sourceSettings, baseline.projectSettings, plan)
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

	progress(Progress{Current: 4, Total: 6, Stage: "Rewrite 3MF package"})
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

	if err := writeArchive(ctx, temporary, source, baseline, projectData, modelSettings, plan); err != nil {
		return Report{}, err
	}
	if err := temporary.Close(); err != nil {
		return Report{}, fmt.Errorf("close temporary output: %w", err)
	}

	progress(Progress{Current: 5, Total: 6, Stage: "Verify output"})
	if err := verifyArchive(temporaryName, plan); err != nil {
		return Report{}, err
	}
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
		PhysicalMapping: plan.physicalMapping,
		VirtualMixes:    plan.virtualMixes,
	}, nil
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

func writeArchive(ctx context.Context, output *os.File, source archive, baseline u1Baseline, projectData, modelSettings []byte, plan materialPlan) error {
	writer := zip.NewWriter(output)
	writtenSliceInfo := false
	for _, file := range source.reader.File {
		if err := ctx.Err(); err != nil {
			writer.Close()
			return err
		}
		if filamentPattern.MatchString(file.Name) {
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
			continue
		}
		if err := writeModifiedMember(writer, file.FileHeader, data); err != nil {
			writer.Close()
			return fmt.Errorf("write %s: %w", file.Name, err)
		}
	}

	if !writtenSliceInfo {
		header := zip.FileHeader{Name: sliceInfoName, Method: zip.Deflate}
		if err := writeModifiedMember(writer, header, baseline.sliceInfo); err != nil {
			writer.Close()
			return fmt.Errorf("write slice info: %w", err)
		}
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

func mergeProjectSettings(source, template map[string]any, plan materialPlan) map[string]any {
	merged := make(map[string]any, len(template)+8)
	for key, value := range template {
		merged[key] = value
	}
	for _, key := range []string{"layer_height", "initial_layer_print_height"} {
		if value, ok := source[key]; ok {
			merged[key] = value
		}
	}
	merged["mixed_filament_definitions"] = plan.definitions
	merged["filament_colour"] = plan.palette.outputColors()
	localZ := "0"
	if plan.forceLocalZ || source["enable_mixed_color_sublayer"] == "1" {
		localZ = "1"
	}
	merged["dithering_local_z_mode"] = localZ
	merged["dithering_local_z_infill"] = localZ
	merged["dithering_step_painted_zones_only"] = "1"
	directMulticolor := "0"
	if localZ == "1" && plan.hasThreeColor {
		directMulticolor = "1"
	}
	merged["dithering_local_z_direct_multicolor"] = directMulticolor
	merged["mixed_filament_gradient_mode"] = "0"
	return merged
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

func verifyArchive(path string, plan materialPlan) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("verify output: %w", err)
	}
	defer reader.Close()
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
	}
	for _, required := range []string{projectSettingsName, modelSettingsName, mainModelName} {
		if !found[required] {
			return fmt.Errorf("verify output: missing %s", required)
		}
	}
	settings, err := readJSONMap(findFile(reader.File, projectSettingsName))
	if err != nil {
		return fmt.Errorf("verify project settings: %w", err)
	}
	definitions, _ := settings["mixed_filament_definitions"].(string)
	if definitions != plan.definitions {
		return fmt.Errorf("verify project settings: mixed definitions changed")
	}
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
