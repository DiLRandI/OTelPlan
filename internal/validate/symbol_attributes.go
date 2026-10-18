package validate

import (
	"errors"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var errSymbolMetadataField = errors.New("unsupported symbol metadata field")

// SymbolMetadataValue resolves a stable declaration fact without reading
// application arguments or results. Functions have an empty receiver value.
func SymbolMetadataValue(symbol model.Symbol, field string) (string, error) {
	switch field {
	case model.SymbolFieldID:
		return string(symbol.ID), nil
	case model.SymbolFieldPackage:
		return symbol.PackageImportPath, nil
	case model.SymbolFieldPackageName:
		return symbol.PackageName, nil
	case model.SymbolFieldName:
		return symbol.Name, nil
	case model.SymbolFieldSignature:
		return symbol.Signature, nil
	case model.SymbolFieldKind:
		return string(symbol.Kind), nil
	case model.SymbolFieldReceiver:
		return symbolReceiverValue(symbol.Receiver), nil
	default:
		return "", errSymbolMetadataField
	}
}

func symbolReceiverValue(receiver *model.Receiver) string {
	if receiver == nil {
		return ""
	}

	if receiver.Pointer {
		return "*" + receiver.Type
	}

	return receiver.Type
}
