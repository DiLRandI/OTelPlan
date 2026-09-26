package validate_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func attributeFixture(t *testing.T) (*model.CodeModel, model.Symbol) {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{"go.mod": "module example.com/app\n\ngo 1.27\n", "app.go": `package app
type Category string
type Details struct { Kind Category; password string }
type Left struct { ID string }
type Right struct { ID string }
type Cycle struct { *Cycle; Value string }
type Request struct { *Details; Left; Right; *Cycle; Count int; Payload []byte }
func Run(request *Request) (result bool) { return true }
`}

	for name, contents := range files {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	code, err := discovery.Load(discovery.Options{
		Root: root, Patterns: nil, BuildTags: nil, BuildFlags: nil, CallGraph: false,
		IncludeTests: false, IncludeDependencies: false, GOOS: "", GOARCH: "", Env: nil, Offline: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	symbol, ok := code.Symbol("example.com/app.Run")
	if !ok {
		t.Fatal("missing fixture symbol")
	}

	return code, *symbol
}

func TestAttributeAccessors(t *testing.T) {
	t.Parallel()

	code, symbol := attributeFixture(t)

	source := model.AttributeSource{Argument: "request.Kind", Result: "", Constant: nil}

	access, err := validate.AttributeAccessor(code, symbol, source)
	if err != nil || access.Kind != "string" || !reflect.DeepEqual(access.Fields, []string{"Details", "Kind"}) {
		t.Fatalf("promoted scalar: %+v %v", access, err)
	}

	for _, source := range []model.AttributeSource{
		{Argument: "request.Count", Result: "", Constant: nil},
		{Argument: "0.Count", Result: "", Constant: nil},
		{Argument: "", Result: "result", Constant: nil},
		{Argument: "", Result: "0", Constant: nil},
		{Argument: "", Result: "", Constant: false},
		{Argument: "", Result: "", Constant: 0},
		{Argument: "", Result: "", Constant: ""},
	} {
		_, err := validate.AttributeAccessor(code, symbol, source)
		if err != nil {
			t.Fatalf("valid source %+v: %v", source, err)
		}
	}

	for _, source := range []model.AttributeSource{
		{Argument: "request", Result: "", Constant: nil},
		{Argument: "request.Payload", Result: "", Constant: nil},
		{Argument: "request.password", Result: "", Constant: nil},
		{Argument: "request.ID", Result: "", Constant: nil},
		{Argument: "missing.Kind", Result: "", Constant: nil},
		{Argument: "", Result: "3", Constant: nil},
		{Argument: "", Result: "", Constant: map[string]string{"key": "value"}},
		{Argument: "", Result: "", Constant: math.NaN()},
		{Argument: "", Result: "", Constant: uint64(math.MaxUint64)},
		{Argument: "request.Count", Result: "", Constant: 1},
		{Argument: "", Result: "", Constant: nil},
	} {
		_, err := validate.AttributeAccessor(code, symbol, source)
		if err == nil {
			t.Fatalf("invalid source accepted: %+v", source)
		}
	}
}

func TestSafetySecretsAndAcknowledgment(t *testing.T) {
	t.Parallel()

	code, symbol := attributeFixture(t)
	attr := constantAttribute("API_KEY", "do-not-print-this-value")
	target := safetyTarget(symbol.ID, []model.AttributePlan{attr})
	plan := model.ResolvedPlan{APIVersion: "", Targets: []model.ResolvedTarget{target}, Skipped: nil}

	var options validate.Options

	diags := validate.Safety(code, plan, options)
	if !diags.HasErrors() || diags[0].Code != model.CodeSecretAttribute {
		t.Fatalf("secret accepted: %+v", diags)
	}

	encoded, err := json.Marshal(diags)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(encoded), "do-not-print-this-value") {
		t.Fatal("diagnostic leaked constant")
	}

	plan.Targets[0].Attributes[0].Allow = true

	plan.Targets[0].Attributes[0].Classification = model.ClassificationPublic

	if !validate.Safety(code, plan, options).HasErrors() {
		t.Fatal("public classification bypassed secret protection")
	}

	plan.Targets[0].Attributes[0].Classification = model.ClassificationSecret
	if validate.Safety(code, plan, options).HasErrors() {
		t.Fatal("explicit secret acknowledgment rejected")
	}

	plan.Targets[0].Attributes[0] = constantAttribute("bank_reference", "reference")
	options.DenyPatterns = []string{"bank"}

	if !validate.Safety(code, plan, options).HasErrors() {
		t.Fatal("custom deny pattern ignored")
	}
}

func TestSafetyWarningsAndLimits(t *testing.T) {
	t.Parallel()

	code, symbol := attributeFixture(t)
	target := safetyTarget(symbol.ID, []model.AttributePlan{constantAttribute("user_id", "id")})
	target.ContextStrategy.Strategy = model.ContextStrategyRoot
	plan := model.ResolvedPlan{APIVersion: "", Targets: []model.ResolvedTarget{target, target}, Skipped: nil}
	options := validate.Options{DenyPatterns: nil, WarningTargets: 1, MaximumTargets: 1, AllowLargePlan: false}

	diags := validate.Safety(code, plan, options)
	if !diags.HasErrors() || len(diags.Warnings()) < 3 {
		t.Fatalf("warnings or limit absent: %+v", diags)
	}

	options.AllowLargePlan = true

	if validate.Safety(code, plan, options).HasErrors() {
		t.Fatal("large plan acknowledgment rejected")
	}
}

func constantAttribute(key string, value any) model.AttributePlan {
	return model.AttributePlan{
		Key: key, From: model.AttributeSource{Argument: "", Result: "", Constant: value},
		Classification: "", Allow: false,
	}
}

func safetyTarget(symbolID model.SymbolID, attributes []model.AttributePlan) model.ResolvedTarget {
	return model.ResolvedTarget{
		SymbolID: symbolID, RuleID: "operation", SpanName: "", Signature: "",
		ContextStrategy: model.ContextStrategy{Strategy: "", Index: 0},
		ErrorStrategy:   model.ErrorStrategy{Record: false, Indexes: nil},
		Attributes:      attributes,
	}
}
