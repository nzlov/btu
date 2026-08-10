package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

type Language string

const (
	English Language = "en"
	Chinese Language = "zh"
)

type Message string

const (
	AppUsage                              Message = "app_usage"
	HelpTemplate                          Message = "help_template"
	HelpShow                              Message = "help_show"
	FlagNozzleUsage                       Message = "flag_nozzle_usage"
	FlagTemplateUsage                     Message = "flag_template_usage"
	FlagMixModeUsage                      Message = "flag_mix_mode_usage"
	FlagSubdivideLayerHeightUsage         Message = "flag_subdivide_layer_height_usage"
	FlagOutputUsage                       Message = "flag_output_usage"
	FlagReplaceUsage                      Message = "flag_replace_usage"
	FlagColorsUsage                       Message = "flag_colors_usage"
	FlagFullSpectrumUsage                 Message = "flag_full_spectrum_usage"
	ErrorIncorrectUsage                   Message = "error_incorrect_usage"
	ErrorInvalidArguments                 Message = "error_invalid_arguments"
	ErrorExpectedSource                   Message = "error_expected_source"
	ErrorInvalidNozzle                    Message = "error_invalid_nozzle"
	ErrorInvalidPalette                   Message = "error_invalid_palette"
	ErrorInvalidMixMode                   Message = "error_invalid_mix_mode"
	ErrorColorPlanInteractiveRequired     Message = "error_color_plan_interactive_required"
	ErrorRerunLocalZ                      Message = "error_rerun_local_z"
	ErrorRerunFullSpectrum                Message = "error_rerun_full_spectrum"
	ErrorRerunReplace                     Message = "error_rerun_replace"
	ErrorOutputNotReplaced                Message = "error_output_not_replaced"
	ErrorInvalidRequiredColors            Message = "error_invalid_required_colors"
	ErrorPreviewFailed                    Message = "error_preview_failed"
	ErrorColorPlanReviewFailed            Message = "error_color_plan_review_failed"
	ErrorConversionFailed                 Message = "error_conversion_failed"
	PromptGenerateFullSpectrum            Message = "prompt_generate_full_spectrum"
	PromptReplaceOutput                   Message = "prompt_replace_output"
	OutputConverted                       Message = "output_converted"
	OutputConvertedWithMixes              Message = "output_converted_with_mixes"
	OutputRequiredColors                  Message = "output_required_colors"
	OutputSourceMapping                   Message = "output_source_mapping"
	ModeRatio                             Message = "mode_ratio"
	ModeCycle                             Message = "mode_cycle"
	ModeMatch                             Message = "mode_match"
	ModeGradient                          Message = "mode_gradient"
	PrintingStepsOne                      Message = "printing_steps_one"
	PrintingStepsMany                     Message = "printing_steps_many"
	PrintingStepKeep                      Message = "printing_step_keep"
	PrintingStepReplace                   Message = "printing_step_replace"
	PrintingPlate                         Message = "printing_plate"
	ColorCyan                             Message = "color_cyan"
	ColorMagenta                          Message = "color_magenta"
	ColorYellow                           Message = "color_yellow"
	ColorGray                             Message = "color_gray"
	ColorWhite                            Message = "color_white"
	ColorBlack                            Message = "color_black"
	ProgressPreparing                     Message = "progress_preparing"
	ProgressOpenSource                    Message = "progress_open_source"
	ProgressAnalyzeColorPlan              Message = "progress_analyze_color_plan"
	ProgressBuildColorEditor              Message = "progress_build_color_editor"
	ProgressColorPlanReady                Message = "progress_color_plan_ready"
	ProgressAnalyzeMaterials              Message = "progress_analyze_materials"
	ProgressEncodeProjectSettings         Message = "progress_encode_project_settings"
	ProgressRewritePackage                Message = "progress_rewrite_package"
	ProgressVerifyOutput                  Message = "progress_verify_output"
	ProgressPublishOutput                 Message = "progress_publish_output"
	ProgressComplete                      Message = "progress_complete"
	ProgressSourceColors                  Message = "progress_source_colors"
	ProgressVirtualMaterials              Message = "progress_virtual_materials"
	ProgressU1Baseline                    Message = "progress_u1_baseline"
	ProgressMaterialMapping               Message = "progress_material_mapping"
	ProgressPrepareOutputArchive          Message = "progress_prepare_output_archive"
	ProgressOpenOutputArchive             Message = "progress_open_output_archive"
	ConfirmNonInteractive                 Message = "confirm_non_interactive"
	ColorPlanReviewTitle                  Message = "color_plan_review_title"
	ColorPlanOriginalOrder                Message = "color_plan_original_order"
	ColorPlanNewOrder                     Message = "color_plan_new_order"
	FullSpectrumConfigureTitle            Message = "full_spectrum_configure_title"
	LocalZSettings                        Message = "local_z_settings"
	StateUnused                           Message = "state_unused"
	StateUsed                             Message = "state_used"
	ActionKeep                            Message = "action_keep"
	KindMaterial                          Message = "kind_material"
	KindBase                              Message = "kind_base"
	KindMixed                             Message = "kind_mixed"
	LocalZLayerHeight                     Message = "local_z_layer_height"
	LocalZInfill                          Message = "local_z_infill"
	LocalZWholeObjects                    Message = "local_z_whole_objects"
	StateDisabled                         Message = "state_disabled"
	StateEnabled                          Message = "state_enabled"
	ColorPlanNonInteractive               Message = "color_plan_non_interactive"
	ColorPlanRequiresSourceOutputAndModes Message = "color_plan_requires_source_output_and_modes"
	ColorPlanRequiresSourceAndOutput      Message = "color_plan_requires_source_and_output"
	ColorPlanInvalidOutput                Message = "color_plan_invalid_output"
	ColorPlanPhysicalSlotsRequired        Message = "color_plan_physical_slots_required"
	ColorPlanMixModeOutOfRange            Message = "color_plan_mix_mode_out_of_range"
	ColorPlanReplacementOutOfRange        Message = "color_plan_replacement_out_of_range"
	ColorPlanMixModesRequired             Message = "color_plan_mix_modes_required"
	ColorPlanInvalidSource                Message = "color_plan_invalid_source"
)

//go:embed locales/*.json
var localeFiles embed.FS

var catalogs = map[Language]map[Message]string{
	English: loadCatalog("locales/en.json"),
	Chinese: loadCatalog("locales/zh.json"),
}

type Localizer struct {
	language Language
}

func FromLANG(value string) Localizer {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "zh" || strings.HasPrefix(value, "zh_") || strings.HasPrefix(value, "zh-") || strings.HasPrefix(value, "zh.") || strings.HasPrefix(value, "zh@") {
		return Localizer{language: Chinese}
	}
	return Localizer{language: English}
}

func EnglishLocalizer() Localizer {
	return Localizer{language: English}
}

func (localizer Localizer) IsChinese() bool {
	return localizer.language == Chinese
}

func (localizer Localizer) Text(message Message) string {
	if text, found := catalogs[localizer.language][message]; found {
		return text
	}
	if text, found := catalogs[English][message]; found {
		return text
	}
	return string(message)
}

func (localizer Localizer) Format(message Message, values ...any) string {
	return fmt.Sprintf(localizer.Text(message), values...)
}

func (localizer Localizer) Wrap(message Message, err error) error {
	return fmt.Errorf(localizer.Text(message), err)
}

func loadCatalog(path string) map[Message]string {
	data, err := localeFiles.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		panic(err)
	}
	catalog := make(map[Message]string, len(raw))
	for key, value := range raw {
		catalog[Message(key)] = value
	}
	return catalog
}
