package model

type ContextStrategy struct {
	Strategy string `json:"strategy"`
	Index    int    `json:"index,omitempty"`
}

const (
	ContextStrategyArgument = "argument"
	ContextStrategyRoot     = "root"
	ContextStrategyNone     = "none"
)

type ErrorStrategy struct {
	Record  bool  `json:"record"`
	Indexes []int `json:"indexes,omitempty"`
}

type AttributePlan struct {
	Key            string               `json:"key"`
	From           AttributeSource      `json:"from"`
	Classification SafetyClassification `json:"classification,omitempty"`
	Allow          bool                 `json:"allow,omitempty"`
}

type ResolvedTarget struct {
	SymbolID        SymbolID        `json:"symbol"`
	SpanName        string          `json:"spanName"`
	ContextStrategy ContextStrategy `json:"context"`
	ErrorStrategy   ErrorStrategy   `json:"errors"`
	Attributes      []AttributePlan `json:"attributes,omitempty"`
	RuleID          string          `json:"sourceRule"`
	Signature       string          `json:"signature,omitempty"`
}

type SkipReason string

const (
	SkipNotExported    SkipReason = "not exported"
	SkipExcluded       SkipReason = "excluded"
	SkipMissingContext SkipReason = "missing context"
	SkipDependency     SkipReason = "dependency code"
)

type SkippedTarget struct {
	SymbolID SymbolID   `json:"symbol"`
	RuleID   string     `json:"rule"`
	Reason   SkipReason `json:"reason"`
}

type ResolvedPlan struct {
	APIVersion string           `json:"apiVersion"`
	Targets    []ResolvedTarget `json:"targets"`
	Skipped    []SkippedTarget  `json:"skipped,omitempty"`
}
