package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestUnsupportedBackendTargetsFailValidation(t *testing.T) {
	for _, tc := range []struct{ name, source, message string }{
		{"main", "package main\nimport \"context\"\nfunc Run(context.Context) error { return nil }\nfunc main(){}\n", "main package targets"},
		{"variadic", "package app\nimport \"context\"\nfunc Run(context.Context, ...string) error { return nil }\n", "variadic targets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, original := cliFixture(t)
			original["app.go"] = tc.source
			if err := os.WriteFile(filepath.Join(root, "app.go"), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"validate", "inspect", "compile", "build"} {
				for _, format := range []string{"text", "json"} {
					var out, errout bytes.Buffer
					if exit := Run([]string{command, "--root", root, "--offline", "--format=" + format}, &out, &errout); exit != 7 {
						t.Fatalf("%s %s exit=%d: %s %s", command, format, exit, &out, &errout)
					}
					if !strings.Contains(out.String(), tc.message) || !strings.Contains(out.String(), string(model.CodeBackendUnsupported)) {
						t.Fatalf("missing compatibility diagnostic: %s", &out)
					}
					if format == "json" {
						var reply response
						if err := json.Unmarshal(out.Bytes(), &reply); err != nil || reply.OK || len(reply.Diagnostics) != 1 {
							t.Fatalf("invalid response: %s", &out)
						}
						diagnostic := reply.Diagnostics[0]
						if diagnostic.Symbol != "example.com/app.Run" || diagnostic.RuleID != "operation" {
							t.Fatalf("missing target provenance: %+v", diagnostic)
						}
					}
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != len(original) {
				t.Fatal("validation failure wrote project files")
			}
			for name, want := range original {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || string(data) != want {
					t.Fatalf("changed project file %s", name)
				}
			}
		})
	}
}
