package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestScanRedactsFreeFormBuildEnvironment(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	for _, variable := range []string{
		"CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_LDFLAGS", "CGO_FFLAGS", "CC", "CXX",
	} {
		t.Setenv(variable, "otelplan-private-env-marker")
	}

	root, _, _ := callGraphFixture(t)

	for _, format := range []string{"text", "json"} {
		for _, verbose := range []bool{false, true} {
			var stdout, stderr bytes.Buffer

			args := []string{"scan", "--root", root, "--offline", "--format=" + format}
			if verbose {
				args = append(args, "--verbose")
			}

			if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 {
				t.Fatalf("scan exit=%d: %s %s", exit, &stdout, &stderr)
			}

			if strings.Contains(stdout.String()+stderr.String(), "otelplan-private-env-marker") {
				t.Fatalf("scan exposed a free-form environment value in %s mode", format)
			}

			if format == "json" {
				checkRedactedInventory(t, stdout.Bytes())
			}
		}
	}
}

func checkRedactedInventory(t *testing.T, output []byte) {
	t.Helper()

	var reply struct {
		Data model.CodeModel `json:"data"`
	}

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatal(err)
	}

	build := reply.Data.EffectiveBuild
	for _, value := range []string{
		build.CGOCFLAGS, build.CGOCPPFLAGS, build.CGOCXXFLAGS, build.CGOLDFLAGS,
		build.CGOFFLAGS, build.CC, build.CXX,
	} {
		if value != "[redacted]" {
			t.Fatal("free-form build value was not marked as redacted")
		}
	}

	if len(reply.Data.Symbols) == 0 || build.GOOS == "" || build.GOARCH == "" || build.GoVersion == "" {
		t.Fatal("redaction removed discovery or build target metadata")
	}
}
