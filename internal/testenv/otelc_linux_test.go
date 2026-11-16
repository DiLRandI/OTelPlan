package testenv_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/testenv"
)

func TestPathWithOTelCSelectsRequestedExecutable(t *testing.T) {
	tools := t.TempDir()

	directory, err := os.OpenRoot(tools)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	}()

	for name, contents := range map[string]string{
		"requested backend": "#!/bin/sh\nprintf 'requested\\n'\n",
		"otelc":             "#!/bin/sh\nprintf 'wrong sibling\\n'\n",
	} {
		err := directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}

		err = directory.Chmod(name, 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("OTELPLAN_OTELC", filepath.Join(tools, "requested backend"))
	t.Setenv("PATH", testenv.PathWithOTelC(t))

	command := exec.CommandContext(t.Context(), "otelc", "version")

	output, err := command.CombinedOutput()
	if err != nil || string(output) != "requested\n" {
		t.Fatalf("PATH selected another backend: error=%v output=%q", err, output)
	}
}
