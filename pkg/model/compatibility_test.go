package model_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPublicModelExternalModuleCompatibility(t *testing.T) {
	t.Parallel()

	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository: %v", err)
	}

	root := t.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open external module: %v", err)
	}

	t.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Errorf("close external module: %v", closeErr)
		}
	})

	goMod := "module example.com/model-compatibility\n\ngo 1.27\n\nrequire github.com/DiLRandI/OTelPlan v0.0.0\n\n" +
		"replace github.com/DiLRandI/OTelPlan v0.0.0 => " + strconv.Quote(filepath.ToSlash(repository)) + "\n"
	writeExternalFile(t, directory, "go.mod", goMod)
	writeExternalFile(t, directory, "client_test.go", externalClient)

	command := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-mod=readonly", "./...")
	command.Dir = root

	command.Env = append(os.Environ(),
		"GOWORK=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off", "GOENV=off", "GOTOOLCHAIN=local",
	)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("external compatibility module: %v: %s", err, output)
	}
}

func writeExternalFile(t *testing.T, directory *os.Root, name, contents string) {
	t.Helper()

	err := directory.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("write external module %s: %v", name, err)
	}
}

const externalClient = `package compatibility_test

import (
    "context"
    "encoding/json"
    "errors"
    "testing"

    "github.com/DiLRandI/OTelPlan/pkg/model"
)

type backend struct{}

var _ model.Backend = backend{}

func (backend) Name() string { return "compatibility" }
func (backend) Version(context.Context) (string, error) { return "v1", nil }
func (backend) Capabilities(context.Context) (model.BackendCapabilities, error) {
    return model.BackendCapabilities{}, nil
}
func (backend) Validate(context.Context, model.ResolvedPlan) model.DiagnosticErrorList { return nil }
func (backend) Compile(context.Context, model.ResolvedPlan, string) (model.Artifacts, error) {
    return model.Artifacts{}, nil
}
func (backend) Build(context.Context, model.BuildRequest) error { return nil }

func TestExternalPublicModel(t *testing.T) {
    function := model.FunctionID("example.com/orders", "Place")
    pointer := model.MethodID("example.com/orders", model.Receiver{Type: "Order", Pointer: true}, "Save")
    value := model.MethodID("example.com/orders", model.Receiver{Type: "Order"}, "Save")
    if function != "example.com/orders.Place" || pointer == value {
        t.Fatalf("unexpected symbol identities: %q %q %q", function, pointer, value)
    }

    parsed, err := model.ParseSymbolID(pointer)
    if err != nil || parsed.ImportPath != "example.com/orders" || parsed.Receiver == nil ||
        !parsed.Receiver.Pointer || parsed.Name != "Save" {
        t.Fatalf("parse pointer method: %+v %v", parsed, err)
    }
    _, first := model.ParseSymbolID("")
    _, second := model.ParseSymbolID("")
    if first == nil || second == nil || !errors.Is(first, second) {
        t.Fatalf("empty symbol errors are not stable: %v %v", first, second)
    }

    policy := model.Policy{
        Backend: model.BackendConfig{Name: model.BackendNameOTelC, Version: "v1.1.0"},
        Rules: []model.Rule{{ID: "place", Match: model.Match{Symbols: []string{string(function)}}}},
    }
    plan := model.ResolvedPlan{
        APIVersion: model.APIVersionV1Alpha1,
        Targets: []model.ResolvedTarget{{
            SymbolID: function,
            RuleID: "place",
            ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument},
        }},
    }
    lock := model.Lockfile{
        APIVersion: model.LockAPIVersionV1Alpha1,
        Backend: model.LockBackend{Name: policy.Backend.Name},
        Targets: []model.LockTarget{{Symbol: function, SourceRule: "place"}},
    }
    code := model.CodeModel{Symbols: []model.Symbol{{ID: function, Name: "Place"}}}
    if found, ok := code.Symbol(function); !ok || found.Name != "Place" ||
        policy.Rules[0].Match.IsEmpty() || plan.Targets[0].RuleID != lock.Targets[0].SourceRule {
        t.Fatal("public model composition did not preserve values")
    }

    diagnostics := model.DiagnosticErrorList{{
        Severity: model.SeverityError, Code: model.CodeMissingContext, Message: "missing context",
    }}
    encoded, err := json.Marshal(diagnostics[0])
    wantJSON := "{\"severity\":\"error\",\"code\":\"OTP3001\",\"message\":\"missing context\"}"
    if err != nil || string(encoded) != wantJSON || !diagnostics.HasErrors() {
        t.Fatalf("diagnostic compatibility failed: %s %v", encoded, err)
    }
    emptyDiff := model.LockDiff{}
    if !emptyDiff.Empty() {
        t.Fatal("empty lock diff reports changes")
    }
}
`
