package validate_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestAttributeSourceIndexBounds(t *testing.T) {
	t.Parallel()

	code, symbol := attributeFixture(t)
	for _, selector := range []string{"-1", "1", "999999999999999999999999999", "missing"} {
		t.Run(selector, func(t *testing.T) {
			t.Parallel()

			source := model.AttributeSource{Argument: selector, Result: "", Constant: nil}

			_, err := validate.AttributeAccessor(code, symbol, source)
			if err == nil || err.Error() != "attribute source parameter or result does not exist" {
				t.Fatalf("error = %v, want missing source", err)
			}
		})
	}
}

func TestAttributeSourceRequiresTypeInformation(t *testing.T) {
	t.Parallel()

	_, symbol := attributeFixture(t)
	source := model.AttributeSource{Argument: "request.Count", Result: "", Constant: nil}

	_, err := validate.AttributeAccessor(nil, symbol, source)
	if err == nil || err.Error() != "attribute source type is unavailable" {
		t.Fatalf("error = %v, want unavailable type", err)
	}
}
