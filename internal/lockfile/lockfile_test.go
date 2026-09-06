package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func fixtureLock(t *testing.T) model.Lockfile {
	t.Helper()
	p := &model.Policy{Backend: model.BackendConfig{Name: "otelc", Version: "v1.1.0"}}
	code := &model.CodeModel{GoVersion: "go1.27.0", Symbols: []model.Symbol{{ID: "example.com/app.Run", Signature: "func()", Location: model.SourceLocation{File: "app.go", Line: 1}}}}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{{SymbolID: code.Symbols[0].ID, Signature: code.Symbols[0].Signature, SpanName: "app.Run", RuleID: "run", ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyRoot}, Attributes: []model.AttributePlan{{Key: "category", From: model.AttributeSource{Constant: 1}}}}}}
	lock, err := Create(p, code, plan, model.LockBackend{Name: "otelc", Version: "v1.1.0"}, Digest([]byte("module graph")), nil)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func TestLockRoundTripAndWrite(t *testing.T) {
	lock := fixtureLock(t)
	data, err := Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Marshal(parsed)
	if err != nil || string(data) != string(again) {
		t.Fatalf("noncanonical roundtrip: %v", err)
	}
	if !Diff(lock, parsed).Empty() {
		t.Fatal("numeric decoding caused false diff")
	}
	filename := filepath.Join(t.TempDir(), "otelplan.lock")
	if err := Write(filename, lock); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filename)
	if err != nil || string(got) != string(data) {
		t.Fatal("lockfile contents differ")
	}
	entries, err := os.ReadDir(filepath.Dir(filename))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary lockfile leaked")
	}
}

func TestLockRejectsMalformedAndTampered(t *testing.T) {
	lock := fixtureLock(t)
	data, _ := Marshal(lock)
	for _, contents := range [][]byte{[]byte("{}"), append(append([]byte(nil), data...), []byte("{}")...), []byte(strings.Replace(string(data), "func()", "func(int)", 1))} {
		if _, err := Parse(contents); err == nil {
			t.Fatal("invalid lockfile accepted")
		}
	}
	lock.Targets = append(lock.Targets, lock.Targets[0])
	if _, err := Marshal(lock); err == nil {
		t.Fatal("duplicate target accepted")
	}
}

func TestCanonicalOrderingDoesNotMutateCaller(t *testing.T) {
	lock := fixtureLock(t)
	other := lock.Targets[0]
	other.Symbol = "example.com/app.Another"
	lock.Targets = append(lock.Targets, other)
	first, err := Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Targets[0].Symbol != "example.com/app.Run" {
		t.Fatal("marshal mutated caller order")
	}
	lock.Targets[0], lock.Targets[1] = lock.Targets[1], lock.Targets[0]
	second, err := Marshal(lock)
	if err != nil || string(first) != string(second) {
		t.Fatal("target order changed lock bytes")
	}
}

func TestRejectUnpinnedVersionAndUnsafePaths(t *testing.T) {
	for _, version := range []string{"latest", "v1", "v1.1", "PIN_EXACT_OTELC_VERSION"} {
		lock := fixtureLock(t)
		lock.Backend.Version = version
		if _, err := Marshal(lock); err == nil {
			t.Fatalf("unpinned version %s accepted", version)
		}
	}
	for _, name := range []string{"/tmp/local.go", "../local.go", "x/../local.go", `C:\local.go`, "."} {
		lock := fixtureLock(t)
		lock.Targets[0].Location.File = name
		if _, err := Marshal(lock); err == nil {
			t.Fatalf("noncanonical path %s accepted", name)
		}
	}
}

func TestDiffClassifications(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   model.DiffClassification
		change func(*model.Lockfile)
	}{
		{"policy", model.DiffPolicy, func(l *model.Lockfile) { l.PolicyDigest = Digest([]byte("other")) }},
		{"backend", model.DiffBackend, func(l *model.Lockfile) { l.Backend.Version = "v1.2.0" }},
		{"build", model.DiffBuild, func(l *model.Lockfile) { l.GoVersion = "go1.27.1" }},
		{"signature", model.DiffSignature, func(l *model.Lockfile) { l.Targets[0].SignatureDigest = Digest([]byte("other")) }},
		{"span", model.DiffSpanName, func(l *model.Lockfile) { l.Targets[0].SpanName = "other" }},
		{"context", model.DiffContext, func(l *model.Lockfile) { l.Targets[0].Context.Strategy = "argument" }},
		{"errors", model.DiffErrorStrategy, func(l *model.Lockfile) { l.Targets[0].Errors.Record = true }},
		{"safety", model.DiffAttribute, func(l *model.Lockfile) { l.Targets[0].Attributes[0].Allow = true }},
		{"source", model.DiffSource, func(l *model.Lockfile) { l.Targets[0].Location.File = "moved.go" }},
		{"remove", model.DiffRemove, func(l *model.Lockfile) { l.Targets = nil }},
		{"add", model.DiffAdd, func(l *model.Lockfile) {
			target := l.Targets[0]
			target.Symbol = "example.com/app.Other"
			l.Targets = append(l.Targets, target)
		}},
		{"artifact", model.DiffArtifact, func(l *model.Lockfile) { l.Artifacts = []model.ArtifactFile{{Path: "rules.yaml", Digest: Digest(nil)}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, after := fixtureLock(t), fixtureLock(t)
			tc.change(&after)
			diff := Diff(before, after)
			if len(diff.Entries) != 1 || diff.Entries[0].Classification != tc.kind {
				t.Fatalf("unexpected diff: %+v", diff)
			}
		})
	}
}
