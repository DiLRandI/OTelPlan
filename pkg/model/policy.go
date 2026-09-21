package model

// Public schema and backend identifiers accepted by the policy and lockfile
// formats.
const (
	APIVersionV1Alpha1      = "otelplan.io/v1alpha1"
	KindInstrumentationPlan = "InstrumentationPlan"
	LockAPIVersionV1Alpha1  = "otelplan.io/lock/v1alpha1"
	BackendNameOTelC        = "otelc"
)

// ContextMode selects whether a target requires a usable context argument or
// may start a root span.
type ContextMode string

// Context modes supported by policy defaults.
const (
	ContextModeRequire ContextMode = "require"
	ContextModeRoot    ContextMode = "root"
)

// ProjectConfig selects packages and build inputs for code discovery.
type ProjectConfig struct {
	Packages            []string `json:"packages,omitempty"            yaml:"packages,omitempty"`
	IncludeTests        bool     `json:"includeTests,omitempty"        yaml:"includeTests,omitempty"`
	IncludeDependencies bool     `json:"includeDependencies,omitempty" yaml:"includeDependencies,omitempty"`
	BuildTags           []string `json:"buildTags,omitempty"           yaml:"buildTags,omitempty"`
}

// BackendConfig pins the backend name and version used to compile a plan.
type BackendConfig struct {
	Name    string `json:"name"    yaml:"name"`
	Version string `json:"version" yaml:"version"`
}

// ContextDefaults sets the policy-wide context requirement for targets.
type ContextDefaults struct {
	Mode ContextMode `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// ErrorDefaults sets whether selected returned errors are recorded by default.
type ErrorDefaults struct {
	Record bool `json:"record,omitempty" yaml:"record,omitempty"`
}

// CaptureDefaults controls the default argument and result capture settings.
// Safe policy defaults leave both disabled.
type CaptureDefaults struct {
	Arguments bool `json:"arguments,omitempty" yaml:"arguments,omitempty"`
	Results   bool `json:"results,omitempty"   yaml:"results,omitempty"`
}

// Defaults contains policy-wide span, context, error, and attribute settings.
type Defaults struct {
	SpanName   string          `json:"spanName,omitempty" yaml:"spanName,omitempty"`
	Context    ContextDefaults `json:"context"            yaml:"context,omitempty"`
	Errors     ErrorDefaults   `json:"errors"             yaml:"errors,omitempty"`
	Attributes CaptureDefaults `json:"attributes"         yaml:"attributes,omitempty"`
}

// Match contains the ANDed selector fields used to identify candidate symbols;
// values within one field are alternatives.
type Match struct {
	Packages     []string  `json:"packages,omitempty"     yaml:"packages,omitempty"`
	Files        []string  `json:"files,omitempty"        yaml:"files,omitempty"`
	Symbols      []string  `json:"symbols,omitempty"      yaml:"symbols,omitempty"`
	Functions    []string  `json:"functions,omitempty"    yaml:"functions,omitempty"`
	Receivers    []string  `json:"receivers,omitempty"    yaml:"receivers,omitempty"`
	Methods      []string  `json:"methods,omitempty"      yaml:"methods,omitempty"`
	Implements   []string  `json:"implements,omitempty"   yaml:"implements,omitempty"`
	Exported     *bool     `json:"exported,omitempty"     yaml:"exported,omitempty"`
	HasContext   *bool     `json:"hasContext,omitempty"   yaml:"hasContext,omitempty"`
	ReturnsError *bool     `json:"returnsError,omitempty" yaml:"returnsError,omitempty"`
	Ownership    Ownership `json:"ownership,omitempty"    yaml:"ownership,omitempty"`
}

// IsEmpty reports whether the match contains no selector criteria.
func (m Match) IsEmpty() bool {
	for _, selectors := range [][]string{
		m.Packages, m.Files, m.Symbols, m.Functions, m.Receivers, m.Methods, m.Implements,
	} {
		if len(selectors) != 0 {
			return false
		}
	}

	return m.Exported == nil && m.HasContext == nil && m.ReturnsError == nil && m.Ownership == ""
}

// SafetyClassification describes the sensitivity category assigned to an
// explicitly captured attribute.
type SafetyClassification string

// Safety classifications used by attribute validation.
const (
	ClassificationPublic   SafetyClassification = "public"
	ClassificationInternal SafetyClassification = "internal"
	ClassificationPII      SafetyClassification = "pii"
	ClassificationSecret   SafetyClassification = "secret"
)

// Safety records an attribute's classification and whether the policy permits
// that capture.
type Safety struct {
	Classification SafetyClassification `json:"classification,omitempty" yaml:"classification,omitempty"`
	Allow          bool                 `json:"allow,omitempty"          yaml:"allow,omitempty"`
}

// AttributeSource identifies one argument, result, or constant value from
// which a policy attribute is obtained.
type AttributeSource struct {
	Argument string `json:"argument,omitempty" yaml:"argument,omitempty"`
	Result   string `json:"result,omitempty"   yaml:"result,omitempty"`
	Constant any    `json:"constant,omitempty" yaml:"constant,omitempty"`
}

// AttributeRule maps an explicit telemetry key to a source and optional safety
// acknowledgement.
type AttributeRule struct {
	Key    string          `json:"key"              yaml:"key"`
	From   AttributeSource `json:"from"             yaml:"from"`
	Safety *Safety         `json:"safety,omitempty" yaml:"safety,omitempty"`
}

// SpanConfig defines target span naming and kind overrides.
type SpanConfig struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

// ErrorConfig overrides whether a rule records returned errors.
type ErrorConfig struct {
	Record bool `json:"record,omitempty" yaml:"record,omitempty"`
}

// Rule combines selectors with exclusions and instrumentation behavior for its
// matching symbols.
type Rule struct {
	ID          string          `json:"id"                    yaml:"id"`
	Description string          `json:"description,omitempty" yaml:"description,omitempty"`
	Match       Match           `json:"match"                 yaml:"match"`
	Exclude     *Match          `json:"exclude,omitempty"     yaml:"exclude,omitempty"`
	Span        *SpanConfig     `json:"span,omitempty"        yaml:"span,omitempty"`
	Errors      *ErrorConfig    `json:"errors,omitempty"      yaml:"errors,omitempty"`
	Attributes  []AttributeRule `json:"attributes,omitempty"  yaml:"attributes,omitempty"`
}

// Exclusion removes matching symbols from policy selection.
type Exclusion struct {
	ID    string `json:"id"    yaml:"id"`
	Match Match  `json:"match" yaml:"match"`
}

// Policy is the complete backend-independent instrumentation policy supplied by
// the user.
type Policy struct {
	APIVersion string        `json:"apiVersion"           yaml:"apiVersion"`
	Kind       string        `json:"kind"                 yaml:"kind"`
	Project    ProjectConfig `json:"project"              yaml:"project,omitempty"`
	Backend    BackendConfig `json:"backend"              yaml:"backend"`
	Defaults   Defaults      `json:"defaults"             yaml:"defaults,omitempty"`
	Rules      []Rule        `json:"rules"                yaml:"rules"`
	Exclusions []Exclusion   `json:"exclusions,omitempty" yaml:"exclusions,omitempty"`
}
