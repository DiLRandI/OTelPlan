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
	Packages            []string `yaml:"packages,omitempty" json:"packages,omitempty"`
	IncludeTests        bool     `yaml:"includeTests,omitempty" json:"includeTests,omitempty"`
	IncludeDependencies bool     `yaml:"includeDependencies,omitempty" json:"includeDependencies,omitempty"`
	BuildTags           []string `yaml:"buildTags,omitempty" json:"buildTags,omitempty"`
}

type BackendConfig struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

type ContextDefaults struct {
	Mode ContextMode `yaml:"mode,omitempty" json:"mode,omitempty"`
}

type ErrorDefaults struct {
	Record bool `yaml:"record,omitempty" json:"record,omitempty"`
}

type CaptureDefaults struct {
	Arguments bool `yaml:"arguments,omitempty" json:"arguments,omitempty"`
	Results   bool `yaml:"results,omitempty" json:"results,omitempty"`
}

type Defaults struct {
	SpanName   string          `yaml:"spanName,omitempty" json:"spanName,omitempty"`
	Context    ContextDefaults `yaml:"context,omitempty" json:"context,omitempty"`
	Errors     ErrorDefaults   `yaml:"errors,omitempty" json:"errors,omitempty"`
	Attributes CaptureDefaults `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

type Match struct {
	Packages     []string  `yaml:"packages,omitempty" json:"packages,omitempty"`
	Files        []string  `yaml:"files,omitempty" json:"files,omitempty"`
	Symbols      []string  `yaml:"symbols,omitempty" json:"symbols,omitempty"`
	Functions    []string  `yaml:"functions,omitempty" json:"functions,omitempty"`
	Receivers    []string  `yaml:"receivers,omitempty" json:"receivers,omitempty"`
	Methods      []string  `yaml:"methods,omitempty" json:"methods,omitempty"`
	Implements   []string  `yaml:"implements,omitempty" json:"implements,omitempty"`
	Exported     *bool     `yaml:"exported,omitempty" json:"exported,omitempty"`
	HasContext   *bool     `yaml:"hasContext,omitempty" json:"hasContext,omitempty"`
	ReturnsError *bool     `yaml:"returnsError,omitempty" json:"returnsError,omitempty"`
	Ownership    Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty"`
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
	Classification SafetyClassification `yaml:"classification,omitempty" json:"classification,omitempty"`
	Allow          bool                 `yaml:"allow,omitempty" json:"allow,omitempty"`
}

type AttributeSource struct {
	Argument string `yaml:"argument,omitempty" json:"argument,omitempty"`
	Result   string `yaml:"result,omitempty" json:"result,omitempty"`
	Constant any    `yaml:"constant,omitempty" json:"constant,omitempty"`
}

type AttributeRule struct {
	Key    string          `yaml:"key" json:"key"`
	From   AttributeSource `yaml:"from" json:"from"`
	Safety *Safety         `yaml:"safety,omitempty" json:"safety,omitempty"`
}

type SpanConfig struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty"`
}

type ErrorConfig struct {
	Record bool `yaml:"record,omitempty" json:"record,omitempty"`
}

type Rule struct {
	ID          string          `yaml:"id" json:"id"`
	Description string          `yaml:"description,omitempty" json:"description,omitempty"`
	Match       Match           `yaml:"match" json:"match"`
	Exclude     *Match          `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Span        *SpanConfig     `yaml:"span,omitempty" json:"span,omitempty"`
	Errors      *ErrorConfig    `yaml:"errors,omitempty" json:"errors,omitempty"`
	Attributes  []AttributeRule `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

type Exclusion struct {
	ID    string `yaml:"id" json:"id"`
	Match Match  `yaml:"match" json:"match"`
}

type Policy struct {
	APIVersion string        `yaml:"apiVersion" json:"apiVersion"`
	Kind       string        `yaml:"kind" json:"kind"`
	Project    ProjectConfig `yaml:"project,omitempty" json:"project,omitempty"`
	Backend    BackendConfig `yaml:"backend" json:"backend"`
	Defaults   Defaults      `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Rules      []Rule        `yaml:"rules" json:"rules"`
	Exclusions []Exclusion   `yaml:"exclusions,omitempty" json:"exclusions,omitempty"`
}
