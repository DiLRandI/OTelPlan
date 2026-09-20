package main

import (
	"context"
	"encoding/json"
	"os"

	"example.com/architecture/entry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func main() {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	ctx, root := provider.Tracer("fixture").Start(context.Background(), "root")
	err := entry.Run(ctx, "private-token-never-capture")
	root.End()
	type span struct {
		Name, ID, Parent, Trace, Scope, Kind string
		Error                                bool
		Events                               int
		Attributes                           map[string]any
	}
	spans := make([]span, 0, len(exporter.GetSpans()))
	for _, item := range exporter.GetSpans() {
		attributes := map[string]any{}
		for _, attribute := range item.Attributes {
			attributes[string(attribute.Key)] = attribute.Value.AsInterface()
		}
		spans = append(spans, span{Name: item.Name, ID: item.SpanContext.SpanID().String(), Parent: item.Parent.SpanID().String(), Trace: item.SpanContext.TraceID().String(), Scope: item.InstrumentationScope.Name, Kind: item.SpanKind.String(), Error: item.Status.Code.String() == "Error", Events: len(item.Events), Attributes: attributes})
	}
	returned := ""
	if err != nil {
		returned = err.Error()
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		ReturnedError string
		Spans         []span
	}{returned, spans}); err != nil {
		panic(err)
	}
}
