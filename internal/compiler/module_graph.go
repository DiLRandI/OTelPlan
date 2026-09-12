package compiler

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// CheckModuleSelection preserves existing module selections when generated
// runtime dependencies are added. Relocations map original local directories
// to their isolated copies; new dependencies are checked against runtime pins
// by calling this function again with the runtime's original selections.
func CheckModuleSelection(original, selected []model.ModuleInfo, relocations map[string]string) error {
	before, err := indexModules(original)
	if err != nil {
		return err
	}
	after, err := indexModules(selected)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(before))
	for path := range before {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		wanted := before[path]
		actual, ok := after[path]
		if !ok {
			return fmt.Errorf("build module selection removed %s", path)
		}
		if wanted.Version != actual.Version || wanted.Main != actual.Main {
			return fmt.Errorf("build module selection changed %s", path)
		}
		if (wanted.Replace == nil) != (actual.Replace == nil) {
			return fmt.Errorf("build module replacement changed %s", path)
		}
		if wanted.Replace != nil {
			a, b := wanted.Replace, actual.Replace
			if a.Version != b.Version {
				return fmt.Errorf("build module replacement changed %s", path)
			}
			if a.Version != "" {
				if a.Path != b.Path {
					return fmt.Errorf("build module replacement changed %s", path)
				}
			} else if !sameModuleDirectory(a.Dir, b.Dir, relocations) {
				return fmt.Errorf("build local module replacement changed %s", path)
			}
		} else if wanted.Main && !sameModuleDirectory(wanted.Dir, actual.Dir, relocations) {
			return fmt.Errorf("build main module directory changed %s", path)
		}
	}
	return nil
}

func indexModules(modules []model.ModuleInfo) (map[string]model.ModuleInfo, error) {
	result := make(map[string]model.ModuleInfo, len(modules))
	for _, module := range modules {
		if module.Path == "" {
			return nil, fmt.Errorf("module selection has an empty identity")
		}
		if _, exists := result[module.Path]; exists {
			return nil, fmt.Errorf("module selection has duplicate identities")
		}
		result[module.Path] = module
	}
	return result, nil
}

func sameModuleDirectory(original, selected string, relocations map[string]string) bool {
	if !filepath.IsAbs(original) || !filepath.IsAbs(selected) {
		return false
	}
	original = filepath.Clean(original)
	if relocated, ok := relocations[original]; ok {
		if !filepath.IsAbs(relocated) {
			return false
		}
		original = filepath.Clean(relocated)
	}
	return original == filepath.Clean(selected)
}
