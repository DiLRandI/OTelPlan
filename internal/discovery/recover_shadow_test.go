package discovery_test

import (
	"os"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRecoverShadowedDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("package declarations", func(t *testing.T) {
		t.Parallel()

		for _, declaration := range []struct {
			name   string
			source string
		}{
			{name: "function", source: "func recover() {}\n"},
			{name: "variable", source: "var recover = 1\n"},
			{name: "type", source: "type recover struct{}\n"},
			{name: "constant", source: "const recover = 1\n"},
		} {
			t.Run(declaration.name, func(t *testing.T) {
				t.Parallel()

				code := loadRecoverFixture(t, map[string]string{
					"target.go": "package recoverfixture\nfunc Target() {}\n",
					"shadow.go": "package recoverfixture\n" + declaration.source,
				})

				assertRecoverShadowed(t, code, "Target", true)
			})
		}
	})

	t.Run("file-local import alias", func(t *testing.T) {
		t.Parallel()

		code := loadRecoverFixture(t, map[string]string{
			"aliased.go": "package recoverfixture\nimport recover \"context\"\nfunc Aliased() { _ = recover.Background }\n",
			"plain.go":   "package recoverfixture\nfunc Plain() {}\n",
		})

		assertRecoverShadowed(t, code, "Aliased", true)
		assertRecoverShadowed(t, code, "Plain", false)
	})

	t.Run("local identifier and method", func(t *testing.T) {
		t.Parallel()

		code := loadRecoverFixture(t, map[string]string{
			"target.go": "package recoverfixture\n" +
				"type Worker struct{}\n" +
				"func Target() { recover := 1; _ = recover }\n" +
				"func (Worker) recover() {}\n" +
				"func (Worker) Method() {}\n",
		})

		assertRecoverShadowed(t, code, "Target", false)
		assertRecoverShadowed(t, code, "(Worker).recover", false)
		assertRecoverShadowed(t, code, "(Worker).Method", false)
	})

	t.Run("inactive build tag and test-only declaration", func(t *testing.T) {
		t.Parallel()

		code := loadRecoverFixture(t, map[string]string{
			"target.go":      "package recoverfixture\nfunc Target() {}\n",
			"tagged.go":      "//go:build recover_fixture\n\npackage recoverfixture\nvar recover = 1\n",
			"target_test.go": "package recoverfixture\nvar recover = 1\n",
		})

		assertRecoverShadowed(t, code, "Target", false)
	})
}

func TestRecoverShadowedFollowsBuildSelection(t *testing.T) {
	t.Parallel()

	for _, includeTests := range []bool{false, true} {
		name := "active build tag"
		files := map[string]string{
			"target.go": "package recoverfixture\nfunc Target() {}\n",
			"shadow.go": "//go:build recover_fixture\n\npackage recoverfixture\nvar recover = 1\n",
		}

		var options discovery.Options

		options.BuildTags = []string{"recover_fixture"}

		if includeTests {
			name = "included test variant"
			files["target_test.go"] = "package recoverfixture\nvar recover = 1\n"
			delete(files, "shadow.go")

			options.BuildTags, options.IncludeTests = nil, true
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			code := loadRecoverFixtureOptions(t, files, options)
			assertRecoverShadowed(t, code, "Target", true)
		})
	}
}

func assertRecoverShadowed(t *testing.T, code *model.CodeModel, name string, want bool) {
	t.Helper()

	symbolID := model.SymbolID("example.com/recoverfixture." + name)

	symbol, exists := code.Symbol(symbolID)
	if !exists {
		t.Fatalf("missing symbol %s", symbolID)
	}

	if symbol.RecoverShadowed != want {
		t.Fatalf("symbol %s RecoverShadowed=%t, want %t", symbolID, symbol.RecoverShadowed, want)
	}
}

func loadRecoverFixture(t *testing.T, files map[string]string) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	return loadRecoverFixtureOptions(t, files, options)
}

func loadRecoverFixtureOptions(t *testing.T, files map[string]string, options discovery.Options) *model.CodeModel {
	t.Helper()

	directory, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Errorf("close recover fixture: %v", err)
		}
	})

	err = directory.WriteFile("go.mod", []byte("module example.com/recoverfixture\n\ngo 1.27\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	for name, source := range files {
		err := directory.WriteFile(name, []byte(source), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	options.Root, options.Offline = directory.Name(), true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}
