package model

// ContextStrategy identifies the source of a span context. Index is the
// zero-based parameter index when Strategy is ContextStrategyArgument.
type ContextStrategy struct {
	Strategy string `json:"strategy"`
	Index    int    `json:"index,omitempty"`
}

// Context strategies distinguish parameter propagation, root spans, and no context.
const (
	ContextStrategyArgument = "argument"
	ContextStrategyRoot     = "root"
	ContextStrategyNone     = "none"
)

// ErrorStrategy selects the zero-based result indexes whose errors are recorded.
type ErrorStrategy struct {
	Record  bool  `json:"record"`
	Indexes []int `json:"indexes,omitempty"`
}

// AttributePlan binds an explicit telemetry attribute to its source and
// safety classification after policy resolution.
type AttributePlan struct {
	Key            string               `json:"key"`
	From           AttributeSource      `json:"from"`
	Classification SafetyClassification `json:"classification,omitempty"`
	Allow          bool                 `json:"allow,omitempty"`
}

// ResolvedTarget identifies one exact instrumentation target and the rule
// responsible for its span, context, error, and attribute behavior.
type ResolvedTarget struct {
	SymbolID        SymbolID        `json:"symbol"`
	SpanName        string          `json:"spanName"`
	ContextStrategy ContextStrategy `json:"context"`
	ErrorStrategy   ErrorStrategy   `json:"errors"`
	Attributes      []AttributePlan `json:"attributes,omitempty"`
	RuleID          string          `json:"sourceRule"`
	Signature       string          `json:"signature,omitempty"`
}

// SkipReason explains why a candidate was not selected for instrumentation.
type SkipReason string

// Skip reasons identify visibility, exclusion, context, and ownership decisions.
const (
	SkipNotExported    SkipReason = "not exported"
	SkipExcluded       SkipReason = "excluded"
	SkipMissingContext SkipReason = "missing context"
	SkipDependency     SkipReason = "dependency code"
)

// SkippedTarget records a rejected candidate, its matching rule, and the reason.
type SkippedTarget struct {
	SymbolID SymbolID   `json:"symbol"`
	RuleID   string     `json:"rule"`
	Reason   SkipReason `json:"reason"`
}

// ResolvedPlan contains exact selected targets and explanations for skipped
// candidates. It is the backend-independent result of policy resolution.
type ResolvedPlan struct {
	APIVersion string           `json:"apiVersion"`
	Targets    []ResolvedTarget `json:"targets"`
	Skipped    []SkippedTarget  `json:"skipped,omitempty"`
}
