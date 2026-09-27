// Package lockfile records and compares reproducible instrumentation resolution state.
package lockfile

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// Diff reports changes in resolution and build-owned identity in deterministic order.
// Source file changes are significant; line and column changes are ignored.
// Both inputs must be valid lockfiles. Neither input is modified.
func Diff(previous, current model.Lockfile) model.LockDiff {
	previous = canonical(previous)
	current = canonical(current)
	diff := model.LockDiff{Entries: metadataChanges(previous, current)}

	old := map[model.SymbolID]model.LockTarget{}
	for _, target := range previous.Targets {
		old[target.Symbol] = target
	}

	for _, target := range current.Targets {
		before, ok := old[target.Symbol]
		if !ok {
			diff.Entries = append(diff.Entries, model.LockDiffEntry{
				Classification: model.DiffAdd, Symbol: target.Symbol, Detail: "target added",
			})

			continue
		}

		delete(old, target.Symbol)

		diff.Entries = append(diff.Entries, targetChanges(before, target)...)
	}

	for symbol := range old {
		diff.Entries = append(diff.Entries, model.LockDiffEntry{
			Classification: model.DiffRemove, Symbol: symbol, Detail: "target removed",
		})
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

func metadataChanges(previous, current model.Lockfile) []model.LockDiffEntry {
	changes := []model.LockDiffEntry{}

	if previous.PolicyDigest != current.PolicyDigest {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffPolicy, Symbol: "", Detail: "policy digest changed",
		})
	}

	if !equal(previous.Backend, current.Backend) {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffBackend, Symbol: "", Detail: "backend identity or capabilities changed",
		})
	}

	if previous.GoVersion != current.GoVersion || previous.ModuleGraphDigest != current.ModuleGraphDigest {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffBuild, Symbol: "", Detail: "Go toolchain or module graph changed",
		})
	}

	if !equal(previous.Artifacts, current.Artifacts) {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffArtifact, Symbol: "", Detail: "generated artifact manifest changed",
		})
	}

	return changes
}

func targetChanges(before, target model.LockTarget) []model.LockDiffEntry {
	var changes []model.LockDiffEntry

	if before.SignatureDigest != target.SignatureDigest {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffSignature, Symbol: target.Symbol, Detail: "signature changed",
		})
	}

	if before.SourceRule != target.SourceRule {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffPolicy, Symbol: target.Symbol, Detail: "source rule changed",
		})
	}

	if before.SpanName != target.SpanName {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffSpanName, Symbol: target.Symbol, Detail: "span name changed",
		})
	}

	if !equal(before.Context, target.Context) {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffContext, Symbol: target.Symbol, Detail: "context strategy changed",
		})
	}

	if !equal(before.Errors, target.Errors) {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffErrorStrategy, Symbol: target.Symbol, Detail: "error strategy changed",
		})
	}

	if !equal(before.Attributes, target.Attributes) {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffAttribute, Symbol: target.Symbol, Detail: "attributes or safety acknowledgment changed",
		})
	}

	if before.Location.File != target.Location.File {
		changes = append(changes, model.LockDiffEntry{
			Classification: model.DiffSource, Symbol: target.Symbol, Detail: "source file changed",
		})
	}

	return changes
}
