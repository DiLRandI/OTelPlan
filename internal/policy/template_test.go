package policy

import "testing"

func TestValidateTemplate(t *testing.T) {
	valid := []string{
		"{{package}}.{{receiver}}.{{method}}",
		"{{symbol}}",
		"static-name",
		"{{import_path}}.{{function}}",
		"",
	}
	for _, tpl := range valid {
		if err := ValidateTemplate(tpl); err != nil {
			t.Errorf("ValidateTemplate(%q) = %v, want nil", tpl, err)
		}
	}
	invalid := []string{
		"{{unknown}}",
		"{{package",
		"{{}}",
	}
	for _, tpl := range invalid {
		if err := ValidateTemplate(tpl); err == nil {
			t.Errorf("ValidateTemplate(%q) = nil, want error", tpl)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	vars := map[string]string{
		"symbol":      "pkg.(*Recv).Method",
		"package":     "pkg",
		"import_path": "example.com/pkg",
		"function":    "Method",
		"receiver":    "Recv",
		"method":      "Method",
	}
	got, err := RenderTemplate("{{package}}.{{receiver}}.{{method}}", vars)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != "pkg.Recv.Method" {
		t.Errorf("render = %q", got)
	}
}
