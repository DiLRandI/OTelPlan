package policy

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errUnterminatedTemplateVariable = errors.New("unterminated template variable")
	errUnknownTemplateVariable      = errors.New("unknown template variable")
)

// ValidateTemplate rejects unterminated or unknown template variables.
func ValidateTemplate(tpl string) error {
	for {
		open := strings.Index(tpl, "{{")
		if open < 0 {
			return nil
		}

		closeRel := strings.Index(tpl[open:], "}}")
		if closeRel < 0 {
			return fmt.Errorf("%w at %q", errUnterminatedTemplateVariable, tpl[open:])
		}

		name := strings.TrimSpace(tpl[open+2 : open+closeRel])
		switch name {
		case "symbol", "package", "import_path", "function", "receiver", "method":
		default:
			return fmt.Errorf("%w %q", errUnknownTemplateVariable, name)
		}

		tpl = tpl[open+closeRel+2:]
	}
}

// RenderTemplate substitutes supported variables after validating tpl.
// Supported variables absent from vars are replaced by an empty string.
func RenderTemplate(tpl string, vars map[string]string) (string, error) {
	err := ValidateTemplate(tpl)
	if err != nil {
		return "", err
	}

	var rendered strings.Builder

	rest := tpl

	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			rendered.WriteString(rest)

			return rendered.String(), nil
		}

		closeRel := strings.Index(rest[open:], "}}")
		name := strings.TrimSpace(rest[open+2 : open+closeRel])
		rendered.WriteString(rest[:open])
		rendered.WriteString(vars[name])

		rest = rest[open+closeRel+2:]
	}
}
