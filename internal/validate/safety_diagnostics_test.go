package validate_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSafetyDiagnosticContract(t *testing.T) {
	t.Parallel()

	code, symbol := attributeFixture(t)

	var target model.ResolvedTarget

	target.SymbolID = symbol.ID
	target.RuleID = "operation"
	target.ContextStrategy.Strategy = model.ContextStrategyRoot
	target.ErrorStrategy.Record = true
	target.Attributes = []model.AttributePlan{
		{Key: "email_token", From: model.AttributeSource{Argument: "", Result: "", Constant: "private-value"},
			Classification: "", Allow: false},
		{Key: "query", From: model.AttributeSource{Argument: "", Result: "", Constant: nil},
			Classification: "", Allow: false},
	}

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	var options validate.Options

	encoded, err := json.Marshal(validate.Safety(code, plan, options))
	if err != nil {
		t.Fatal(err)
	}

	var expected bytes.Buffer

	err = json.Compact(&expected, []byte(safetyDiagnosticJSON))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encoded, expected.Bytes()) {
		t.Fatalf("diagnostics = %s, want %s", encoded, expected.Bytes())
	}
}

const safetyDiagnosticJSON = `[
 {"severity":"warning","code":"OTP3001","message":"explicit root span will not inherit caller context",
  "rule":"operation","symbol":"example.com/app.Run"},
 {"severity":"warning","code":"OTP1005","message":"error recording requested but target has no error result",
  "rule":"operation","symbol":"example.com/app.Run"},
 {"severity":"error","code":"OTP4001",
  "message":"sensitive attribute requires explicit secret classification and acknowledgment",
  "rule":"operation","symbol":"example.com/app.Run"},
 {"severity":"warning","code":"OTP4002",
  "message":"attribute may contain personal information; review classification and consent",
  "rule":"operation","symbol":"example.com/app.Run"},
 {"severity":"error","code":"OTP4005","message":"attribute must have exactly one source",
  "rule":"operation","symbol":"example.com/app.Run"},
 {"severity":"warning","code":"OTP4003",
  "message":"attribute may have unbounded cardinality; prefer a bounded category",
  "rule":"operation","symbol":"example.com/app.Run"}
]`
