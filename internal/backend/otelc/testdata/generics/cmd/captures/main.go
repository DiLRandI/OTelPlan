package main

import (
	"encoding/json"
	"os"

	"example.com/genericprobe/ops"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func main() {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	req := ops.NewCaptureRequest()
	result, err := ops.Capture("private-input-marker", req, false)
	if err != nil || result.Message != "approved-output" {
		panic("generic capture changed function results")
	}
	result, err = ops.Capture("private-input-marker", nil, true)
	if err != nil || result.Empty != "" {
		panic("generic capture changed nil-input results")
	}
	store := ops.Store[string]{}
	zero, result, err := store.Capture(req, true)
	if zero != "" || err != nil || result.Message != "approved-output" {
		panic("generic capture changed method results")
	}
	zero, result, err = store.Capture(nil, false)
	if zero != "" || err != nil || result.Empty != "" {
		panic("generic capture changed nil method-input results")
	}

	type span struct {
		Name       string         `json:"name"`
		Attributes map[string]any `json:"attributes"`
	}
	spans := make([]span, 0, len(exporter.GetSpans()))
	for _, item := range exporter.GetSpans() {
		attributes := map[string]any{}
		for _, attribute := range item.Attributes {
			attributes[string(attribute.Key)] = attribute.Value.AsInterface()
		}
		spans = append(spans, span{Name: item.Name, Attributes: attributes})
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Spans []span `json:"spans"`
	}{Spans: spans}); err != nil {
		panic(err)
	}
}
