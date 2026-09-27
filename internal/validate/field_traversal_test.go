package validate_test

import (
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestAttributeFieldTraversalWithRecursiveEmbedding(t *testing.T) {
	t.Parallel()

	code, symbol := attributeFixture(t)
	source := model.AttributeSource{Argument: "request.Value", Result: "", Constant: nil}

	access, err := validate.AttributeAccessor(code, symbol, source)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(access.Fields, []string{"Cycle", "Value"}) || access.Kind != "string" {
		t.Fatalf("accessor = %+v, want promoted scalar through Cycle", access)
	}

	source.Argument = "request.Absent"

	_, err = validate.AttributeAccessor(code, symbol, source)
	if err == nil || err.Error() != "attribute field is missing or inaccessible" {
		t.Fatalf("error = %v, want missing field after recursive traversal", err)
	}
}
