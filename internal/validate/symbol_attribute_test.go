package validate_test

import (
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSymbolMetadataValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		field, want string
	}{
		{field: "id", want: "example.com/app.(*Worker).Run"},
		{field: "package", want: "example.com/app"},
		{field: "packageName", want: "app"},
		{field: "name", want: "Run"},
		{field: "receiver", want: "*Worker"},
		{field: "signature", want: "func (*Worker).Run()"},
		{field: "kind", want: "method"},
	}

	var symbol model.Symbol

	symbol.ID = "example.com/app.(*Worker).Run"
	symbol.PackageImportPath, symbol.PackageName, symbol.Name = "example.com/app", "app", "Run"
	symbol.Receiver = &model.Receiver{Name: "receiver", Type: "Worker", Pointer: true}
	symbol.Signature, symbol.Kind = "func (*Worker).Run()", model.SymbolMethod

	for _, testCase := range testCases {
		got, err := validate.SymbolMetadataValue(symbol, testCase.field)
		if err != nil || got != testCase.want {
			t.Errorf("field %s=%q, %v; want %q", testCase.field, got, err, testCase.want)
		}
	}

	var function model.Symbol

	function.ID = "example.com/app.Run"
	function.PackageImportPath, function.PackageName, function.Name = "example.com/app", "app", "Run"
	function.Signature, function.Kind = "func Run()", model.SymbolFunction

	got, err := validate.SymbolMetadataValue(function, "receiver")
	if err != nil || got != "" {
		t.Fatalf("function receiver=%q, %v; want empty", got, err)
	}

	value := symbol
	value.Receiver = &model.Receiver{Name: "receiver", Type: "Worker", Pointer: false}

	got, err = validate.SymbolMetadataValue(value, "receiver")
	if err != nil || got != "Worker" {
		t.Fatalf("value receiver=%q, %v; want Worker", got, err)
	}

	_, err = validate.SymbolMetadataValue(symbol, "private-marker")
	if err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatalf("unknown field error=%v", err)
	}
}

func TestAttributeAccessorSymbolSourceNeedsNoCodeTypes(t *testing.T) {
	t.Parallel()

	var symbol model.Symbol

	symbol.ID = "example.com/app.Run"

	var source model.AttributeSource

	source.Symbol = "name"
	accessor, err := validate.AttributeAccessor(nil, symbol, source)

	valid := err == nil && accessor.Source == "constant" && accessor.Kind == "string" &&
		accessor.Type == "" && accessor.Index == 0
	if !valid {
		t.Fatalf("symbol accessor=%+v, %v", accessor, err)
	}

	if source.Symbol != "name" {
		t.Fatal("symbol source was mutated")
	}

	source.Constant = "other"

	_, err = validate.AttributeAccessor(nil, symbol, source)
	if err == nil {
		t.Fatal("mixed symbol and constant sources accepted")
	}
}
