// Package policy parses instrumentation policies and validates their schema
// and safety constraints before resolution.
package policy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/DiLRandI/OTelPlan/internal/model"
	"gopkg.in/yaml.v3"
)

var errPolicyDocumentCount = errors.New("parse policy: expected exactly one YAML document")

// Load reads the policy at path. Callers accepting untrusted paths must enforce
// their own permitted filesystem scope.
func Load(path string) (*model.Policy, error) {
	// #nosec G304 -- Load intentionally accepts a caller-selected policy path.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy %s: %w", path, err)
	}

	return Parse(data)
}

// Parse decodes exactly one policy document, rejects unknown fields, and
// applies safe context and error defaults. Validate performs semantic checks.
func Parse(data []byte) (*model.Policy, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var parsed model.Policy

	parsed.Defaults.Context.Mode = model.ContextModeRequire
	parsed.Defaults.Errors.Record = true

	err := dec.Decode(&parsed)
	if err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}

	var trailing any

	err = dec.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		return nil, errPolicyDocumentCount
	}

	return &parsed, nil
}
