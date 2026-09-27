package resolve_test

import (
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestResolveDiagnosticContext(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		prepare func(*model.Policy, *model.CodeModel) *model.CodeModel
		code    model.Code
		message string
		rule    string
		symbol  model.SymbolID
	}{
		{
			name:    "missing model",
			prepare: func(_ *model.Policy, _ *model.CodeModel) *model.CodeModel { return nil },
			code:    model.CodeInvalidPolicy, message: "code model is required", rule: "", symbol: "",
		},
		{
			name: "missing context",
			prepare: func(_ *model.Policy, code *model.CodeModel) *model.CodeModel {
				code.Symbols[0].ContextIndexes = nil

				return code
			},
			code: model.CodeMissingContext, message: "selected target has no context.Context argument",
			rule: "business", symbol: "example.com/app.Run",
		},
		{
			name: "missing exact symbol",
			prepare: func(policy *model.Policy, code *model.CodeModel) *model.CodeModel {
				policy.Rules[0].Match.Symbols = []string{"example.com/app.Missing"}

				return code
			},
			code: model.CodeUnresolvedSymbol, message: "exact symbol does not exist",
			rule: "business", symbol: "example.com/app.Missing",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			policy, code := resolverInputs()
			result := resolve.Resolve(policy, testCase.prepare(policy, code))

			want := model.DiagnosticErrorList{{
				Severity: model.SeverityError, Code: testCase.code, Message: testCase.message,
				RuleID: testCase.rule, Symbol: testCase.symbol, File: "", Line: 0,
			}}
			if !reflect.DeepEqual(result.Diagnostics, want) {
				t.Fatalf("diagnostics = %+v; want %+v", result.Diagnostics, want)
			}
		})
	}
}
