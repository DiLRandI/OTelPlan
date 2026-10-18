package otelc_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSymbolMetadataAccessorsCompileWithoutApplicationReads(t *testing.T) {
	t.Parallel()

	var symbol model.Symbol

	symbol.ID = "example.com/app.Run"
	symbol.Name, symbol.PackageName, symbol.PackageImportPath = "Run", "app", "example.com/app"
	symbol.Kind, symbol.HasBody, symbol.Signature = model.SymbolFunction, true, "func()"

	var code model.CodeModel

	code.Symbols = []model.Symbol{symbol}

	var target model.ResolvedTarget

	target.SymbolID, target.Signature = symbol.ID, symbol.Signature
	target.ContextStrategy.Strategy = model.ContextStrategyRoot

	want := map[string]string{
		"id": string(symbol.ID), "package": "example.com/app", "packageName": "app", "name": "Run",
		"receiver": "", "signature": "func()", "kind": "function",
	}
	for field := range want {
		var attribute model.AttributePlan

		attribute.Key, attribute.From.Symbol = field, field
		target.Attributes = append(target.Attributes, attribute)
	}

	before := append([]model.AttributePlan(nil), target.Attributes...)

	source, bindings, err := otelc.RenderAccessors(&code, target)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(before, target.Attributes) {
		t.Fatal("rendering symbol metadata mutated the caller's plan")
	}

	if len(bindings) != len(want) {
		t.Fatalf("metadata bindings=%d, want %d", len(bindings), len(want))
	}

	compileSymbolAccessors(t, source, bindings, want)
}

func compileSymbolAccessors(t *testing.T, source []byte, bindings []otelc.AccessorBinding, want map[string]string) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	var assertions bytes.Buffer

	assertions.WriteString("package app\nimport \"testing\"\nfunc TestMetadata(t *testing.T) {\n")

	for _, binding := range bindings {
		if binding.Source != "constant" || binding.Index != -1 || binding.Kind != "string" {
			t.Fatalf("symbol metadata requires a static scalar binding: %+v", binding)
		}

		fmt.Fprintf(&assertions, "if got, ok := %s(nil); !ok || got != %q {t.Fatal(%q, got, ok)}\n",
			binding.Function, want[binding.Key], binding.Key)
	}

	assertions.WriteString("}\n")

	for name, contents := range map[string][]byte{
		"go.mod":            []byte("module example.com/app\n\ngo 1.27\n"),
		"accessors.go":      []byte(strings.TrimPrefix(string(source), "//go:build ignore\n\n")),
		"accessors_test.go": assertions.Bytes(),
	} {
		err = root.WriteFile(name, contents, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	command := exec.CommandContext(t.Context(), "go", "test", "./...")
	command.Dir = root.Name()

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off",
		"GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("symbol accessors failed to compile or run: %v\n%s", err, output)
	}
}
