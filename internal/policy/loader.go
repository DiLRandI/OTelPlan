package policy

import (
	"bytes"
	"fmt"
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
	var p model.Policy
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	return &p, nil
}
