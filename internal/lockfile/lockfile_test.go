package lockfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func fixtureLock(t *testing.T) model.Lockfile {
	t.Helper()

	var (
		defaults     model.Defaults
		environment  model.BuildEnvironment
		capabilities model.BackendCapabilities
	)

	policy := &model.Policy{
		APIVersion: "", Kind: "", Project: model.ProjectConfig{
			Packages: nil, BuildTags: nil, IncludeTests: false, IncludeDependencies: false,
		},
		Backend:  model.BackendConfig{Name: "otelc", Version: "v1.1.0"},
		Defaults: defaults, Rules: nil, Exclusions: nil,
	}
	code := &model.CodeModel{
		GoVersion: "go1.27.0", ModuleRoot: "", WorkspaceFile: "", Modules: nil, Packages: nil,
		Types: nil, Implements: nil, InterfaceMethods: nil, CallGraph: nil, CallEdges: nil,
		BuildTags: nil, GOOS: "", GOARCH: "", EffectiveBuild: environment,
		Symbols: []model.Symbol{{
			ID: "example.com/app.Run", Signature: "func()",
			Location: model.SourceLocation{File: "app.go", Line: 1, Column: 0},
			Kind:     "", PackageImportPath: "", PackageName: "", Name: "", Receiver: nil, Visibility: "",
			Parameters: nil, Results: nil, ContextIndexes: nil, ErrorIndexes: nil, Generics: nil,
			Generated: false, TestFile: false, Ownership: "", HasBody: false, Variadic: false,
		}},
	}
	plan := model.ResolvedPlan{
		APIVersion: "", Skipped: nil,
		Targets: []model.ResolvedTarget{{
			SymbolID: code.Symbols[0].ID, Signature: code.Symbols[0].Signature, SpanName: "app.Run", RuleID: "run",
			ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyRoot, Index: 0},
			ErrorStrategy:   model.ErrorStrategy{Record: false, Indexes: nil},
			Attributes: []model.AttributePlan{{
				Key: "category", From: model.AttributeSource{Constant: 1, Argument: "", Result: ""},
				Classification: "", Allow: false,
			}},
		}},
	}
	backend := model.LockBackend{Name: "otelc", Version: "v1.1.0", Digest: "", Capabilities: capabilities}

	lock, err := lockfile.Create(policy, code, plan, backend, lockfile.Digest([]byte("module graph")), nil)
	if err != nil {
		t.Fatal(err)
	}

	return lock
}

func TestLockRoundTripAndWrite(t *testing.T) {
	t.Parallel()

	lock := fixtureLock(t)

	data, err := lockfile.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := lockfile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	again, err := lockfile.Marshal(parsed)
	if err != nil || string(data) != string(again) {
		t.Fatalf("noncanonical roundtrip: %v", err)
	}

	if !lockfile.Diff(lock, parsed).Empty() {
		t.Fatal("numeric decoding caused false diff")
	}
}

func TestLockWrite(t *testing.T) {
	t.Parallel()

	lock := fixtureLock(t)

	data, err := lockfile.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal expected lockfile: %v", err)
	}

	directory := t.TempDir()

	filename := filepath.Join(directory, "otelplan.lock")

	err = lockfile.Write(filename, lock)
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatalf("open lockfile fixture directory: %v", err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Errorf("close lockfile fixture directory: %v", err)
		}
	})

	got, err := root.ReadFile("otelplan.lock")
	if err != nil || string(got) != string(data) {
		t.Fatal("lockfile contents differ")
	}

	entries, err := os.ReadDir(filepath.Dir(filename))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary lockfile leaked")
	}
}

func TestLockRejectsMalformedAndTampered(t *testing.T) {
	t.Parallel()

	lock := fixtureLock(t)

	data, err := lockfile.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal valid lock fixture: %v", err)
	}

	for _, contents := range [][]byte{
		[]byte("{}"),
		append(append([]byte(nil), data...), []byte("{}")...),
		[]byte(strings.Replace(string(data), "func()", "func(int)", 1)),
	} {
		_, err := lockfile.Parse(contents)
		if err == nil {
			t.Fatal("invalid lockfile accepted")
		}
	}

	lock.Targets = append(lock.Targets, lock.Targets[0])

	_, err = lockfile.Marshal(lock)
	if err == nil {
		t.Fatal("duplicate target accepted")
	}
}

func TestCanonicalOrderingDoesNotMutateCaller(t *testing.T) {
	t.Parallel()

	lock := fixtureLock(t)
	other := lock.Targets[0]
	other.Symbol = "example.com/app.Another"
	lock.Targets = append(lock.Targets, other)

	first, err := lockfile.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}

	if lock.Targets[0].Symbol != "example.com/app.Run" {
		t.Fatal("marshal mutated caller order")
	}

	lock.Targets[0], lock.Targets[1] = lock.Targets[1], lock.Targets[0]

	second, err := lockfile.Marshal(lock)
	if err != nil || string(first) != string(second) {
		t.Fatal("target order changed lock bytes")
	}
}

func TestRejectUnpinnedVersionAndUnsafePaths(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"latest", "v1", "v1.1", "PIN_EXACT_OTELC_VERSION"} {
		lock := fixtureLock(t)

		lock.Backend.Version = version

		_, err := lockfile.Marshal(lock)
		if err == nil {
			t.Fatalf("unpinned version %s accepted", version)
		}
	}

	for _, name := range []string{"/tmp/local.go", "../local.go", "x/../local.go", `C:\local.go`, "."} {
		lock := fixtureLock(t)

		lock.Targets[0].Location.File = name

		_, err := lockfile.Marshal(lock)
		if err == nil {
			t.Fatalf("noncanonical path %s accepted", name)
		}
	}
}

func TestDiffClassifications(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		kind   model.DiffClassification
		change func(*model.Lockfile)
	}{
		{"policy", model.DiffPolicy, func(l *model.Lockfile) { l.PolicyDigest = lockfile.Digest([]byte("other")) }},
		{"backend", model.DiffBackend, func(l *model.Lockfile) { l.Backend.Version = "v1.2.0" }},
		{"build", model.DiffBuild, func(l *model.Lockfile) { l.GoVersion = "go1.27.1" }},
		{"signature", model.DiffSignature, func(l *model.Lockfile) {
			l.Targets[0].SignatureDigest = lockfile.Digest([]byte("other"))
		}},
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
		{"artifact", model.DiffArtifact, func(l *model.Lockfile) {
			l.Artifacts = []model.ArtifactFile{{Path: "rules.yaml", Digest: lockfile.Digest(nil)}}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			before, after := fixtureLock(t), fixtureLock(t)
			testCase.change(&after)

			diff := lockfile.Diff(before, after)
			if len(diff.Entries) != 1 || diff.Entries[0].Classification != testCase.kind {
				t.Fatalf("unexpected diff: %+v", diff)
			}
		})
	}
}
