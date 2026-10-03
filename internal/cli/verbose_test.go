package cli_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
)

func TestVerbosePolicyFailurePreservesDiagnostic(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"text", "json"} {
		var ordinary, verbose, stderr bytes.Buffer

		args := []string{"inspect", "--root", t.TempDir(), "--format=" + format}
		if exit := cli.Run(t.Context(), args, &ordinary, &stderr); exit != 3 {
			t.Fatalf("ordinary policy failure exit=%d: %s", exit, &ordinary)
		}

		if exit := cli.Run(t.Context(), slices.Concat(args, []string{"--verbose"}), &verbose, &stderr); exit != 3 {
			t.Fatalf("verbose policy failure exit=%d: %s", exit, &verbose)
		}

		if format == "text" {
			if !strings.HasPrefix(verbose.String(), ordinary.String()) ||
				!strings.Contains(verbose.String(), "load policy") ||
				!strings.Contains(verbose.String(), "operation failed") {
				t.Fatalf("verbose failure omitted stable diagnostic or safe causes: %s", &verbose)
			}
		} else {
			checkVerboseJSON(t, ordinary.Bytes(), verbose.Bytes())
		}
	}
}

func checkVerboseJSON(t *testing.T, ordinary, verbose []byte) {
	t.Helper()

	var normalReply, verboseReply map[string]json.RawMessage

	err := json.Unmarshal(ordinary, &normalReply)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(verbose, &verboseReply)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(normalReply["diagnostics"], verboseReply["diagnostics"]) ||
		normalReply["details"] != nil || !bytes.Contains(verboseReply["details"], []byte("load policy")) {
		t.Fatalf("verbose JSON changed diagnostics or omitted details: %s", verbose)
	}
}

func TestVerboseScanIncludesBuildContext(t *testing.T) {
	t.Parallel()

	root, _, _ := callGraphFixture(t)

	var stdout, stderr bytes.Buffer

	args := []string{"scan", "--root", root, "--offline", "--verbose", "--format=json"}
	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 {
		t.Fatalf("verbose scan exit=%d: %s %s", exit, &stdout, &stderr)
	}

	var reply struct {
		Details struct {
			Build struct {
				GOOS       string `json:"goos"`
				GOARCH     string `json:"goarch"`
				ModuleMode string `json:"moduleMode"`
			} `json:"build"`
		} `json:"details"`
	}

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil || reply.Details.Build.GOOS == "" || reply.Details.Build.GOARCH == "" ||
		reply.Details.Build.ModuleMode == "" {
		t.Fatalf("missing verbose build context: %s (%v)", &stdout, err)
	}
}
