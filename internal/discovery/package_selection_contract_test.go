package discovery_test

import (
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPackageLoadingSelectionAndTestVariants(t *testing.T) {
	t.Parallel()

	for _, dependencies := range []bool{false, true} {
		t.Run(stringDependencyOption(dependencies), func(t *testing.T) {
			t.Parallel()

			root, directory, files := buildContextFixture(t)
			files["go.mod"] += "\nrequire example.com/dependency v0.0.0\nreplace example.com/dependency => ./dependency\n"
			files["fallback.go"] = "package buildcontext\nimport _ \"example.com/dependency\"\nfunc Fallback() {}\n"
			files["fallback_test.go"] = "package buildcontext\nfunc testOnly() {}\n"
			files["dependency/go.mod"] = "module example.com/dependency\n\ngo 1.27.0\n"
			files["dependency/dependency.go"] = "package dependency\nfunc Dependency() {}\n"

			err := directory.Mkdir("dependency", 0o700)
			if err != nil {
				t.Fatal(err)
			}

			for name, contents := range files {
				err := directory.WriteFile(name, []byte(contents), 0o600)
				if err != nil {
					t.Fatal(err)
				}
			}

			var options discovery.Options

			options.Root, options.Offline, options.IncludeTests = root, true, true
			options.IncludeDependencies = dependencies
			options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}
			code := loadBuildContext(t, options)
			checkSelectedPackages(t, code, dependencies)
			checkBuildContextFiles(t, directory, files)
		})
	}
}

func stringDependencyOption(enabled bool) string {
	if enabled {
		return "include dependencies"
	}

	return "application only"
}

func checkSelectedPackages(t *testing.T, code *model.CodeModel, dependencies bool) {
	t.Helper()

	for _, symbol := range []model.SymbolID{"example.com/buildcontext.Fallback", "example.com/buildcontext.testOnly"} {
		if _, found := code.Symbol(symbol); !found {
			t.Fatalf("application/test-variant symbol omitted: %s", symbol)
		}
	}

	_, dependencySelected := code.Symbol("example.com/dependency.Dependency")
	if dependencySelected != dependencies {
		t.Fatalf("dependency selected=%t; want %t", dependencySelected, dependencies)
	}

	seen := make(map[model.SymbolID]bool, len(code.Symbols))
	for _, symbol := range code.Symbols {
		if seen[symbol.ID] {
			t.Fatalf("test variants duplicated symbol %s", symbol.ID)
		}

		seen[symbol.ID] = true
	}

	packages := make([]string, 0, len(code.Packages))
	for _, pkg := range code.Packages {
		packages = append(packages, pkg.ImportPath)
	}

	if !slices.IsSorted(packages) {
		t.Fatalf("selected package order changed: %v", packages)
	}
}
