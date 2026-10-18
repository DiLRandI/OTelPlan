package validate

import (
	"errors"
	"slices"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var errErrorTypeResult = errors.New("error-type attribute must select an entire error result by name or index")

func errorTypeAccessor(code *model.CodeModel, symbol model.Symbol, selector string) (Accessor, error) {
	if selector == "_" {
		return Accessor{}, errErrorTypeResult
	}

	var source model.AttributeSource

	source.Result = selector

	access, fields, err := sourceAccessor(symbol, source)
	if err != nil {
		return Accessor{}, err
	}

	if len(fields) != 0 || !slices.Contains(symbol.ErrorIndexes, access.Index) {
		return Accessor{}, errErrorTypeResult
	}

	if code == nil {
		return Accessor{}, errAttributeTypeUnavailable
	}

	for _, typ := range code.Types {
		if typ.Type == access.Type {
			access.Kind = attributeStringKind

			return access, nil
		}
	}

	return Accessor{}, errAttributeTypeUnavailable
}
