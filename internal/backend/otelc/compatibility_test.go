package otelc

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func compatibilityFixture() (*model.CodeModel, model.ResolvedPlan) {
	symbol := new(model.Symbol)
	symbol.ID = "example.com/app.Run"
	symbol.HasBody = true
	symbol.Signature = "func(context.Context) error"
	parameter := new(model.Parameter)
	parameter.Type = "context.Context"
	symbol.Parameters = []model.Parameter{*parameter}
	result := new(model.Result)
	result.Type = "error"
	symbol.Results = []model.Result{*result}
	symbol.ContextIndexes = []int{0}
	symbol.ErrorIndexes = []int{0}

	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{*symbol}
	target := new(model.ResolvedTarget)
	target.SymbolID = symbol.ID
	target.Signature = symbol.Signature
	target.RuleID = "run"
	target.ContextStrategy.Strategy = model.ContextStrategyArgument
	target.ErrorStrategy.Record = true
	target.ErrorStrategy.Indexes = []int{0}

	plan := new(model.ResolvedPlan)
	plan.Targets = []model.ResolvedTarget{*target}

	return code, *plan
}

func genericRootFixture() (*model.CodeModel, model.ResolvedPlan) {
	code, plan := ruleFixture()
	code.Symbols = code.Symbols[:1]
	plan.Targets = plan.Targets[:1]
	symbol := &code.Symbols[0]
	symbol.Signature = "func[T any](T, bool) (T, error)"
	symbol.Generics = new(model.GenericInfo)
	symbol.Generics.TypeParams = []string{"T"}
	valueParameter := new(model.Parameter)
	valueParameter.Name, valueParameter.Type = "value", "T"
	failParameter := new(model.Parameter)
	failParameter.Name, failParameter.Type = "fail", "bool"
	symbol.Parameters = []model.Parameter{*valueParameter, *failParameter}
	outputResult := new(model.Result)
	outputResult.Name, outputResult.Type = "output", "T"
	errorResult := new(model.Result)
	errorResult.Name, errorResult.Type = "err", "error"
	symbol.Results = []model.Result{*outputResult, *errorResult}
	symbol.ErrorIndexes = []int{1}
	symbol.Ownership = model.OwnershipApplication
	plan.Targets[0].Signature = symbol.Signature
	plan.Targets[0].SpanName = "generic.operation"
	plan.Targets[0].ErrorStrategy.Record = true
	plan.Targets[0].ErrorStrategy.Indexes = []int{1}

	return code, plan
}

func TestGenericRootCompatibility(t *testing.T) {
	t.Parallel()

	code, plan := genericRootFixture()
	if diagnostics := Check(SupportedVersion, code, plan); diagnostics.HasErrors() {
		t.Fatalf("generic root span with returned errors rejected: %+v", diagnostics)
	}

	attribute := new(model.AttributePlan)
	attribute.Key, attribute.From.Constant = "operation.kind", "generic"
	attribute.Classification = model.ClassificationPublic

	plan.Targets[0].Attributes = []model.AttributePlan{*attribute}
	if diagnostics := Check(SupportedVersion, code, plan); diagnostics.HasErrors() {
		t.Fatalf("generic constant attribute rejected: %+v", diagnostics)
	}

	attribute.From.Constant, attribute.From.Argument = nil, "value"

	plan.Targets[0].Attributes = []model.AttributePlan{*attribute}
	if diagnostics := Check(SupportedVersion, code, plan); !diagnostics.HasErrors() {
		t.Fatal("generic argument capture accepted despite disabled backend APIs")
	}

	attribute.From.Argument, attribute.From.Result = "", "output"

	plan.Targets[0].Attributes = []model.AttributePlan{*attribute}
	if diagnostics := Check(SupportedVersion, code, plan); !diagnostics.HasErrors() {
		t.Fatal("generic result capture accepted despite disabled backend APIs")
	}
}

func TestPinnedCapabilities(t *testing.T) {
	t.Parallel()

	identity, err := Identity(SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	if !identity.Capabilities.ContextReplacement || !identity.Capabilities.AfterHook ||
		identity.Capabilities.PanicObservation {
		t.Fatalf("capabilities: %+v %v", identity, err)
	}

	for _, version := range []string{"latest", "v1.0.0", "v1.1", "v1.2.0"} {
		_, identityErr := Identity(version)
		if identityErr == nil {
			t.Fatalf("unverified version %s accepted", version)
		}
	}

	code, plan := compatibilityFixture()
	if diags := Check(SupportedVersion, code, plan); diags.HasErrors() {
		t.Fatalf("compatible target rejected: %+v", diags)
	}
}

func TestUnsupportedIdentityErrorsRetainCause(t *testing.T) {
	t.Parallel()

	_, firstErr := Identity("latest")
	_, secondErr := Identity("latest")

	if firstErr == nil || secondErr == nil {
		t.Fatal("unsupported version returned no error")
	}

	if !errors.Is(secondErr, firstErr) {
		t.Fatalf("repeated error does not retain its cause: first %v, second %v", firstErr, secondErr)
	}
}

func TestCompatibilityDiagnosticsPreserveOrderAndContext(t *testing.T) {
	t.Parallel()

	code, plan := compatibilityFixture()
	symbol := &code.Symbols[0]
	symbol.PackageName = "main"
	symbol.Variadic = true
	variadicParameter := new(model.Parameter)
	variadicParameter.Type = "[]example.com/app.Item"
	symbol.Parameters = append(symbol.Parameters, *variadicParameter)
	symbol.HasBody = false
	symbol.Generics = new(model.GenericInfo)
	symbol.Generics.TypeParams = []string{"T"}
	plan.Targets[0].ContextStrategy.Index = 2
	plan.Targets[0].ErrorStrategy.Indexes = []int{2}

	diagnostics := Check(SupportedVersion, code, plan)
	expected := []struct {
		message string
		ruleID  string
		symbol  model.SymbolID
	}{
		{"main package targets require verified command-specific build scoping", "run", symbol.ID},
		{"variadic targets require a built-in element type that generated hooks can name safely", "run", symbol.ID},
		{"backend cannot hook a declaration without a Go body", "run", symbol.ID},
		{"otelc v1.1.0 cannot replace generic context arguments; see upstream issue 1280", "run", symbol.ID},
		{"context replacement requires the unique analyzed context argument", "run", symbol.ID},
		{"error strategy refers to an invalid error result", "run", symbol.ID},
	}

	if len(diagnostics) != len(expected) {
		t.Fatalf("got %d diagnostics, want %d: %+v", len(diagnostics), len(expected), diagnostics)
	}

	for index, diagnostic := range diagnostics {
		if diagnostic.Severity != model.SeverityError || diagnostic.Code != model.CodeBackendUnsupported ||
			diagnostic.Message != expected[index].message || diagnostic.RuleID != expected[index].ruleID ||
			diagnostic.Symbol != expected[index].symbol {
			t.Errorf("diagnostic %d = %+v, want message %q with rule %q and symbol %q",
				index, diagnostic, expected[index].message, expected[index].ruleID, expected[index].symbol)
		}
	}
}

func TestCompatibilitySignatureMismatchShortCircuitsOnlyTarget(t *testing.T) {
	t.Parallel()

	code, plan := compatibilityFixture()
	firstTarget := plan.Targets[0]
	firstTarget.Signature = "changed"
	firstTarget.ContextStrategy.Index = 2
	firstTarget.ErrorStrategy.Indexes = []int{2}
	secondTarget := firstTarget
	secondTarget.SymbolID = "example.com/app.Other"
	secondTarget.Signature = "func()"
	secondTarget.RuleID = "other"
	secondTarget.ContextStrategy.Strategy = model.ContextStrategyRoot
	secondTarget.ContextStrategy.Index = 0
	secondTarget.ErrorStrategy.Record = false
	secondTarget.ErrorStrategy.Indexes = nil
	otherSymbol := new(model.Symbol)
	otherSymbol.ID = secondTarget.SymbolID
	otherSymbol.PackageName = "main"
	otherSymbol.HasBody = true
	otherSymbol.Signature = secondTarget.Signature
	code.Symbols = append(code.Symbols, *otherSymbol)
	plan.Targets = []model.ResolvedTarget{firstTarget, secondTarget}

	diagnostics := Check(SupportedVersion, code, plan)
	if len(diagnostics) != 2 {
		t.Fatalf("got %d diagnostics, want mismatch and second-target validation: %+v", len(diagnostics), diagnostics)
	}

	if diagnostics[0].Message != "backend target must match the analyzed symbol and signature" ||
		diagnostics[0].RuleID != "run" || diagnostics[0].Symbol != firstTarget.SymbolID {
		t.Errorf("first diagnostic = %+v, want signature mismatch for first target", diagnostics[0])
	}

	if diagnostics[1].Message != "main package targets require verified command-specific build scoping" ||
		diagnostics[1].RuleID != "other" || diagnostics[1].Symbol != secondTarget.SymbolID {
		t.Errorf("second diagnostic = %+v, want main package diagnostic for second target", diagnostics[1])
	}
}

func TestCompatibilityRejectsUnknownVersionAndNilCodeEarly(t *testing.T) {
	t.Parallel()

	plan := new(model.ResolvedPlan)
	diagnostics := Check("latest", nil, *plan)

	if len(diagnostics) != 1 || diagnostics[0].Code != model.CodeBackendVersionMismatch {
		t.Fatalf("unknown version diagnostics = %+v", diagnostics)
	}

	diagnostics = Check(SupportedVersion, nil, *plan)
	if len(diagnostics) != 1 || diagnostics[0].Code != model.CodeBackendUnsupported ||
		diagnostics[0].Message != "backend requires an analyzed code model" {
		t.Fatalf("nil code diagnostics = %+v", diagnostics)
	}
}

func TestRejectUnsupportedTargets(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		change func(*model.CodeModel, *model.ResolvedPlan)
	}{
		{"generic", func(code *model.CodeModel, _ *model.ResolvedPlan) {
			code.Symbols[0].Generics = new(model.GenericInfo)
			code.Symbols[0].Generics.TypeParams = []string{"T"}
		}},
		{"main package", func(code *model.CodeModel, _ *model.ResolvedPlan) { code.Symbols[0].PackageName = "main" }},
		{"variadic", func(code *model.CodeModel, _ *model.ResolvedPlan) { code.Symbols[0].Variadic = true }},
		{"declaration", func(code *model.CodeModel, _ *model.ResolvedPlan) { code.Symbols[0].HasBody = false }},
		{"context", func(_ *model.CodeModel, plan *model.ResolvedPlan) { plan.Targets[0].ContextStrategy.Index = 2 }},
		{"error result", func(_ *model.CodeModel, plan *model.ResolvedPlan) {
			plan.Targets[0].ErrorStrategy.Indexes = []int{2}
		}},
		{"signature", func(_ *model.CodeModel, plan *model.ResolvedPlan) { plan.Targets[0].Signature = "changed" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, plan := compatibilityFixture()
			testCase.change(code, &plan)

			if !Check(SupportedVersion, code, plan).HasErrors() {
				t.Fatal("unsupported target accepted")
			}
		})
	}
}

func TestVariadicBuiltinTargetCompatibility(t *testing.T) {
	t.Parallel()

	for _, element := range []string{"int", "string", "bool", "error", "any", "interface{}"} {
		t.Run(element, func(t *testing.T) {
			t.Parallel()

			code, plan := compatibilityFixture()
			code.Symbols[0].Variadic = true
			variadicParameter := new(model.Parameter)
			variadicParameter.Name = "values"
			variadicParameter.Type = "[]" + element
			code.Symbols[0].Parameters = append(code.Symbols[0].Parameters, *variadicParameter)
			code.Symbols[0].Signature = "func(context.Context, ..." + element + ") error"
			plan.Targets[0].Signature = code.Symbols[0].Signature

			if diagnostics := Check(SupportedVersion, code, plan); diagnostics.HasErrors() {
				t.Fatalf("builtin variadic target rejected: %+v", diagnostics)
			}
		})
	}
}

func TestVariadicApplicationTypeRemainsRejected(t *testing.T) {
	t.Parallel()

	code, plan := compatibilityFixture()
	code.Symbols[0].Variadic = true
	variadicParameter := new(model.Parameter)
	variadicParameter.Name = "values"
	variadicParameter.Type = "[]example.com/app.Item"
	code.Symbols[0].Parameters = append(code.Symbols[0].Parameters, *variadicParameter)
	code.Symbols[0].Signature = "func(context.Context, ...Item) error"
	plan.Targets[0].Signature = code.Symbols[0].Signature

	if diagnostics := Check(SupportedVersion, code, plan); !diagnostics.HasErrors() {
		t.Fatal("application type variadic target accepted without package-local hook support")
	}
}

func TestMissingExecutable(t *testing.T) {
	t.Parallel()

	_, err := VerifyExecutable(t.Context(), "/missing/otelc", SupportedVersion)
	if err == nil {
		t.Fatal("missing executable accepted")
	}
}

func TestPinnedExecutableIdentity(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to the pinned backend executable")
	}

	identity, err := VerifyExecutable(t.Context(), executable, SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	if identity.Version != SupportedVersion || !strings.HasPrefix(identity.Digest, "sha256:") ||
		len(identity.Digest) != 71 {
		t.Fatalf("invalid executable identity: %+v", identity)
	}
}

func TestReportedExecutableVersionContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		message string
	}{
		{name: "pinned", output: "otelc version v1.1.0\n", message: ""},
		{name: "build metadata", output: "otelc version v1.1.0+custom-build\n", message: ""},
		{name: "trailing context", output: "otelc version v1.1.0 built with Go\n", message: ""},
		{name: "empty", output: "", message: "unrecognized otelc version output"},
		{name: "wrong label", output: "private-label version v1.1.0", message: "unrecognized otelc version output"},
		{name: "missing version", output: "otelc version", message: "unrecognized otelc version output"},
		{name: "different pin", output: "otelc version v1.2.0+private-build",
			message: "otelc executable does not match pinned version"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			first := validateReportedVersion([]byte(testCase.output), SupportedVersion)
			second := validateReportedVersion([]byte(testCase.output), SupportedVersion)

			if testCase.message == "" {
				if first != nil || second != nil {
					t.Fatalf("valid pinned version rejected: %v, %v", first, second)
				}

				return
			}

			if first == nil || first.Error() != testCase.message || !errors.Is(second, first) {
				t.Fatalf("version error changed or lost its cause: first=%v second=%v", first, second)
			}
		})
	}
}

func TestExecutableDigestPreservesSelectedSymlinkIdentity(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	selected := filepath.Join(directory, "otelc")
	contents := []byte("selected backend bytes for hashing")

	err := os.WriteFile(selected, contents, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	linked := filepath.Join(t.TempDir(), "selected-otelc")

	err = os.Symlink(selected, linked)
	if err != nil {
		t.Fatalf("create selected backend symlink: %v", err)
	}

	sha := sha256.Sum256(contents)
	want := fmt.Sprintf("sha256:%x", sha)

	for _, filename := range []string{selected, linked} {
		got, err := executableDigest(filename)
		if err != nil || got != want {
			t.Fatalf("selected digest=%s error=%v; want %s", got, err, want)
		}
	}
}
