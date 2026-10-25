package policy_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestErrorTypeAttributeSchemaIsExclusiveNonEmptyString(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../schemas/otelplan.schema.json")
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
				Type      string `json:"type"`
				MinLength int    `json:"minLength"`
			} `json:"properties"`
		} `json:"properties"`
	}

	err = json.Unmarshal(schema.Definitions["attribute"], &attribute)
	if err != nil {
		t.Fatal(err)
	}

	from := attribute.Properties["from"]
	if from.MaxProperties != 1 {
		t.Fatalf("from.maxProperties = %d, want 1", from.MaxProperties)
	}

	errorType, ok := from.Properties["errorType"]
	if !ok {
		t.Fatal("schema omitted from.errorType")
	}

	if errorType.Type != "string" || errorType.MinLength != 1 {
		t.Fatalf("errorType schema = %+v, want nonempty string", errorType)
	}
}
