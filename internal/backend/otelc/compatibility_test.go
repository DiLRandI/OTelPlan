package otelc

import (
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func compatibilityFixture() (*model.CodeModel, model.ResolvedPlan) {
	symbol := model.Symbol{ID: "example.com/app.Run", HasBody: true, Signature: "func(context.Context) error", Parameters: []model.Parameter{{Type: "context.Context"}}, Results: []model.Result{{Type: "error"}}, ContextIndexes: []int{0}, ErrorIndexes: []int{0}}
	return &model.CodeModel{Symbols: []model.Symbol{symbol}}, model.ResolvedPlan{Targets: []model.ResolvedTarget{{SymbolID: symbol.ID, Signature: symbol.Signature, RuleID: "run", ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument}, ErrorStrategy: model.ErrorStrategy{Record: true, Indexes: []int{0}}}}}
}

func TestPinnedCapabilities(t *testing.T) {
	identity, err := Identity(SupportedVersion)
	if err != nil || !identity.Capabilities.ContextReplacement || !identity.Capabilities.AfterHook || identity.Capabilities.PanicObservation {
		t.Fatalf("capabilities: %+v %v", identity, err)
	}
	for _, version := range []string{"latest", "v1.0.0", "v1.1", "v1.2.0"} {
		if _, err := Identity(version); err == nil {
			t.Fatalf("unverified version %s accepted", version)
		}
	}
	code, plan := compatibilityFixture()
	if diags := Check(SupportedVersion, code, plan); diags.HasErrors() {
		t.Fatalf("compatible target rejected: %+v", diags)
	}
}

func TestRejectUnsupportedTargets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.CodeModel, *model.ResolvedPlan)
	}{
		{"generic", func(c *model.CodeModel, _ *model.ResolvedPlan) {
			c.Symbols[0].Generics = &model.GenericInfo{TypeParams: []string{"T"}}
		}},
		{"declaration", func(c *model.CodeModel, _ *model.ResolvedPlan) { c.Symbols[0].HasBody = false }},
		{"context", func(_ *model.CodeModel, p *model.ResolvedPlan) { p.Targets[0].ContextStrategy.Index = 2 }},
		{"error result", func(_ *model.CodeModel, p *model.ResolvedPlan) { p.Targets[0].ErrorStrategy.Indexes = []int{2} }},
		{"signature", func(_ *model.CodeModel, p *model.ResolvedPlan) { p.Targets[0].Signature = "changed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, plan := compatibilityFixture()
			tc.change(code, &plan)
			if !Check(SupportedVersion, code, plan).HasErrors() {
				t.Fatal("unsupported target accepted")
			}
		})
	}
}

func TestMissingExecutable(t *testing.T) {
	if _, err := VerifyExecutable(t.Context(), "/missing/otelc", SupportedVersion); err == nil {
		t.Fatal("missing executable accepted")
	}
}

func TestPinnedExecutableIdentity(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to the pinned backend executable")
	}
	identity, err := VerifyExecutable(t.Context(), executable, SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Version != SupportedVersion || !strings.HasPrefix(identity.Digest, "sha256:") || len(identity.Digest) != 71 {
		t.Fatalf("invalid executable identity: %+v", identity)
	}
}
