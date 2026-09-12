package lockfile

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func Diff(previous, current model.Lockfile) model.LockDiff {
	previous = canonical(previous)
	current = canonical(current)
	diff := model.LockDiff{Entries: []model.LockDiffEntry{}}
	add := func(kind model.DiffClassification, symbol model.SymbolID, detail string) {
		diff.Entries = append(diff.Entries, model.LockDiffEntry{Classification: kind, Symbol: symbol, Detail: detail})
	}
	if previous.PolicyDigest != current.PolicyDigest {
		add(model.DiffPolicy, "", "policy digest changed")
	}
	if !equal(previous.Backend, current.Backend) {
		add(model.DiffBackend, "", "backend identity or capabilities changed")
	}
	if previous.GoVersion != current.GoVersion || previous.ModuleGraphDigest != current.ModuleGraphDigest {
		add(model.DiffBuild, "", "Go toolchain or module graph changed")
	}
	if !equal(previous.Artifacts, current.Artifacts) {
		add(model.DiffArtifact, "", "generated artifact manifest changed")
	}
	old := map[model.SymbolID]model.LockTarget{}
	for _, target := range previous.Targets {
		old[target.Symbol] = target
	}
	for _, target := range current.Targets {
		before, ok := old[target.Symbol]
		if !ok {
			add(model.DiffAdd, target.Symbol, "target added")
			continue
		}
		delete(old, target.Symbol)
		if before.SignatureDigest != target.SignatureDigest {
			add(model.DiffSignature, target.Symbol, "signature changed")
		}
		if before.SourceRule != target.SourceRule {
			add(model.DiffPolicy, target.Symbol, "source rule changed")
		}
		if before.SpanName != target.SpanName {
			add(model.DiffSpanName, target.Symbol, "span name changed")
		}
		if !equal(before.Context, target.Context) {
			add(model.DiffContext, target.Symbol, "context strategy changed")
		}
		if !equal(before.Errors, target.Errors) {
			add(model.DiffErrorStrategy, target.Symbol, "error strategy changed")
		}
		if !equal(before.Attributes, target.Attributes) {
			add(model.DiffAttribute, target.Symbol, "attributes or safety acknowledgment changed")
		}
		if before.Location.File != target.Location.File {
			add(model.DiffSource, target.Symbol, "source file changed")
		}
	}
	for symbol := range old {
		add(model.DiffRemove, symbol, "target removed")
	}
	sort.Slice(diff.Entries, func(i, j int) bool {
		a, b := diff.Entries[i], diff.Entries[j]
		if a.Symbol != b.Symbol {
			return a.Symbol < b.Symbol
		}
		return a.Classification < b.Classification
	})
	return diff
}

func equal(a, b any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}
