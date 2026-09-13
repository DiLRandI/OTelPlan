package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestWorkspaceModuleSelection(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"app/go.mod":     "module example.com/app\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\nreplace example.com/dep => ../dep\n",
		"runtime/go.mod": "module example.com/runtime\n\ngo 1.25.0\nrequire example.com/dep v1.1.0\n",
		"dep/go.mod":     "module example.com/dep\n\ngo 1.25.0\n",
		"go.work":        "go 1.25.0\nuse (\n./app\n./runtime\n)\n",
	}
	for path, data := range files {
		filename := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(workspace string) []model.ModuleInfo {
		modules, err := ReadModuleSelection(t.Context(), filepath.Join(root, "app"), append(os.Environ(), "GOWORK="+workspace, "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off"))
		if err != nil {
			t.Fatal(err)
		}
		return modules
	}
	original := read("off")
	changed := read(filepath.Join(root, "go.work"))
	if err := CheckModuleSelection(original, changed, nil); err == nil {
		t.Fatal("accepted workspace-selected dependency upgrade")
	}
	runtimeMod := "module example.com/runtime\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\n"
	if err := os.WriteFile(filepath.Join(root, "runtime", "go.mod"), []byte(runtimeMod), 0600); err != nil {
		t.Fatal(err)
	}
	compatible := read(filepath.Join(root, "go.work"))
	if err := CheckModuleSelection(original, compatible, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"app/go.mod", "dep/go.mod", "go.work"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || string(data) != files[path] {
			t.Fatalf("Go module inspection modified %s", path)
		}
	}
}
