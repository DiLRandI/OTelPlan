package validate_test

import (
	"math"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestConstantAccessorErrorMessages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		value   any
		message string
	}{
		{name: "missing source", value: nil, message: "attribute must have exactly one source"},
		{name: "collection", value: []int{1}, message: "constant must be a scalar"},
		{name: "not a number", value: math.NaN(), message: "constant must be finite"},
		{name: "infinity", value: float32(math.Inf(1)), message: "constant must be finite"},
		{name: "overflow", value: uint64(math.MaxUint64), message: "integer constant exceeds telemetry range"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var symbol model.Symbol

			source := model.AttributeSource{Argument: "", Result: "", Constant: testCase.value}

			access, err := validate.AttributeAccessor(nil, symbol, source)
			if err == nil || err.Error() != testCase.message {
				t.Fatalf("error = %v, want %q", err, testCase.message)
			}

			if access.Source != "" || access.Index != 0 || access.Fields != nil || access.Type != "" || access.Kind != "" {
				t.Fatalf("accessor = %+v, want zero value", access)
			}
		})
	}
}
