package otelc_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
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

	code, target := accessorRuleFixture(t)
	target.Attributes = []model.AttributePlan{
		accessorAttributePlan("request.id", argumentAttributeSource("req.ID")),
	}
	plan := resolvedPlanWithTargets(target)
	provider := "example.com/generated/accessors"

	rules, files, err := otelc.RenderAccessorRules(otelc.SupportedVersion, code, plan, provider)
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

func assertAccessorRuleInjection(t *testing.T, rules []byte, files []otelc.AccessorFile, provider string) {
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
	files []otelc.AccessorFile,
) {
	t.Helper()

	expected, _, err := otelc.RenderAccessors(code, target)
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
	files []otelc.AccessorFile,
) {
	t.Helper()

	repeatedRules, repeatedFiles, err := otelc.RenderAccessorRules(otelc.SupportedVersion, code, plan, provider)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(rules, repeatedRules) || !reflect.DeepEqual(files, repeatedFiles) {
		t.Fatal("generation is nondeterministic")
	}
}

func TestAccessorRulesRejectInvalidProvider(t *testing.T) {
	t.Parallel()

	code, target := accessorRuleFixture(t)
	target.Attributes = []model.AttributePlan{
		accessorAttributePlan("request.id", argumentAttributeSource("req.ID")),
	}
	plan := resolvedPlanWithTargets(target)

	for _, provider := range []string{"../escape", "example.com/accessorprobe/ops"} {
		rules, files, err := otelc.RenderAccessorRules(otelc.SupportedVersion, code, plan, provider)
		if err == nil || rules != nil || files != nil {
			t.Fatalf("accepted provider %q", provider)
		}
	}
}

func TestAccessorRulesOrderingAndFailure(t *testing.T) {
	t.Parallel()

	code, plan := bundleFixture()
	for i := range plan.Targets {
		plan.Targets[i].Attributes = []model.AttributePlan{
			accessorAttributePlan("component", constantAttributeSource("app")),
		}
	}

	original := append([]model.ResolvedTarget(nil), plan.Targets...)
	provider := "example.com/generated/accessors"

	rules, files, err := otelc.RenderAccessorRules(otelc.SupportedVersion, code, plan, provider)
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
	rules, files, err = otelc.RenderAccessorRules(otelc.SupportedVersion, code, plan, provider)

	if err == nil || rules != nil || files != nil {
		t.Fatal("invalid accessor returned partial output")
	}
}

func TestAccessorRulesSkipTargetsWithoutCaptures(t *testing.T) {
	t.Parallel()

	code, plan := bundleFixture()

	rules, files, err := otelc.RenderAccessorRules(otelc.SupportedVersion, code, plan, "example.com/generated/accessors")
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

func accessorRuleFixture(t *testing.T) (*model.CodeModel, model.ResolvedTarget) {
	t.Helper()

	source := openAccessorRuleRoot(t, "testdata/typed-accessor-contract")
	path := t.TempDir()
	directory := openAccessorRuleRoot(t, path)

	for _, name := range []string{"go.mod", "dep/dep.go", "ops/ops.go"} {
		contents, readErr := source.ReadFile(name)
		if readErr != nil {
			t.Fatalf("read accessor-rule fixture source %s: %v", name, readErr)
		}

		err := directory.MkdirAll(filepath.Dir(name), 0o700)
		if err != nil {
			t.Fatalf("create accessor-rule fixture directory: %v", err)
		}

		err = directory.WriteFile(name, contents, 0o600)
		if err != nil {
			t.Fatalf("write accessor-rule fixture %s: %v", name, err)
		}
	}

	var options discovery.Options

	options.Root, options.Patterns = path, []string{"./ops"}
	options.Env = []string{"GOWORK=off", "GOFLAGS="}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatalf("discover accessor-rule fixture: %v", err)
	}

	symbol, exists := code.Symbol("example.com/accessorprobe/ops.Handle")
	if !exists {
		t.Fatal("accessor-rule fixture symbol missing")
	}

	var target model.ResolvedTarget

	target.SymbolID, target.Signature = symbol.ID, symbol.Signature
	target.ContextStrategy.Strategy = model.ContextStrategyArgument

	return code, target
}

func openAccessorRuleRoot(t *testing.T, path string) *os.Root {
	t.Helper()

	directory, err := os.OpenRoot(path)
	if err != nil {
		t.Fatalf("open accessor-rule fixture root: %v", err)
	}

	t.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Errorf("close accessor-rule fixture root: %v", closeErr)
		}
	})

	return directory
}
