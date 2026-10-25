package validate_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestErrorTypeAccessorRequiresCompleteErrorResult(t *testing.T) {
	t.Parallel()
	code, symbol := errorTypeFixture(t)

	before, err := json.Marshal(code)
	if err != nil {
		t.Fatal(err)
	}

	for _, selector := range []string{"err", "0"} {
		var source model.AttributeSource

		source.ErrorType = selector

		accessor, err := validate.AttributeAccessor(code, symbol, sourceAttribute(source))
		if err != nil {
			t.Fatal(err)
		}

		if accessor.Source != "result" || accessor.Kind != "string" || accessor.Index != 0 ||
			accessor.Type != symbol.Results[0].Type {
			t.Fatalf("unexpected error source accessor: %+v", accessor)
		}
	}

	after, err := json.Marshal(code)
	if err != nil || string(before) != string(after) {
		t.Fatalf("accessor mutated caller model: %v", err)
	}
}

func TestErrorTypeAccessorRejectsInvalidSources(t *testing.T) {
	t.Parallel()
	code, symbol := errorTypeFixture(t)

	checks := []struct {
		selector string
		mixed    bool
	}{
		{"", false}, {"err.Code", false}, {"caller-private-value", false}, {"3", false},
		{"-1", false}, {"999999999999999999999999", false}, {"value", false}, {"err", true},
	}
	for _, check := range checks {
		var source model.AttributeSource

		source.ErrorType = check.selector
		if check.mixed {
			source.Result = "err"
		}

		_, err := validate.AttributeAccessor(code, symbol, sourceAttribute(source))
		if err == nil || strings.Contains(err.Error(), "caller-private-value") {
			t.Fatalf("invalid source accepted or leaked: %v", err)
		}
	}

	var source model.AttributeSource

	source.ErrorType = "err"

	_, err := validate.AttributeAccessor(nil, symbol, sourceAttribute(source))
	if err == nil {
		t.Fatal("missing model accepted")
	}

	code.Types = nil

	_, err = validate.AttributeAccessor(code, symbol, sourceAttribute(source))
	if err == nil {
		t.Fatal("missing TypeInfo accepted")
	}
}

func TestErrorTypeSourceUsesExistingSafetyClassification(t *testing.T) {
	t.Parallel()
	code, symbol := errorTypeFixture(t)

	var attribute model.AttributePlan

	attribute.Key, attribute.From.ErrorType = "error.token", "err"
	target := safetyTarget(symbol.ID, []model.AttributePlan{attribute})

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	var options validate.Options

	diagnostics := validate.Safety(code, plan, options)
	if !containsDiagnosticCode(diagnostics, model.CodeSecretAttribute) {
		t.Fatal("sensitive key was not rejected")
	}

	plan.Targets[0].Attributes[0].Key = "failure.class"
	plan.Targets[0].Attributes[0].From.ErrorType = "credential"

	diagnostics = validate.Safety(code, plan, options)
	if !containsDiagnosticCode(diagnostics, model.CodeSecretAttribute) {
		t.Fatal("sensitive source selector bypassed safety")
	}

	plan.Targets[0].Attributes[0].Classification, plan.Targets[0].Attributes[0].Allow = model.ClassificationSecret, true

	diagnostics = validate.Safety(code, plan, options)
	if diagnostics.HasErrors() {
		t.Fatalf("explicit source acknowledgment rejected: %+v", diagnostics)
	}
}

func errorTypeFixture(t *testing.T) (*model.CodeModel, model.Symbol) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	files := map[string]string{
		"go.mod": "module example.com/errorapp\n\ngo 1.27\n",
		"app.go": `package app
import "context"
type AppError struct{}
func (AppError) Error() string { return "private" }
func Run(context.Context) (err error, value int, credential error) { return AppError{}, 0, AppError{} }
type FailureAlias = error
func Alias() FailureAlias { return nil }
func Blanks() (_ error, _ error) { var absent error; return absent, nil }
func Concrete() AppError { return AppError{} }
`,
	}
	for name, contents := range files {
		err := root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var options discovery.Options

	options.Root, options.Offline = root.Name(), true

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	symbol, ok := code.Symbol("example.com/errorapp.Run")
	if !ok {
		t.Fatal("missing error fixture symbol")
	}

	return code, *symbol
}

func containsDiagnosticCode(diagnostics model.DiagnosticErrorList, want model.Code) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == want {
			return true
		}
	}

	return false
}

func TestErrorTypeAccessorKeepsExistingErrorResultRecognition(t *testing.T) {
	t.Parallel()
	code, _ := errorTypeFixture(t)

	var source model.AttributeSource

	source.ErrorType = "0"

	alias, exists := code.Symbol("example.com/errorapp.Alias")
	if !exists {
		t.Fatal("missing alias declaration")
	}

	_, err := validate.AttributeAccessor(code, *alias, sourceAttribute(source))
	if err != nil {
		t.Fatalf("error alias rejected: %v", err)
	}

	concrete, exists := code.Symbol("example.com/errorapp.Concrete")
	if !exists {
		t.Fatal("missing concrete declaration")
	}

	_, err = validate.AttributeAccessor(code, *concrete, sourceAttribute(source))
	if err == nil {
		t.Fatal("unrecognized concrete error result accepted")
	}

	blank, exists := code.Symbol("example.com/errorapp.Blanks")
	if !exists {
		t.Fatal("missing blank results")
	}

	source.ErrorType = "_"

	_, err = validate.AttributeAccessor(code, *blank, sourceAttribute(source))
	if err == nil {
		t.Fatal("ambiguous blank result name accepted")
	}
}
