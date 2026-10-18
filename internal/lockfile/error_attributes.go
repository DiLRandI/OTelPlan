package lockfile

import (
	"errors"
	"go/token"
	"strconv"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var errInvalidErrorTypeAttribute = errors.New("invalid error-type attribute")

func checkErrorTypeAttribute(attribute model.LockAttribute) error {
	source := attribute.From
	if source.ErrorType == "" {
		return nil
	}

	if source.Argument != "" || source.Result != "" || source.Symbol != "" || source.Constant != nil ||
		attribute.Kind != attributeStringKind || attribute.SymbolValue != nil || !validErrorTypeSelector(source.ErrorType) {
		return errInvalidErrorTypeAttribute
	}

	return nil
}

func validErrorTypeSelector(selector string) bool {
	if token.IsIdentifier(selector) && selector != "_" {
		return true
	}

	index, err := strconv.Atoi(selector)

	return err == nil && index >= 0
}
