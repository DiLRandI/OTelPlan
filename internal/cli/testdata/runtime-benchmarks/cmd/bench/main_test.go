package main

import (
	"errors"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var errSameTextFailure = errors.New("benchmark failure")

func TestSameFailurePreservesIdentity(t *testing.T) {
	t.Parallel()

	original := errBenchmarkFailure
	for _, test := range []struct {
		name string
		got  error
		want bool
	}{
		{name: "same instance", got: original, want: true},
		{name: "wrapped instance", got: fmt.Errorf("wrapped: %w", original), want: false},
		{name: "same message", got: errSameTextFailure, want: false},
		{name: "missing failure", got: nil, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if sameFailure(test.got, original) != test.want {
				t.Fatalf("error identity check for %s = %t, want %t", test.name,
					sameFailure(test.got, original), test.want)
			}
		})
	}
}

func TestVerifierRejectsErrorStatusOnSuccessfulOperation(t *testing.T) {
	t.Parallel()

	var span tracetest.SpanStub

	span.Name = "business.zero"
	span.Status.Code = codes.Error
	span.Status.Description = "unexpected failure"

	err := verifyBusinessSpan([]tracetest.SpanStub{span}, span.Name, nil, false)
	if err == nil {
		t.Fatal("successful operation with error span status passed preflight")
	}
}
