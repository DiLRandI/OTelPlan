package model

// LockContext records the context strategy and parameter index frozen for a
// resolved target.
type LockContext struct {
	Strategy string `json:"strategy"`
	Index    int    `json:"index"`
}

// LockErrors records whether returned errors are captured and which result
// indexes carry them.
type LockErrors struct {
	Record  bool  `json:"record"`
	Indexes []int `json:"indexes,omitempty"`
}

// LockAttribute records an attribute source and its safety decision in the
// reproducible plan.
type LockAttribute struct {
	Key            string               `json:"key"`
	From           AttributeSource      `json:"from"`
	Kind           string               `json:"kind"`
	Classification SafetyClassification `json:"classification,omitempty"`
	Allow          bool                 `json:"allow,omitempty"`
}

// LockTarget freezes one exact symbol selection together with its signature,
// source rule, span behavior, safety metadata, and source location. Line and
// column changes are diagnostic metadata; file changes count as source drift.
type LockTarget struct {
	Symbol          SymbolID        `json:"symbol"`
	Signature       string          `json:"signature"`
	SignatureDigest string          `json:"signatureDigest"`
	SourceRule      string          `json:"sourceRule"`
	SpanName        string          `json:"spanName"`
	Context         LockContext     `json:"context"`
	Errors          LockErrors      `json:"errors"`
	Attributes      []LockAttribute `json:"attributes,omitempty"`
	Location        SourceLocation  `json:"location"`
}

// LockBackend records backend identity and capabilities. Digest is the optional
// executable digest verified by the compilation phase.
type LockBackend struct {
	Name         string              `json:"name"`
	Version      string              `json:"version"`
	Digest       string              `json:"digest,omitempty"`
	Capabilities BackendCapabilities `json:"capabilities"`
}

// Lockfile is the reproducible record of policy, build, backend, target, and
// generated artifact identities.
type Lockfile struct {
	APIVersion        string         `json:"apiVersion"`
	PolicyDigest      string         `json:"policyDigest"`
	GoVersion         string         `json:"goVersion"`
	ModuleGraphDigest string         `json:"moduleGraphDigest"`
	Backend           LockBackend    `json:"backend"`
	Targets           []LockTarget   `json:"targets"`
	Artifacts         []ArtifactFile `json:"artifacts,omitempty"`
}

// DiffClassification identifies the part of a locked plan that changed.
type DiffClassification string

// Lock diff classifications used when comparing a current resolution with a
// locked plan.
const (
	DiffAdd           DiffClassification = "ADD"
	DiffRemove        DiffClassification = "REMOVE"
	DiffSignature     DiffClassification = "SIGNATURE"
	DiffPolicy        DiffClassification = "POLICY"
	DiffSpanName      DiffClassification = "SPAN_NAME"
	DiffContext       DiffClassification = "CONTEXT"
	DiffErrorStrategy DiffClassification = "ERROR_STRATEGY"
	DiffAttribute     DiffClassification = "ATTRIBUTE"
	DiffBackend       DiffClassification = "BACKEND"
	DiffSource        DiffClassification = "SOURCE"
	DiffBuild         DiffClassification = "BUILD"
	DiffArtifact      DiffClassification = "ARTIFACT"
)

// LockDiffEntry describes one classified change, optionally tied to a symbol.
type LockDiffEntry struct {
	Classification DiffClassification `json:"classification"`
	Symbol         SymbolID           `json:"symbol,omitempty"`
	Detail         string             `json:"detail,omitempty"`
}

// LockDiff is the ordered set of changes between two lock states.
type LockDiff struct {
	Entries []LockDiffEntry `json:"entries"`
}

// Empty reports whether the lock comparison found no changes.
func (d LockDiff) Empty() bool {
	return len(d.Entries) == 0
}
