package model

type LockContext struct {
	Strategy string `json:"strategy"`
	Index    int    `json:"index,omitempty"`
}

type LockErrors struct {
	Indexes []int `json:"indexes,omitempty"`
}

type LockAttribute struct {
	Key  string          `json:"key"`
	From AttributeSource `json:"from"`
}

type LockTarget struct {
	Symbol          SymbolID        `json:"symbol"`
	Signature       string          `json:"signature"`
	SignatureDigest string          `json:"signatureDigest"`
	SourceRule      string          `json:"sourceRule"`
	SpanName        string          `json:"spanName"`
	Context         LockContext     `json:"context"`
	Errors          LockErrors      `json:"errors"`
	Attributes      []LockAttribute `json:"attributes,omitempty"`
}

type LockBackend struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest,omitempty"`
}

type Lockfile struct {
	APIVersion        string       `json:"apiVersion"`
	PolicyDigest      string       `json:"policyDigest"`
	GoVersion         string       `json:"goVersion"`
	ModuleGraphDigest string       `json:"moduleGraphDigest"`
	Backend           LockBackend  `json:"backend"`
	Targets           []LockTarget `json:"targets"`
}

type DiffClassification string

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
)

type LockDiffEntry struct {
	Classification DiffClassification `json:"classification"`
	Symbol         SymbolID           `json:"symbol,omitempty"`
	Detail         string             `json:"detail,omitempty"`
}

type LockDiff struct {
	Entries []LockDiffEntry `json:"entries"`
}

func (d LockDiff) Empty() bool {
	return len(d.Entries) == 0
}
