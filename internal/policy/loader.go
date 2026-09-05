package policy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"gopkg.in/yaml.v3"
)

func Load(path string) (*model.Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy %s: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (*model.Policy, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	p := model.Policy{Defaults: model.Defaults{Context: model.ContextDefaults{Mode: model.ContextModeRequire}, Errors: model.ErrorDefaults{Record: true}}}
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse policy: expected exactly one YAML document")
	}
	return &p, nil
}
