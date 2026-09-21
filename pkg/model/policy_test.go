package model_test

import (
	"encoding/json"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestMatchIsEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		empty bool
	}{
		{input: `{}`, empty: true},
		{input: `{"packages":[],"files":null}`, empty: true},
		{input: `{"packages":["app"]}`, empty: false},
		{input: `{"files":["app.go"]}`, empty: false},
		{input: `{"symbols":["app.Run"]}`, empty: false},
		{input: `{"functions":["Run"]}`, empty: false},
		{input: `{"receivers":["Worker"]}`, empty: false},
		{input: `{"methods":["Run"]}`, empty: false},
		{input: `{"implements":["app.Worker"]}`, empty: false},
		{input: `{"exported":false}`, empty: false},
		{input: `{"hasContext":false}`, empty: false},
		{input: `{"returnsError":false}`, empty: false},
		{input: `{"ownership":"application"}`, empty: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.input, func(t *testing.T) {
			t.Parallel()

			var match model.Match

			err := json.Unmarshal([]byte(testCase.input), &match)
			if err != nil {
				t.Fatal(err)
			}

			if match.IsEmpty() != testCase.empty {
				t.Fatalf("IsEmpty() = %v, want %v", match.IsEmpty(), testCase.empty)
			}
		})
	}
}
