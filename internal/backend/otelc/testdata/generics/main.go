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
	checkReturnBehavior()
	checkPanicBehavior()

	type span struct {
		Name       string         `json:"name"`
		Parent     string         `json:"parent"`
		Error      bool           `json:"error"`
		Events     int            `json:"events"`
		Attributes map[string]any `json:"attributes"`
	}
	spans := make([]span, 0, len(exporter.GetSpans()))
	for _, item := range exporter.GetSpans() {
		attributes := map[string]any{}
		for _, attribute := range item.Attributes {
			attributes[string(attribute.Key)] = attribute.Value.AsInterface()
		}
		spans = append(spans, span{
			Name: item.Name, Parent: item.Parent.SpanID().String(),
			Error: item.Status.Code.String() == "Error", Events: len(item.Events), Attributes: attributes,
		})
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Spans []span `json:"spans"`
	}{Spans: spans}); err != nil {
		panic(err)
	}
}

func checkReturnBehavior() {
	value, err := ops.Transform(7, false)
	if value != 7 || err != nil {
		panic("generic function success behavior changed")
	}
	text, err := ops.Transform("private-input-marker", true)
	if text != "private-input-marker" || err == nil {
		panic("generic function error behavior changed")
	}
	store := ops.Store[int]{}
	value, err = store.Apply(9, false)
	if value != 9 || err != nil {
		panic("generic method success behavior changed")
	}
	value, err = store.Apply(11, true)
	if value != 11 || err == nil {
		panic("generic method error behavior changed")
	}
	values, err := ops.Batch([]string{"private-input-marker"})
	if len(values) != 1 || values[0] != "private-input-marker" || err != nil {
		panic("generic composite result behavior changed")
	}
	var pointer *ops.Store[int]
	value, err = pointer.PointerApply(13)
	if value != 13 || err != nil {
		panic("generic nil-receiver behavior changed")
	}
}

func checkPanicBehavior() {
	defer func() {
		if recover() != "original generic panic" {
			panic("generic application panic was changed or swallowed")
		}
	}()
	ops.Panic(1)
}
