package otelc

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// The pin reparses the complete hook file for each before/after hook lookup.
const maximumHookPackageTargets = 64

func renderBundleHooks(version, runtimeVersion string, code *model.CodeModel,
	plan model.ResolvedPlan, modulePath string) ([]GeneratedFile, error) {
	if len(plan.Targets) <= maximumHookPackageTargets {
		return renderHookGroup(version, runtimeVersion, code, plan, modulePath, "hooks", "hooks")
	}

	// Validate global identities before partitioning, including cross-group duplicates.
	_, _, err := RenderRules(version, code, plan, modulePath+"/hooks")
	if err != nil {
		return nil, err
	}

	targets := slices.Clone(plan.Targets)
	slices.SortFunc(targets, func(a, b model.ResolvedTarget) int { return cmp.Compare(a.SymbolID, b.SymbolID) })

	var files []GeneratedFile

	for start := 0; start < len(targets); start += maximumHookPackageTargets {
		group := plan
		group.Targets = targets[start:min(start+maximumHookPackageTargets, len(targets))]
		name := fmt.Sprintf("hooks_%04d", start/maximumHookPackageTargets)
		directory := fmt.Sprintf("hooks/part_%04d", start/maximumHookPackageTargets)

		generated, err := renderHookGroup(version, runtimeVersion, code, group, modulePath, directory, name)
		if err != nil {
			return nil, err
		}

		files = append(files, generated...)
	}

	return files, nil
}

func renderHookGroup(version, runtimeVersion string, code *model.CodeModel,
	plan model.ResolvedPlan, modulePath, directory, ruleName string) ([]GeneratedFile, error) {
	hookPath := modulePath + "/" + directory

	hooks, err := RenderHooks(version, runtimeVersion, code, plan, hookPath)
	if err != nil {
		return nil, err
	}

	rules, _, err := RenderRules(version, code, plan, hookPath)
	if err != nil {
		return nil, err
	}

	return []GeneratedFile{
		{Path: directory + "/hooks.go", Data: hooks},
		{Path: "rules/" + ruleName + ".otelc.yaml", Data: rules},
	}, nil
}
