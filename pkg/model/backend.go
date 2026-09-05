package model

import "context"

type BackendCapabilities struct {
	BeforeHook             bool `json:"beforeHook"`
	AfterHook              bool `json:"afterHook"`
	ArgumentRead           bool `json:"argumentRead"`
	ArgumentReplace        bool `json:"argumentReplace"`
	ResultRead             bool `json:"resultRead"`
	PanicObservation       bool `json:"panicObservation"`
	ContextReplacement     bool `json:"contextReplacement"`
	FunctionEntrySelection bool `json:"functionEntrySelection"`
	FunctionCallSelection  bool `json:"functionCallSelection"`
}

type Artifacts struct {
	Dir   string         `json:"dir"`
	Files []ArtifactFile `json:"files"`
}

type ArtifactFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type BuildRequest struct {
	Plan      ResolvedPlan
	Artifacts Artifacts
	GoArgs    []string
	RootDir   string
	WorkDir   string
}

type Backend interface {
	Name() string
	Version(ctx context.Context) (string, error)
	Capabilities(ctx context.Context) (BackendCapabilities, error)
	Validate(ctx context.Context, plan ResolvedPlan) DiagnosticList
	Compile(ctx context.Context, plan ResolvedPlan, outDir string) (Artifacts, error)
	Build(ctx context.Context, req BuildRequest) error
}
