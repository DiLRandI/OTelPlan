package validate

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
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
type Request struct { *Details; Left; Right; Count int; Payload []byte }
func Run(request *Request) (result bool) { return true }
`}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	code, err := discovery.Load(discovery.Options{Root: root})
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
	code, symbol := attributeFixture(t)
	access, err := AttributeAccessor(code, symbol, model.AttributeSource{Argument: "request.Kind"})
	if err != nil || access.Kind != "string" || !reflect.DeepEqual(access.Fields, []string{"Details", "Kind"}) {
		t.Fatalf("promoted scalar: %+v %v", access, err)
	}
	for _, source := range []model.AttributeSource{{Argument: "request.Count"}, {Argument: "0.Count"}, {Result: "result"}, {Result: "0"}, {Constant: false}, {Constant: 0}, {Constant: ""}} {
		if _, err := AttributeAccessor(code, symbol, source); err != nil {
			t.Fatalf("valid source %+v: %v", source, err)
		}
	}
	for _, source := range []model.AttributeSource{{Argument: "request"}, {Argument: "request.Payload"}, {Argument: "request.password"}, {Argument: "request.ID"}, {Argument: "missing.Kind"}, {Result: "3"}, {Constant: map[string]string{"key": "value"}}, {Constant: math.NaN()}, {Constant: uint64(math.MaxUint64)}, {Argument: "request.Count", Constant: 1}, {}} {
		if _, err := AttributeAccessor(code, symbol, source); err == nil {
			t.Fatalf("invalid source accepted: %+v", source)
		}
	}
}

func TestSafetySecretsAndAcknowledgment(t *testing.T) {
	code, symbol := attributeFixture(t)
	attr := model.AttributePlan{Key: "API_KEY", From: model.AttributeSource{Constant: "do-not-print-this-value"}}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{{SymbolID: symbol.ID, RuleID: "operation", Attributes: []model.AttributePlan{attr}}}}
	diags := Safety(code, plan, Options{})
	if !diags.HasErrors() || diags[0].Code != model.CodeSecretAttribute {
		t.Fatalf("secret accepted: %+v", diags)
	}
	encoded, _ := json.Marshal(diags)
	if strings.Contains(string(encoded), "do-not-print-this-value") {
		t.Fatal("diagnostic leaked constant")
	}
	plan.Targets[0].Attributes[0].Allow = true
	plan.Targets[0].Attributes[0].Classification = model.ClassificationPublic
	if !Safety(code, plan, Options{}).HasErrors() {
		t.Fatal("public classification bypassed secret protection")
	}
	plan.Targets[0].Attributes[0].Classification = model.ClassificationSecret
	if Safety(code, plan, Options{}).HasErrors() {
		t.Fatal("explicit secret acknowledgment rejected")
	}
	plan.Targets[0].Attributes[0] = model.AttributePlan{Key: "bank_reference", From: model.AttributeSource{Constant: "reference"}}
	if !Safety(code, plan, Options{DenyPatterns: []string{"bank"}}).HasErrors() {
		t.Fatal("custom deny pattern ignored")
	}
}

func TestSafetyWarningsAndLimits(t *testing.T) {
	code, symbol := attributeFixture(t)
	target := model.ResolvedTarget{SymbolID: symbol.ID, RuleID: "operation", ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyRoot}, Attributes: []model.AttributePlan{{Key: "user_id", From: model.AttributeSource{Constant: "id"}}}}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{target, target}}
	diags := Safety(code, plan, Options{WarningTargets: 1, MaximumTargets: 1})
	if !diags.HasErrors() || len(diags.Warnings()) < 3 {
		t.Fatalf("warnings or limit absent: %+v", diags)
	}
	if Safety(code, plan, Options{WarningTargets: 1, MaximumTargets: 1, AllowLargePlan: true}).HasErrors() {
		t.Fatal("large plan acknowledgment rejected")
	}
}
