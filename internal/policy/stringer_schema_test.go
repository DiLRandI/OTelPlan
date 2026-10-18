package policy_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestStringerSchemaKeepsConversionOutsideExclusiveSource(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot("../../schemas")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	data, err := root.ReadFile("otelplan.schema.json")
	if err != nil {
		t.Fatal(err)
	}

	var schema struct {
		Definitions map[string]json.RawMessage `json:"$defs"`
	}

	err = json.Unmarshal(data, &schema)
	if err != nil {
		t.Fatal(err)
	}

	var attribute struct {
		Properties map[string]struct {
			Type          string `json:"type"`
			MaxProperties int    `json:"maxProperties"`
		} `json:"properties"`
	}

	err = json.Unmarshal(schema.Definitions["attribute"], &attribute)
	if err != nil {
		t.Fatal(err)
	}

	if attribute.Properties["stringer"].Type != "boolean" || attribute.Properties["from"].MaxProperties != 1 {
		t.Fatal("Stringer schema changed source exclusivity")
	}
}
