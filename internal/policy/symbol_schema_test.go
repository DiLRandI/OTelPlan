package policy_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestSymbolAttributeSchemaMatchesPolicy(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot("../../schemas")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

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
			MaxProperties int `json:"maxProperties"`
			Properties    map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"properties"`
	}

	err = json.Unmarshal(schema.Definitions["attribute"], &attribute)
	if err != nil {
		t.Fatal(err)
	}

	from := attribute.Properties["from"]

	fields := from.Properties["symbol"].Enum
	if from.MaxProperties != 1 || len(fields) != 7 {
		t.Fatal("symbol source schema changed exclusivity or field vocabulary")
	}

	for _, field := range []string{"id", "package", "packageName", "name", "receiver", "signature", "kind"} {
		if !slices.Contains(fields, field) {
			t.Fatalf("symbol source schema omits %s", field)
		}
	}
}
