package policy_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
)

func TestValidateTemplate(t *testing.T) {
	t.Parallel()

	valid := []string{
		"{{package}}.{{receiver}}.{{method}}",
		"{{symbol}}",
		"static-name",
		"{{import_path}}.{{function}}",
		"",
	}
	for _, tpl := range valid {
		err := policy.ValidateTemplate(tpl)
		if err != nil {
			t.Errorf("ValidateTemplate(%q) = %v, want nil", tpl, err)
		}
	}

	invalid := []string{
		"{{unknown}}",
		"{{package",
		"{{}}",
	}
	for _, tpl := range invalid {
		err := policy.ValidateTemplate(tpl)
		if err == nil {
			t.Errorf("ValidateTemplate(%q) = nil, want error", tpl)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	t.Parallel()

	vars := map[string]string{
		"symbol":      "pkg.(*Recv).Method",
		"package":     "pkg",
		"import_path": "example.com/pkg",
		"function":    "Method",
		"receiver":    "Recv",
		"method":      "Method",
	}

	got, err := policy.RenderTemplate("{{package}}.{{receiver}}.{{method}}", vars)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if got != "pkg.Recv.Method" {
		t.Errorf("render = %q", got)
	}
}
