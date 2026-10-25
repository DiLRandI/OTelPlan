package validate

import (
	"errors"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var (
	errStringerSource = errors.New("stringer conversion requires a named or indexed argument or result source")
	errStringerType   = errors.New("stringer conversion requires a source implementing String() string")
)

func validateStringerSource(source model.AttributeSource) error {
	path := source.Argument
	if path == "" {
		path = source.Result
	}

	root, _, _ := strings.Cut(path, ".")
	if root == "" || root == "_" {
		return errStringerSource
	}

	return nil
}
