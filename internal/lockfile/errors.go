package lockfile

import "errors"

var (
	errMissingAnalysis           = errors.New("policy and analyzed Go version are required")
	errBackendPolicyMismatch     = errors.New("backend identity must match an exact pinned policy version")
	errMissingGraphDigest        = errors.New("module graph digest is required")
	errDuplicateTarget           = errors.New("duplicate lock target")
	errTargetSignatureMismatch   = errors.New("target signature does not match analyzed symbol")
	errNonrelativeSource         = errors.New("source path must be module-relative")
	errMultipleDocuments         = errors.New("lockfile must contain one JSON document")
	errInvalidIdentity           = errors.New("invalid lockfile identity")
	errInvalidBackend            = errors.New("invalid locked backend")
	errInvalidBackendDigest      = errors.New("invalid backend digest")
	errInvalidContextStrategy    = errors.New("invalid context strategy")
	errInvalidContextIndex       = errors.New("invalid context index")
	errInvalidErrorIndex         = errors.New("invalid error index")
	errInvalidAttributeKey       = errors.New("invalid attribute key")
	errInvalidAttributeKind      = errors.New("invalid attribute kind")
	errInvalidTarget             = errors.New("invalid or duplicate locked target")
	errInvalidArtifacts          = errors.New("invalid artifact manifest")
	errRefreshBuildIdentity      = errors.New("resolution refresh cannot supply build-owned identity")
	errRefreshStaleBuildIdentity = errors.New(
		"resolution changed while the lock contains build-owned identity; " +
			"refresh requires regenerated build metadata",
	)
	errMissingModuleMetadata       = errors.New("module metadata is required")
	errMissingLocalModuleDirectory = errors.New("local module directory is unavailable")
	errInvalidWorkspaceModule      = errors.New("workspace module has invalid manifest")
)
