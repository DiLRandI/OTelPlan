package compiler_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestWorkspaceModuleSelection(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	files := map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.25.0\n" +
			"require example.com/dep v1.0.0\nreplace example.com/dep => ../dep\n",
		"runtime/go.mod": "module example.com/runtime\n\ngo 1.25.0\nrequire example.com/dep v1.1.0\n",
		"dep/go.mod":     "module example.com/dep\n\ngo 1.25.0\n",
		"go.work":        "go 1.25.0\nuse (\n./app\n./runtime\n)\n",
	}
	writeRelocationFixture(t, root, files)

	read := func(workspace string) []model.ModuleInfo {
		env := append(os.Environ(), "GOWORK="+workspace, "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off")

		modules, err := compiler.ReadModuleSelection(t.Context(), filepath.Join(root.Name(), "app"), env)
		if err != nil {
			t.Fatal(err)
		}

		return modules
	}
	original := read("off")

	changed := read(filepath.Join(root.Name(), "go.work"))

	err = compiler.CheckModuleSelection(original, changed, nil)
	if err == nil {
		t.Fatal("accepted workspace-selected dependency upgrade")
	}

	runtimeMod := "module example.com/runtime\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\n"

	err = root.WriteFile("runtime/go.mod", []byte(runtimeMod), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	compatible := read(filepath.Join(root.Name(), "go.work"))

	err = compiler.CheckModuleSelection(original, compatible, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"app/go.mod", "dep/go.mod", "go.work"} {
		data, err := root.ReadFile(path)
		if err != nil || string(data) != files[path] {
			t.Fatalf("Go module inspection modified %s", path)
		}
	}
}
