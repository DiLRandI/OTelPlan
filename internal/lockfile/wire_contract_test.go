package lockfile_test

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/internal/model"
)

func TestLockfileWireContracts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"complete", "resolution"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input := lockWireFixture()
			if name == "resolution" {
				input.Backend.Digest, input.Artifacts = "", nil
				input.Targets = input.Targets[1:]
			}

			goldens := openWireRoot(t, "testdata/wire-contracts")

			want, err := goldens.ReadFile(name + ".json")
			if err != nil {
				t.Fatal(err)
			}

			got, err := lockfile.Marshal(input)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("canonical lockfile wire contract changed: %v\ngot:\n%s\nwant:\n%s", err, got, want)
			}

			parsed, err := lockfile.Parse(want)
			if err != nil {
				t.Fatal("existing wire contract is no longer accepted", err)
			}

			again, err := lockfile.Marshal(parsed)
			if err != nil || !bytes.Equal(again, want) {
				t.Fatalf("parsed lockfile wire contract changed: %v\ngot:\n%s\nwant:\n%s", err, again, want)
			}

			checkWrittenWireContract(t, input, want)
		})
	}
}

func checkWrittenWireContract(t *testing.T, input model.Lockfile, want []byte) {
	t.Helper()

	root := openWireRoot(t, t.TempDir())

	err := lockfile.Write(filepath.Join(root.Name(), "otelplan.lock"), input)
	if err != nil {
		t.Fatal(err)
	}

	got, err := root.ReadFile("otelplan.lock")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("persisted lockfile wire contract changed: %v\ngot:\n%s", err, got)
	}
}

func openWireRoot(t *testing.T, path string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	return root
}

func lockWireFixture() model.Lockfile {
	var locked model.Lockfile

	locked.APIVersion = model.LockAPIVersionV1Alpha1
	locked.PolicyDigest = "sha256:" + strings.Repeat("a", 64)
	locked.GoVersion = "go1.27.0"
	locked.ModuleGraphDigest = "sha256:" + strings.Repeat("b", 64)
	locked.Backend = model.LockBackend{
		Name: "otelc", Version: "v1.1.0", Digest: "sha256:" + strings.Repeat("c", 64),
		Capabilities: model.BackendCapabilities{
			BeforeHook: true, AfterHook: true, ArgumentRead: true, ArgumentReplace: true, ResultRead: true,
			PanicObservation: false, ContextReplacement: true, FunctionEntrySelection: true, FunctionCallSelection: false,
		},
	}
	runSignature := "func(request example.com/wire.Category, ctx context.Context) (ok bool, err error)"
	locked.Targets = []model.LockTarget{
		{
			Symbol: "example.com/wire.Run", Signature: runSignature, SignatureDigest: lockfile.Digest([]byte(runSignature)),
			SourceRule: "work", SpanName: "wire.Run", Attributes: lockWireAttributes(),
			Context:  model.LockContext{Strategy: model.ContextStrategyArgument, Index: 1},
			Errors:   model.LockErrors{Record: true, Indexes: []int{1}},
			Location: model.SourceLocation{File: "ops/work.go", Line: 17, Column: 1},
		},
		{
			Symbol: "example.com/wire.Another", Signature: "func()", SignatureDigest: lockfile.Digest([]byte("func()")),
			SourceRule: "other", SpanName: "wire.Another", Attributes: nil,
			Context:  model.LockContext{Strategy: model.ContextStrategyRoot, Index: 0},
			Errors:   model.LockErrors{Record: false, Indexes: nil},
			Location: model.SourceLocation{File: "other.go", Line: 3, Column: 0},
		},
	}
	locked.Artifacts = []model.ArtifactFile{
		{Path: "rules/hooks.otelc.yaml", Digest: "sha256:" + strings.Repeat("d", 64)},
		{Path: "hooks/hooks.go", Digest: "sha256:" + strings.Repeat("e", 64)},
	}

	return locked
}

func lockWireAttributes() []model.LockAttribute {
	var argument, result, constant, symbol, errorType model.AttributeSource

	argument.Argument, result.Result, constant.Constant = "request", "ok", int64(math.MaxInt64)
	symbol.Symbol, errorType.ErrorType = model.SymbolFieldID, "err"
	value := "example.com/wire.Run"

	return []model.LockAttribute{
		{Key: "output.ok", From: result, Kind: "bool", Stringer: false,
			Classification: model.ClassificationPublic, Allow: false, SymbolValue: nil},
		{Key: "input.text", From: argument, Kind: "string", Stringer: true,
			Classification: "", Allow: false, SymbolValue: nil},
		{Key: "failure.class", From: errorType, Kind: "string", Stringer: false,
			Classification: "", Allow: false, SymbolValue: nil},
		{Key: "deployment.code", From: constant, Kind: "integer", Stringer: false,
			Classification: model.ClassificationInternal, Allow: true, SymbolValue: nil},
		{Key: "code.operation", From: symbol, Kind: "string", Stringer: false,
			Classification: model.ClassificationInternal, Allow: true, SymbolValue: &value},
	}
}
