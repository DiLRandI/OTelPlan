package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestApplicationSelectionExcludesRuntimeUpgrade(t *testing.T) {
	root := t.TempDir()
	app, generated := filepath.Join(root, "app"), filepath.Join(root, "runtime")

	files := map[string]string{"app/go.mod": "module example.com/app\n\ngo 1.27.0\nrequire example.com/dep v1.0.0\nreplace example.com/dep => ../dep\n", "runtime/go.mod": "module example.com/runtime\n\ngo 1.27.0\nrequire example.com/dep v1.1.0\n", "dep/go.mod": "module example.com/dep\n\ngo 1.27.0\n", "go.work": fmt.Sprintf("go 1.27.0\nuse (\n%q\n%q\n)\n", app, generated)}
	for path, data := range files {
		filename := filepath.Join(root, path)
		err := os.MkdirAll(filepath.Dir(filename), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = os.WriteFile(filename, []byte(data), 0o600)

		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := PreparedWorkspace{Dir: root, WorkspaceFile: filepath.Join(root, "go.work"), Runtime: model.Artifacts{Dir: generated}}

	env := append(os.Environ(), "GOFLAGS=", "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off")

	baseline, err := applicationModuleSelection(t.Context(), workspace, app, env)
	if err != nil {
		t.Fatal(err)
	}

	selected, err := ReadModuleSelection(t.Context(), app, append(env, "GOWORK="+workspace.WorkspaceFile))
	if err != nil {
		t.Fatal(err)
	}

	if err := CheckModuleSelection(baseline, selected, nil); err == nil {
		t.Fatal("runtime silently upgraded an application dependency")
	}

	found := false

	for _, module := range baseline {
		if module.Path == "example.com/dep" && module.Version == "v1.0.0" {
			found = true
		}
	}

	if !found {
		t.Fatal("baseline omitted application dependency")
	}

	for path, want := range files {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(data) != want {
			t.Fatal("selection changed workspace files")
		}
	}

	leftovers, err := filepath.Glob(filepath.Join(root, "application-*.work*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatal("selection left temporary workspace")
	}
}
