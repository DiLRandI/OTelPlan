package cli

import "github.com/DiLRandI/OTelPlan/pkg/model"

// Free-form commands and flags can contain secrets. Redact an output copy so
// lock fingerprints and build validation retain the original inputs.
func previewInventory(code *model.CodeModel) *model.CodeModel {
	if code == nil {
		return nil
	}

	preview := *code
	build := &preview.EffectiveBuild
	build.CGOCFLAGS = redactBuildValue(build.CGOCFLAGS)
	build.CGOCPPFLAGS = redactBuildValue(build.CGOCPPFLAGS)
	build.CGOCXXFLAGS = redactBuildValue(build.CGOCXXFLAGS)
	build.CGOLDFLAGS = redactBuildValue(build.CGOLDFLAGS)
	build.CGOFFLAGS = redactBuildValue(build.CGOFFLAGS)
	build.CC = redactBuildValue(build.CC)
	build.CXX = redactBuildValue(build.CXX)

	return &preview
}

func redactBuildValue(value string) string {
	if value == "" {
		return ""
	}

	return "[redacted]"
}
