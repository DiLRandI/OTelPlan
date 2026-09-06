package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func graphFixture(t *testing.T) *model.CodeModel {
	t.Helper()
	root := t.TempDir()
	app, dependency := filepath.Join(root, "app"), filepath.Join(root, "dependency")
	for _, dir := range []string{app, dependency} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(app, "go.mod"):        "module example.com/app\n\ngo 1.27\n\nrequire example.com/dependency v0.0.0\nreplace example.com/dependency => " + filepath.ToSlash(dependency) + "\n",
		filepath.Join(dependency, "go.mod"): "module example.com/dependency\n\ngo 1.27\n",
		filepath.Join(root, "go.work"):      "go 1.27\n\nuse " + filepath.ToSlash(app) + "\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(name, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return &model.CodeModel{ModuleRoot: root, WorkspaceFile: filepath.Join(root, "go.work"), GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"production"}, Modules: []model.ModuleInfo{
		{Path: "example.com/app", Main: true, Dir: app},
		{Path: "example.com/dependency", Version: "v0.0.0", Dir: dependency, Replace: &model.ModuleReplacement{Path: dependency, Dir: dependency}},
	}}
}

func TestGraphDigestRelocationAndImmutability(t *testing.T) {
	first, second := graphFixture(t), graphFixture(t)
	original, err := os.ReadFile(filepath.Join(first.Modules[0].Dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := GraphDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := GraphDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("absolute checkout/replacement/workspace paths affected digest")
	}
	after, err := os.ReadFile(filepath.Join(first.Modules[0].Dir, "go.mod"))
	if err != nil || string(after) != string(original) {
		t.Fatal("fingerprinting modified module file")
	}
	second.Modules[0], second.Modules[1] = second.Modules[1], second.Modules[0]
	reordered, err := GraphDigest(second)
	if err != nil || reordered != a {
		t.Fatal("module order affected digest")
	}
}

func TestGraphDigestTracksBuildInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *model.CodeModel)
	}{
		{"architecture", func(_ *testing.T, c *model.CodeModel) { c.GOARCH = "arm64" }},
		{"tags", func(_ *testing.T, c *model.CodeModel) { c.BuildTags = []string{"other"} }},
		{"dependency manifest", func(t *testing.T, c *model.CodeModel) {
			if err := os.WriteFile(filepath.Join(c.Modules[1].Dir, "go.mod"), []byte("module example.com/dependency\n\ngo 1.27\nrequire example.com/transitive v1.0.0\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"checksums", func(t *testing.T, c *model.CodeModel) {
			if err := os.WriteFile(filepath.Join(c.Modules[0].Dir, "go.sum"), []byte("example.com/external v1.0.0 h1:example\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"vendor", func(t *testing.T, c *model.CodeModel) {
			dir := filepath.Join(c.Modules[0].Dir, "vendor")
			if err := os.Mkdir(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "modules.txt"), []byte("# example.com/vendor v1.0.0\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := graphFixture(t)
			before, err := GraphDigest(code)
			if err != nil {
				t.Fatal(err)
			}
			tc.change(t, code)
			after, err := GraphDigest(code)
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
	if _, err := GraphDigest(nil); err == nil {
		t.Fatal("nil code model accepted")
	}
	code := graphFixture(t)
	code.Modules[0].Dir = filepath.Join(t.TempDir(), "missing")
	if _, err := GraphDigest(code); err == nil {
		t.Fatal("missing module manifest accepted")
	}
}
