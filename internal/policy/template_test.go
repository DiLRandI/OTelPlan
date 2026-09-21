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

func TestRenderTemplateContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		template string
		output   string
		message  string
	}{
		{template: "{{ package }} / {{function}} / {{package}}", output: "shop /  / shop", message: ""},
		{template: "prefix {{unknown}}", output: "", message: `unknown template variable "unknown"`},
		{template: "prefix {{package", output: "", message: `unterminated template variable at "{{package"`},
	}

	for _, testCase := range cases {
		t.Run(testCase.template, func(t *testing.T) {
			t.Parallel()

			output, err := policy.RenderTemplate(testCase.template, map[string]string{"package": "shop"})
			if testCase.message == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != testCase.message {
				t.Fatalf("error = %v, want %q", err, testCase.message)
			}

			if output != testCase.output {
				t.Fatalf("output = %q, want %q", output, testCase.output)
			}
		})
	}
}
