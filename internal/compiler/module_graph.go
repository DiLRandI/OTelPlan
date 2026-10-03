package compiler

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var (
	errModuleRemoved              = errors.New("build module selection removed")
	errModuleChanged              = errors.New("build module selection changed")
	errModuleReplacementChanged   = errors.New("build module replacement changed")
	errLocalReplacementChanged    = errors.New("build local module replacement changed")
	errMainModuleDirectoryChanged = errors.New("build main module directory changed")
	errEmptyModuleIdentity        = errors.New("module selection has an empty identity")
	errDuplicateModuleIdentity    = errors.New("module selection has duplicate identities")
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
			return fmt.Errorf("%w %s", errModuleRemoved, path)
		}

		err = checkSelectedModule(wanted, actual, relocations)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkSelectedModule(wanted, actual model.ModuleInfo, relocations map[string]string) error {
	if wanted.Version != actual.Version || wanted.Main != actual.Main {
		return fmt.Errorf("%w %s", errModuleChanged, wanted.Path)
	}

	if (wanted.Replace == nil) != (actual.Replace == nil) {
		return fmt.Errorf("%w %s", errModuleReplacementChanged, wanted.Path)
	}

	if wanted.Replace != nil {
		return checkSelectedReplacement(wanted.Path, wanted.Replace, actual.Replace, relocations)
	}

	if wanted.Main && !sameModuleDirectory(wanted.Dir, actual.Dir, relocations) {
		return fmt.Errorf("%w %s", errMainModuleDirectoryChanged, wanted.Path)
	}

	return nil
}

func checkSelectedReplacement(path string, wanted, actual *model.ModuleReplacement,
	relocations map[string]string) error {
	if wanted.Version != actual.Version {
		return fmt.Errorf("%w %s", errModuleReplacementChanged, path)
	}

	if wanted.Version != "" {
		if wanted.Path != actual.Path {
			return fmt.Errorf("%w %s", errModuleReplacementChanged, path)
		}

		return nil
	}

	if !sameModuleDirectory(wanted.Dir, actual.Dir, relocations) {
		return fmt.Errorf("%w %s", errLocalReplacementChanged, path)
	}

	return nil
}

func indexModules(modules []model.ModuleInfo) (map[string]model.ModuleInfo, error) {
	result := make(map[string]model.ModuleInfo, len(modules))

	for _, module := range modules {
		if module.Path == "" {
			return nil, errEmptyModuleIdentity
		}

		if _, exists := result[module.Path]; exists {
			return nil, errDuplicateModuleIdentity
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
