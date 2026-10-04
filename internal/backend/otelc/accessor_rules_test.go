package otelc

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"gopkg.in/yaml.v3"
)

type decodedAccessorRules map[string]struct {
	Target  string `yaml:"target"`
	Actions []struct {
		AddFile struct {
			File string `yaml:"file"`
			Path string `yaml:"path"`
		} `yaml:"add_file"`
	} `yaml:"do"`
}

func TestAccessorRules(t *testing.T) {
	t.Parallel()

	_, code, target := accessorFixture(t)
	target.Attributes = []model.AttributePlan{
		accessorAttributePlan("request.id", argumentAttributeSource("req.ID")),
	}
	plan := resolvedPlanWithTargets(target)
	provider := "example.com/generated/accessors"

	rules, files, err := RenderAccessorRules(SupportedVersion, code, plan, provider)
	if err != nil {
		t.Fatal(err)
	}

	assertAccessorRuleInjection(t, rules, files, provider)
	assertAccessorRuleHelperSource(t, code, target, files)
	assertAccessorRuleDeterminism(t, code, plan, provider, rules, files)
}

func accessorAttributePlan(key string, source model.AttributeSource) model.AttributePlan {
	var attribute model.AttributePlan

	attribute.Key = key
	attribute.From = source

	return attribute
}

func argumentAttributeSource(argument string) model.AttributeSource {
	var source model.AttributeSource

	source.Argument = argument

	return source
}

func constantAttributeSource(value string) model.AttributeSource {
	var source model.AttributeSource

	source.Constant = value

	return source
}

func resolvedPlanWithTargets(targets ...model.ResolvedTarget) model.ResolvedPlan {
	var plan model.ResolvedPlan

	plan.Targets = targets

	return plan
}

func assertAccessorRuleInjection(t *testing.T, rules []byte, files []AccessorFile, provider string) {
	t.Helper()

	if len(files) != 1 {
		t.Fatalf("files: %d", len(files))
	}

	var document decodedAccessorRules

	err := yaml.Unmarshal(rules, &document)
	if err != nil {
		t.Fatal(err)
	}

	if len(document) != 1 {
		t.Fatalf("rules: %s", rules)
	}

	for _, rule := range document {
		if rule.Target != "example.com/accessorprobe/ops" {
			t.Fatalf("wrong injection target: %q", rule.Target)
		}

		if len(rule.Actions) != 1 {
			t.Fatalf("injection actions: %d", len(rule.Actions))
		}

		if rule.Actions[0].AddFile.File != files[0].Name {
			t.Fatalf("injected file: %q, want %q", rule.Actions[0].AddFile.File, files[0].Name)
		}

		if rule.Actions[0].AddFile.Path != provider {
			t.Fatalf("injection path: %q, want %q", rule.Actions[0].AddFile.Path, provider)
		}
	}
}

func assertAccessorRuleHelperSource(
	t *testing.T,
	code *model.CodeModel,
	target model.ResolvedTarget,
	files []AccessorFile,
) {
	t.Helper()

	expected, _, err := RenderAccessors(code, target)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected, files[0].Source) {
		t.Fatal("helper source differs")
	}
}

func assertAccessorRuleDeterminism(
	t *testing.T,
	code *model.CodeModel,
	plan model.ResolvedPlan,
	provider string,
	rules []byte,
	files []AccessorFile,
) {
	t.Helper()

	repeatedRules, repeatedFiles, err := RenderAccessorRules(SupportedVersion, code, plan, provider)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(rules, repeatedRules) || !reflect.DeepEqual(files, repeatedFiles) {
		t.Fatal("generation is nondeterministic")
	}
}

func TestAccessorRulesRejectInvalidProvider(t *testing.T) {
	t.Parallel()

	_, code, target := accessorFixture(t)
	target.Attributes = []model.AttributePlan{
		accessorAttributePlan("request.id", argumentAttributeSource("req.ID")),
	}
	plan := resolvedPlanWithTargets(target)

	for _, provider := range []string{"../escape", "example.com/accessorprobe/ops"} {
		rules, files, err := RenderAccessorRules(SupportedVersion, code, plan, provider)
		if err == nil || rules != nil || files != nil {
			t.Fatalf("accepted provider %q", provider)
		}
	}
}

func TestAccessorRulesOrderingAndFailure(t *testing.T) {
	t.Parallel()

	code, plan := ruleFixture()
	for i := range plan.Targets {
		plan.Targets[i].Attributes = []model.AttributePlan{
			accessorAttributePlan("component", constantAttributeSource("app")),
		}
	}

	original := append([]model.ResolvedTarget(nil), plan.Targets...)
	provider := "example.com/generated/accessors"

	rules, files, err := RenderAccessorRules(SupportedVersion, code, plan, provider)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 3 {
		t.Fatalf("helper files: %d, want 3", len(files))
	}

	if !reflect.DeepEqual(original, plan.Targets) {
		t.Fatal("render mutated plan")
	}

	plan.Targets[0], plan.Targets[2] = plan.Targets[2], plan.Targets[0]
	assertAccessorRuleDeterminism(t, code, plan, provider, rules, files)

	plan.Targets[0].Attributes = []model.AttributePlan{
		accessorAttributePlan("bad", argumentAttributeSource("missing")),
	}
	rules, files, err = RenderAccessorRules(SupportedVersion, code, plan, provider)

	if err == nil || rules != nil || files != nil {
		t.Fatal("invalid accessor returned partial output")
	}
}

func TestAccessorRulesSkipTargetsWithoutCaptures(t *testing.T) {
	t.Parallel()

	code, plan := ruleFixture()

	rules, files, err := RenderAccessorRules(SupportedVersion, code, plan, "example.com/generated/accessors")
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 0 {
		t.Fatalf("helper files: %d, want 0", len(files))
	}

	if string(rules) != "{}\n" {
		t.Fatalf("unexpected empty output: %s", rules)
	}
}
