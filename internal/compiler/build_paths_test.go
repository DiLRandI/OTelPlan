package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRelocateBuildArguments(t *testing.T) {
	source, parent := t.TempDir(), t.TempDir()
	app, other := filepath.Join(source, "app"), filepath.Join(source, "other")

	copiedApp, copiedOther := filepath.Join(parent, "app"), filepath.Join(parent, "other")
	for _, dir := range []string{copiedApp, copiedOther, filepath.Join(copiedApp, "cmd")} {
		err := os.MkdirAll(dir, 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := PreparedWorkspace{Dir: parent, Relocations: map[string]string{app: copiedApp, other: copiedOther}}
	args := []string{"-tags", "a,b", "-p=2", ".", "./cmd/...", "../other", filepath.Join(app, "main.go"), "helper.go", "example.com/app/...", "./cmd/.../nested"}
	before := append([]string(nil), args...)
	want := []string{"-tags", "a,b", "-p=2", copiedApp, filepath.Join(copiedApp, "cmd", "..."), copiedOther, filepath.Join(copiedApp, "main.go"), filepath.Join(copiedApp, "helper.go"), "example.com/app/...", filepath.Join(copiedApp, "cmd", "...", "nested")}

	got, err := workspace.RelocateBuildArguments(args, app)

	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("relocated arguments = %v, %v; want %v", got, err, want)
	}

	if !reflect.DeepEqual(args, before) {
		t.Fatal("caller arguments changed")
	}

	for _, arg := range []string{"../../outside", filepath.Join(source, "unprepared"), "../missing/..."} {
		if _, err := workspace.RelocateBuildArguments([]string{arg}, app); err == nil {
			t.Fatal("accepted unprepared package path")
		}
	}
}
