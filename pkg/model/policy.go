package model

const (
	APIVersionV1Alpha1      = "otelplan.io/v1alpha1"
	KindInstrumentationPlan = "InstrumentationPlan"
	LockAPIVersionV1Alpha1  = "otelplan.io/lock/v1alpha1"
	BackendNameOTelC        = "otelc"
)

type ContextMode string

const (
	ContextModeRequire ContextMode = "require"
	ContextModeRoot    ContextMode = "root"
)

type ProjectConfig struct {
	Packages            []string `json:"packages,omitempty"            yaml:"packages,omitempty"`
	IncludeTests        bool     `json:"includeTests,omitempty"        yaml:"includeTests,omitempty"`
	IncludeDependencies bool     `json:"includeDependencies,omitempty" yaml:"includeDependencies,omitempty"`
	BuildTags           []string `json:"buildTags,omitempty"           yaml:"buildTags,omitempty"`
}

type BackendConfig struct {
	Name    string `json:"name"    yaml:"name"`
	Version string `json:"version" yaml:"version"`
}

type ContextDefaults struct {
	Mode ContextMode `json:"mode,omitempty" yaml:"mode,omitempty"`
}

type ErrorDefaults struct {
	Record bool `json:"record,omitempty" yaml:"record,omitempty"`
}

type CaptureDefaults struct {
	Arguments bool `json:"arguments,omitempty" yaml:"arguments,omitempty"`
	Results   bool `json:"results,omitempty"   yaml:"results,omitempty"`
}

type Defaults struct {
	SpanName   string          `json:"spanName,omitempty" yaml:"spanName,omitempty"`
	Context    ContextDefaults `json:"context"            yaml:"context,omitempty"`
	Errors     ErrorDefaults   `json:"errors"             yaml:"errors,omitempty"`
	Attributes CaptureDefaults `json:"attributes"         yaml:"attributes,omitempty"`
}

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

func (m Match) IsEmpty() bool {
	return len(m.Packages) == 0 && len(m.Files) == 0 && len(m.Symbols) == 0 &&
		len(m.Functions) == 0 && len(m.Receivers) == 0 && len(m.Methods) == 0 &&
		len(m.Implements) == 0 && m.Exported == nil && m.HasContext == nil &&
		m.ReturnsError == nil && m.Ownership == ""
}

type SafetyClassification string

const (
	ClassificationPublic   SafetyClassification = "public"
	ClassificationInternal SafetyClassification = "internal"
	ClassificationPII      SafetyClassification = "pii"
	ClassificationSecret   SafetyClassification = "secret"
)

type Safety struct {
	Classification SafetyClassification `json:"classification,omitempty" yaml:"classification,omitempty"`
	Allow          bool                 `json:"allow,omitempty"          yaml:"allow,omitempty"`
}

type AttributeSource struct {
	Argument string `json:"argument,omitempty" yaml:"argument,omitempty"`
	Result   string `json:"result,omitempty"   yaml:"result,omitempty"`
	Constant any    `json:"constant,omitempty" yaml:"constant,omitempty"`
}

type AttributeRule struct {
	Key    string          `json:"key"              yaml:"key"`
	From   AttributeSource `json:"from"             yaml:"from"`
	Safety *Safety         `json:"safety,omitempty" yaml:"safety,omitempty"`
}

type SpanConfig struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

type ErrorConfig struct {
	Record bool `json:"record,omitempty" yaml:"record,omitempty"`
}

type Rule struct {
	ID          string          `json:"id"                    yaml:"id"`
	Description string          `json:"description,omitempty" yaml:"description,omitempty"`
	Match       Match           `json:"match"                 yaml:"match"`
	Exclude     *Match          `json:"exclude,omitempty"     yaml:"exclude,omitempty"`
	Span        *SpanConfig     `json:"span,omitempty"        yaml:"span,omitempty"`
	Errors      *ErrorConfig    `json:"errors,omitempty"      yaml:"errors,omitempty"`
	Attributes  []AttributeRule `json:"attributes,omitempty"  yaml:"attributes,omitempty"`
}

type Exclusion struct {
	ID    string `json:"id"    yaml:"id"`
	Match Match  `json:"match" yaml:"match"`
}

type Policy struct {
	APIVersion string        `json:"apiVersion"           yaml:"apiVersion"`
	Kind       string        `json:"kind"                 yaml:"kind"`
	Project    ProjectConfig `json:"project"              yaml:"project,omitempty"`
	Backend    BackendConfig `json:"backend"              yaml:"backend"`
	Defaults   Defaults      `json:"defaults"             yaml:"defaults,omitempty"`
	Rules      []Rule        `json:"rules"                yaml:"rules"`
	Exclusions []Exclusion   `json:"exclusions,omitempty" yaml:"exclusions,omitempty"`
}
