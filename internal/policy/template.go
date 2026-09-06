package policy

import (
	"fmt"
	"strings"
)

var templateVars = map[string]bool{
	"symbol":      true,
	"package":     true,
	"import_path": true,
	"function":    true,
	"receiver":    true,
	"method":      true,
}

func ValidateTemplate(tpl string) error {
	for {
		open := strings.Index(tpl, "{{")
		if open < 0 {
			return nil
		}
		closeRel := strings.Index(tpl[open:], "}}")
		if closeRel < 0 {
			return fmt.Errorf("unterminated template variable at %q", tpl[open:])
		}
		name := strings.TrimSpace(tpl[open+2 : open+closeRel])
		if !templateVars[name] {
			return fmt.Errorf("unknown template variable %q", name)
		}
		tpl = tpl[open+closeRel+2:]
	}
}

func RenderTemplate(tpl string, vars map[string]string) (string, error) {
	if err := ValidateTemplate(tpl); err != nil {
		return "", err
	}
	var b strings.Builder
	rest := tpl
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			b.WriteString(rest)
			return b.String(), nil
		}
		closeRel := strings.Index(rest[open:], "}}")
		name := strings.TrimSpace(rest[open+2 : open+closeRel])
		b.WriteString(rest[:open])
		b.WriteString(vars[name])
		rest = rest[open+closeRel+2:]
	}
}
