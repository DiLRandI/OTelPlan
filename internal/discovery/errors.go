package discovery

import "errors"

var (
	errCustomPackageDriver     = errors.New("custom GOPACKAGESDRIVER is unsupported for reproducible analysis")
	errModuleManifestExtension = errors.New("alternate module manifest must have a .mod extension")
	errUnsupportedModuleMode   = errors.New("invalid GOFLAGS: unsupported module mode")
	errEmptyModuleManifest     = errors.New("invalid GOFLAGS: empty modfile")
	errInvalidFlagToken        = errors.New("invalid GOFLAGS token")
	errUnterminatedQuote       = errors.New("unterminated quoted string")
	errInvalidGoFlags          = errors.New("invalid GOFLAGS")
	errUnsupportedGoFlag       = errors.New("unsupported GOFLAGS option")
	errPackageAnalysis         = errors.New("package analysis failed")
)
