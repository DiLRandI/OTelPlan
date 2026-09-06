package lockfile

import (
	"cmp"
	"slices"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// canonicalPolicy copies only collections whose order is irrelevant to discovery
// and resolution. IDs, attribute sources, and behavior settings remain intact.
func canonicalPolicy(policy model.Policy) model.Policy {
	policy.Project.Packages = canonicalSet(policy.Project.Packages)
	policy.Project.BuildTags = canonicalSet(policy.Project.BuildTags)
	policy.Rules = slices.Clone(policy.Rules)
	for i := range policy.Rules {
		rule := &policy.Rules[i]
		rule.Match = canonicalMatch(rule.Match)
		if rule.Exclude != nil {
			exclude := canonicalMatch(*rule.Exclude)
			rule.Exclude = &exclude
		}
		rule.Attributes = slices.Clone(rule.Attributes)
		slices.SortFunc(rule.Attributes, func(a, b model.AttributeRule) int { return cmp.Compare(a.Key, b.Key) })
	}
	slices.SortFunc(policy.Rules, func(a, b model.Rule) int { return cmp.Compare(a.ID, b.ID) })
	policy.Exclusions = slices.Clone(policy.Exclusions)
	for i := range policy.Exclusions {
		policy.Exclusions[i].Match = canonicalMatch(policy.Exclusions[i].Match)
	}
	slices.SortFunc(policy.Exclusions, func(a, b model.Exclusion) int { return cmp.Compare(a.ID, b.ID) })
	return policy
}

func canonicalMatch(match model.Match) model.Match {
	match.Packages = canonicalSet(match.Packages)
	match.Files = canonicalSet(match.Files)
	match.Symbols = canonicalSet(match.Symbols)
	match.Functions = canonicalSet(match.Functions)
	match.Receivers = canonicalSet(match.Receivers)
	match.Methods = canonicalSet(match.Methods)
	match.Implements = canonicalSet(match.Implements)
	return match
}

func canonicalSet(values []string) []string {
	values = slices.Clone(values)
	slices.Sort(values)
	return slices.Compact(values)
}
