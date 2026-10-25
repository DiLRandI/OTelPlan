package validate_test

import (
	"os"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestStringerConversionRequiresExplicitTypedOptIn(t *testing.T) {
	t.Parallel()
	code, symbol := stringerValidationFixture(t)

	for _, selector := range []string{"value", "pointer", "envelope.Label"} {
		var attribute model.AttributePlan

		attribute.From.Argument = selector

		_, err := validate.AttributeAccessor(code, symbol, attribute)
		if err == nil {
			t.Fatal("object capture enabled without opt-in")
		}

		attribute.Stringer = true

		access, err := validate.AttributeAccessor(code, symbol, attribute)
		if err != nil || !access.Stringer || access.Kind != "string" {
			t.Fatalf("Stringer source rejected: %v %+v", err, access)
		}
	}
}

func TestStringerPrimitiveCaptureKeepsDefaultAndOptInDistinct(t *testing.T) {
	t.Parallel()
	code, symbol := stringerValidationFixture(t)

	var attribute model.AttributePlan

	attribute.From.Argument = "category"

	plain, err := validate.AttributeAccessor(code, symbol, attribute)
	if err != nil || plain.Kind != "integer" || plain.Stringer {
		t.Fatalf("primitive behavior changed: %v %+v", err, plain)
	}

	attribute.Stringer = true

	converted, err := validate.AttributeAccessor(code, symbol, attribute)
	if err != nil || converted.Kind != "string" || !converted.Stringer {
		t.Fatalf("primitive opt-in ignored: %v %+v", err, converted)
	}
}

func TestStringerConversionRejectsUnsafeAndIncompatibleSources(t *testing.T) {
	t.Parallel()
	code, symbol := stringerValidationFixture(t)

	for _, selector := range []string{"pointerValue", "wrong", "plain", "envelope.private", "missing", "_"} {
		var attribute model.AttributePlan

		attribute.From.Argument, attribute.Stringer = selector, true

		_, err := validate.AttributeAccessor(code, symbol, attribute)
		if err == nil {
			t.Fatalf("incompatible source accepted: %s", selector)
		}
	}

	for _, source := range []model.AttributeSource{
		{Argument: "", Result: "", Constant: "literal", Symbol: "", ErrorType: ""},
		{Argument: "", Result: "", Constant: nil, Symbol: "name", ErrorType: ""},
		{Argument: "", Result: "", Constant: nil, Symbol: "", ErrorType: "0"},
	} {
		var attribute model.AttributePlan

		attribute.From, attribute.Stringer = source, true

		_, err := validate.AttributeAccessor(code, symbol, attribute)
		if err == nil {
			t.Fatal("static/error source conversion accepted")
		}
	}
}

func TestStringerConversionPreservesSensitiveSourceChecks(t *testing.T) {
	t.Parallel()
	code, symbol := stringerValidationFixture(t)

	var attribute model.AttributePlan

	attribute.Key, attribute.From.Argument, attribute.Stringer = "value", "credential", true
	target := safetyTarget(symbol.ID, []model.AttributePlan{attribute})

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	var options validate.Options

	diagnostics := validate.Safety(code, plan, options)
	if !containsDiagnosticCode(diagnostics, model.CodeSecretAttribute) {
		t.Fatal("Stringer bypassed sensitive-source validation")
	}

	plan.Targets[0].Attributes[0].Classification, plan.Targets[0].Attributes[0].Allow = model.ClassificationSecret, true
	if diagnostics := validate.Safety(code, plan, options); diagnostics.HasErrors() {
		t.Fatalf("explicit acknowledgment rejected: %+v", diagnostics)
	}
}

func stringerValidationFixture(t *testing.T) (*model.CodeModel, model.Symbol) {
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

	for name, content := range map[string]string{
		"go.mod": "module example.com/validation\n\ngo 1.27\n",
		"app.go": `package app
import "context"
type Value struct{}
func (Value) String() string { panic("validation must not execute String") }
type PointerOnly struct{}
func (*PointerOnly) String() string { panic("validation must not execute String") }
type Category int
func (Category) String() string { panic("validation must not execute String") }
type Wrong struct{}
func (Wrong) String(int) string { panic("validation must not execute String") }
type Envelope struct{ Label Value; private Value }
func Run(ctx context.Context, value Value, pointer *PointerOnly, pointerValue PointerOnly,
 category Category, envelope *Envelope, wrong Wrong, plain string, credential Value) {}
`,
	} {
		err := root.WriteFile(name, []byte(content), 0o600)
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

	symbol, exists := code.Symbol("example.com/validation.Run")
	if !exists {
		t.Fatal("validation fixture symbol missing")
	}

	return code, *symbol
}
