package cli

import (
	"errors"

	urfavecli "github.com/urfave/cli/v3"

	"github.com/nzlov/btu/internal/i18n"
	"github.com/nzlov/btu/internal/progressui"
	"github.com/nzlov/btu/internal/threemf"
)

func localizedHelpTemplate(localizer i18n.Localizer) string {
	if localizer.IsChinese() {
		return localizer.Text(i18n.HelpTemplate)
	}
	return ""
}

func localizedNozzleError(localizer i18n.Localizer, value string, err error) error {
	if err == nil || !localizer.IsChinese() {
		return err
	}
	return errors.New(localizer.Format(i18n.ErrorInvalidNozzle, value))
}

func localizedPaletteError(localizer i18n.Localizer, value string, err error) error {
	if err == nil || !localizer.IsChinese() {
		return err
	}
	return errors.New(localizer.Format(i18n.ErrorInvalidPalette, value))
}

func localizedMixModeError(localizer i18n.Localizer, value string, err error) error {
	if err == nil || !localizer.IsChinese() {
		return err
	}
	return errors.New(localizer.Format(i18n.ErrorInvalidMixMode, value))
}

func localizedUsageError(localizer i18n.Localizer, command *urfavecli.Command, err error) error {
	if !localizer.IsChinese() {
		return err
	}
	if value := command.String("colors"); value != "" {
		if _, parseErr := threemf.ParsePalette(value); parseErr != nil {
			return localizedPaletteError(localizer, value, parseErr)
		}
	}
	if value := command.String("mix-mode"); value != "" {
		if _, parseErr := threemf.ParseMixMode(value); parseErr != nil {
			return localizedMixModeError(localizer, value, parseErr)
		}
	}
	if command.IsSet("nozzle") {
		value := command.String("nozzle")
		if _, parseErr := threemf.ParseNozzleSize(value); parseErr != nil {
			return localizedNozzleError(localizer, value, parseErr)
		}
	}
	return errors.New(localizer.Text(i18n.ErrorInvalidArguments))
}

// GLUE: converts domain progress stages into localized terminal text.
func localizedProgress(localizer i18n.Localizer, event threemf.Progress) progressui.Progress {
	detail := event.Detail.Value
	switch event.Stage {
	case threemf.ProgressStageColorPlanReady:
		detail = localizer.Format(i18n.ProgressSourceColors, event.DetailCount)
	case threemf.ProgressStageEncodeProjectSettings:
		detail = localizer.Format(i18n.ProgressVirtualMaterials, event.DetailCount)
	}
	switch event.Detail.Kind {
	case threemf.ProgressDetailU1Baseline:
		detail = localizer.Text(i18n.ProgressU1Baseline)
	case threemf.ProgressDetailMaterialMapping:
		detail = localizer.Text(i18n.ProgressMaterialMapping)
	case threemf.ProgressDetailPrepareOutputArchive:
		detail = localizer.Text(i18n.ProgressPrepareOutputArchive)
	case threemf.ProgressDetailOpenOutputArchive:
		detail = localizer.Text(i18n.ProgressOpenOutputArchive)
	}
	return progressui.Progress{
		Current: event.Current, Total: event.Total, Stage: localizer.Text(progressMessage(event.Stage)), Detail: detail,
		ItemCurrent: event.ItemCurrent, ItemTotal: event.ItemTotal,
	}
}

func progressMessage(stage threemf.ProgressStage) i18n.Message {
	switch stage {
	case threemf.ProgressStageOpenSource:
		return i18n.ProgressOpenSource
	case threemf.ProgressStageAnalyzeColorPlan:
		return i18n.ProgressAnalyzeColorPlan
	case threemf.ProgressStageBuildColorEditor:
		return i18n.ProgressBuildColorEditor
	case threemf.ProgressStageColorPlanReady:
		return i18n.ProgressColorPlanReady
	case threemf.ProgressStageAnalyzeMaterials:
		return i18n.ProgressAnalyzeMaterials
	case threemf.ProgressStageEncodeProjectSettings:
		return i18n.ProgressEncodeProjectSettings
	case threemf.ProgressStageRewritePackage:
		return i18n.ProgressRewritePackage
	case threemf.ProgressStageVerifyOutput:
		return i18n.ProgressVerifyOutput
	case threemf.ProgressStagePublishOutput:
		return i18n.ProgressPublishOutput
	case threemf.ProgressStageComplete:
		return i18n.ProgressComplete
	default:
		return ""
	}
}

func localizedColorRole(localizer i18n.Localizer, role threemf.ColorRole) string {
	switch role {
	case threemf.ColorCyan:
		return localizer.Text(i18n.ColorCyan)
	case threemf.ColorMagenta:
		return localizer.Text(i18n.ColorMagenta)
	case threemf.ColorYellow:
		return localizer.Text(i18n.ColorYellow)
	case threemf.ColorGray:
		return localizer.Text(i18n.ColorGray)
	case threemf.ColorWhite:
		return localizer.Text(i18n.ColorWhite)
	case threemf.ColorBlack:
		return localizer.Text(i18n.ColorBlack)
	default:
		return string(role)
	}
}
