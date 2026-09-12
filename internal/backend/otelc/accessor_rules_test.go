package otelc

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"gopkg.in/yaml.v3"
)

func TestAccessorRules(t *testing.T) {
	_, code, target := accessorFixture(t)
	target.Attributes = []model.AttributePlan{{Key: "request.id", From: model.AttributeSource{Argument: "req.ID"}}}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{target}}
	rules, files, err := RenderAccessorRules(SupportedVersion, code, plan, "example.com/generated/accessors")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files: %d", len(files))
	}
	var document map[string]struct {
		Target  string `yaml:"target"`
		Actions []struct {
			AddFile struct{ File, Path string } `yaml:"add_file"`
		} `yaml:"do"`
	}
	if err := yaml.Unmarshal(rules, &document); err != nil {
		t.Fatal(err)
	}
	if len(document) != 1 {
		t.Fatalf("rules: %s", rules)
	}
	for _, rule := range document {
		if rule.Target != "example.com/accessorprobe/ops" || len(rule.Actions) != 1 || rule.Actions[0].AddFile.File != files[0].Name || rule.Actions[0].AddFile.Path != "example.com/generated/accessors" {
			t.Fatalf("wrong injection: %+v", rule)
		}
	}
	expected, _, err := RenderAccessors(code, target)
	if err != nil || !bytes.Equal(expected, files[0].Source) {
		t.Fatalf("helper source differs: %v", err)
	}
	repeatedRules, repeatedFiles, err := RenderAccessorRules(SupportedVersion, code, plan, "example.com/generated/accessors")
	if err != nil || !bytes.Equal(rules, repeatedRules) || !reflect.DeepEqual(files, repeatedFiles) {
		t.Fatal("generation is nondeterministic")
	}
}

func TestAccessorRulesRejectInvalidProvider(t *testing.T) {
	_, code, target := accessorFixture(t)
	target.Attributes = []model.AttributePlan{{Key: "request.id", From: model.AttributeSource{Argument: "req.ID"}}}
	for _, provider := range []string{"../escape", "example.com/accessorprobe/ops"} {
		rules, files, err := RenderAccessorRules(SupportedVersion, code, model.ResolvedPlan{Targets: []model.ResolvedTarget{target}}, provider)
		if err == nil || rules != nil || files != nil {
			t.Fatalf("accepted provider %q", provider)
		}
	}
}

func TestAccessorRulesOrderingAndFailure(t *testing.T) {
	code, plan := ruleFixture()
	for i := range plan.Targets {
		plan.Targets[i].Attributes = []model.AttributePlan{{Key: "component", From: model.AttributeSource{Constant: "app"}}}
	}
	original := append([]model.ResolvedTarget(nil), plan.Targets...)
	rules, files, err := RenderAccessorRules(SupportedVersion, code, plan, "example.com/generated/accessors")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || !reflect.DeepEqual(original, plan.Targets) {
		t.Fatal("missing helpers or mutated plan")
	}
	plan.Targets[0], plan.Targets[2] = plan.Targets[2], plan.Targets[0]
	reordered, reorderedFiles, err := RenderAccessorRules(SupportedVersion, code, plan, "example.com/generated/accessors")
	if err != nil || !bytes.Equal(rules, reordered) || !reflect.DeepEqual(files, reorderedFiles) {
		t.Fatal("target order changed output")
	}
	plan.Targets[0].Attributes = []model.AttributePlan{{Key: "bad", From: model.AttributeSource{Argument: "missing"}}}
	rules, files, err = RenderAccessorRules(SupportedVersion, code, plan, "example.com/generated/accessors")
	if err == nil || rules != nil || files != nil {
		t.Fatal("invalid accessor returned partial output")
	}
}

func TestAccessorRulesSkipTargetsWithoutCaptures(t *testing.T) {
	code, plan := ruleFixture()
	rules, files, err := RenderAccessorRules(SupportedVersion, code, plan, "example.com/generated/accessors")
	if err != nil || len(files) != 0 || string(rules) != "{}\n" {
		t.Fatalf("unexpected empty output: %s, %v", rules, err)
	}
}
