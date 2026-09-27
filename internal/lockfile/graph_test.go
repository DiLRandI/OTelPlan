package lockfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func graphFixture(t *testing.T) *model.CodeModel {
	t.Helper()
	root := t.TempDir()

	app, dependency := filepath.Join(root, "app"), filepath.Join(root, "dependency")
	for _, dir := range []string{app, dependency} {
		err := os.MkdirAll(dir, 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		filepath.Join(app, "go.mod"): "module example.com/app\n\ngo 1.27\n\n" +
			"require example.com/dependency v0.0.0\nreplace example.com/dependency => " + filepath.ToSlash(dependency) + "\n",
		filepath.Join(dependency, "go.mod"): "module example.com/dependency\n\ngo 1.27\n",
		filepath.Join(root, "go.work"):      "go 1.27\n\nuse " + filepath.ToSlash(app) + "\n",
	}
	for name, contents := range files {
		err := os.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var effectiveBuild model.BuildEnvironment

	return &model.CodeModel{
		GoVersion: "", ModuleRoot: root, WorkspaceFile: filepath.Join(root, "go.work"),
		GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"production"},
		Packages: nil, Symbols: nil, Types: nil, Implements: nil, InterfaceMethods: nil,
		CallGraph: nil, CallEdges: nil, EffectiveBuild: effectiveBuild,
		Modules: []model.ModuleInfo{
			{Path: "example.com/app", Main: true, Dir: app, Version: "", Ownership: "", Replace: nil},
			{Path: "example.com/dependency", Version: "v0.0.0", Dir: dependency, Main: false, Ownership: "",
				Replace: &model.ModuleReplacement{Path: dependency, Dir: dependency, Version: ""}},
		},
	}
}

func TestGraphDigestRelocationAndImmutability(t *testing.T) {
	t.Parallel()

	first, second := graphFixture(t), graphFixture(t)

	original, err := os.ReadFile(filepath.Join(first.Modules[0].Dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	firstDigest, err := lockfile.GraphDigest(first)
	if err != nil {
		t.Fatal(err)
	}

	secondDigest, err := lockfile.GraphDigest(second)
	if err != nil {
		t.Fatal(err)
	}

	if firstDigest != secondDigest {
		t.Fatal("absolute checkout/replacement/workspace paths affected digest")
	}

	after, err := os.ReadFile(filepath.Join(first.Modules[0].Dir, "go.mod"))
	if err != nil || string(after) != string(original) {
		t.Fatal("fingerprinting modified module file")
	}

	second.Modules[0], second.Modules[1] = second.Modules[1], second.Modules[0]

	reordered, err := lockfile.GraphDigest(second)
	if err != nil || reordered != firstDigest {
		t.Fatal("module order affected digest")
	}
}

func TestGraphDigestTracksBuildInputs(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		change func(*testing.T, *model.CodeModel)
	}{
		{"architecture", func(_ *testing.T, c *model.CodeModel) { c.GOARCH = "arm64" }},
		{"tags", func(_ *testing.T, c *model.CodeModel) { c.BuildTags = []string{"other"} }},
		{"dependency manifest", func(t *testing.T, c *model.CodeModel) {
			t.Helper()

			contents := "module example.com/dependency\n\ngo 1.27\nrequire example.com/transitive v1.0.0\n"

			err := os.WriteFile(filepath.Join(c.Modules[1].Dir, "go.mod"), []byte(contents), 0o600)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"checksums", func(t *testing.T, c *model.CodeModel) {
			t.Helper()

			contents := "example.com/external v1.0.0 h1:example\n"

			err := os.WriteFile(filepath.Join(c.Modules[0].Dir, "go.sum"), []byte(contents), 0o600)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"vendor", func(t *testing.T, c *model.CodeModel) {
			t.Helper()

			dir := filepath.Join(c.Modules[0].Dir, "vendor")

			err := os.Mkdir(dir, 0o700)
			if err != nil {
				t.Fatal(err)
			}

			err = os.WriteFile(filepath.Join(dir, "modules.txt"), []byte("# example.com/vendor v1.0.0\n"), 0o600)
			if err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code := graphFixture(t)

			before, err := lockfile.GraphDigest(code)
			if err != nil {
				t.Fatal(err)
			}

			testCase.change(t, code)

			after, err := lockfile.GraphDigest(code)
			if err != nil {
				t.Fatal(err)
			}

			if before == after {
				t.Fatal("build input change not detected")
			}
		})
	}
}

func TestGraphDigestMissingMetadata(t *testing.T) {
	t.Parallel()

	_, err := lockfile.GraphDigest(nil)
	if err == nil {
		t.Fatal("nil code model accepted")
	}

	code := graphFixture(t)

	code.Modules[0].Dir = filepath.Join(t.TempDir(), "missing")

	_, err = lockfile.GraphDigest(code)
	if err == nil {
		t.Fatal("missing module manifest accepted")
	}
}
