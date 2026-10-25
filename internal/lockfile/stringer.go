package lockfile

import (
	"errors"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var errInvalidStringerAttribute = errors.New("invalid stringer attribute")

func checkStringerAttribute(attribute model.LockAttribute) error {
	if !attribute.Stringer {
		return nil
	}

	source := attribute.From
	if (source.Argument != "") == (source.Result != "") {
		return errInvalidStringerAttribute
	}

	if source.Constant != nil || source.Symbol != "" || source.ErrorType != "" {
		return errInvalidStringerAttribute
	}

	if attribute.Kind != attributeStringKind || attribute.SymbolValue != nil {
		return errInvalidStringerAttribute
	}

	return nil
}
