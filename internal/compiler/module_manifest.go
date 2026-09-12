package compiler

import (
	"fmt"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// RelocateModuleManifest rewrites local replacement paths for a copied module.
// Every local replacement must have a verified copy in relocations.
func RelocateModuleManifest(data []byte, originalDir string, relocations map[string]string) ([]byte, error) {
	if !filepath.IsAbs(originalDir) {
		return nil, fmt.Errorf("original module directory must be absolute")
	}
	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse module manifest: %w", err)
	}
	for _, replacement := range file.Replace {
		if replacement.New.Version != "" {
			continue
		}
		original := replacement.New.Path
		if !filepath.IsAbs(original) {
			original = filepath.Join(originalDir, original)
		}
		copied, ok := relocations[filepath.Clean(original)]
		if !ok || !filepath.IsAbs(copied) {
			return nil, fmt.Errorf("local module replacement has no isolated copy")
		}
		if err := file.AddReplace(replacement.Old.Path, replacement.Old.Version, filepath.Clean(copied), ""); err != nil {
			return nil, fmt.Errorf("relocate module replacement: %w", err)
		}
	}
	file.Cleanup()
	result, err := file.Format()
	if err != nil {
		return nil, fmt.Errorf("format isolated module manifest: %w", err)
	}
	return result, nil
}
