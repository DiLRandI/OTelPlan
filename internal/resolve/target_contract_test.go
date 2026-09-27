package resolve_test

import (
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestResolveTargetAttributeOrderingAndErrorOwnership(t *testing.T) {
	t.Parallel()

	policy, code := resolverInputs()
	policy.Rules[0].Attributes = []model.AttributeRule{
		{
			Key: "z.kind", From: model.AttributeSource{Argument: "", Result: "", Constant: "operation"},
			Safety: &model.Safety{Classification: model.ClassificationInternal, Allow: true},
		},
		{
			Key: "a.version", From: model.AttributeSource{Argument: "", Result: "", Constant: "v1"},
			Safety: nil,
		},
	}

	result := resolve.Resolve(policy, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 1 {
		t.Fatalf("expected one valid target: %+v", result)
	}

	target := result.Plan.Targets[0]

	want := []model.AttributePlan{
		{
			Key: "a.version", From: policy.Rules[0].Attributes[1].From,
			Classification: "", Allow: false,
		},
		{
			Key: "z.kind", From: policy.Rules[0].Attributes[0].From,
			Classification: model.ClassificationInternal, Allow: true,
		},
	}
	if !reflect.DeepEqual(target.Attributes, want) {
		t.Fatalf("target attributes = %+v; want %+v", target.Attributes, want)
	}

	if policy.Rules[0].Attributes[0].Key != "z.kind" {
		t.Fatal("resolving sorted the caller's policy attributes")
	}

	if !reflect.DeepEqual(target.ErrorStrategy.Indexes, []int{0}) {
		t.Fatalf("error indexes = %v; want [0]", target.ErrorStrategy.Indexes)
	}

	target.ErrorStrategy.Indexes[0] = 1

	if code.Symbols[0].ErrorIndexes[0] != 0 {
		t.Fatal("resolved error indexes alias the caller's code model")
	}
}
