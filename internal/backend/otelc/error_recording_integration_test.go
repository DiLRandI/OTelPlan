package otelc_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPinnedBackendErrorRecordingFormattingPanic(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to run the pinned error-recording regression")
	}

	cache := t.TempDir()
	t.Setenv("GOMODCACHE", cache)
	testenv.ConfigureGoCache(t, cache)
	seed := lifecycleWorkspace(t)
	seedCommand := exec.CommandContext(t.Context(), "go", "mod", "download", "all")
	seedCommand.Dir = seed.Name()

	output, seedErr := seedCommand.CombinedOutput()
	if seedErr != nil {
		t.Fatalf("prepare lifecycle dependencies: %v: %s", seedErr, output)
	}

	testenv.DenyNetwork(t)

	_, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	root := lifecycleWorkspace(t)
	writeErrorRecordingSources(t, root)
	code := discoverLifecycleFixture(t, root)
	plan := errorRecordingPlan(t, code)
	writeLifecycleArtifacts(t, root, code, plan)
	original := lifecycleInputs(t, root)
	buildLifecycleProbe(t, root, executable)

	if got := lifecycleInputs(t, root); !maps.Equal(got, original) {
		t.Fatal("backend modified fixture source, manifests, or rules during build")
	}

	result := runErrorRecordingProbe(t, root)
	checkErrorRecordingResult(t, result)

	if got := lifecycleInputs(t, root); !maps.Equal(got, original) {
		t.Fatal("probe modified fixture source, manifests, or rules")
	}
}

func writeErrorRecordingSources(t *testing.T, root *os.Root) {
	t.Helper()
	writeRootFile(t, root, "ops/ops.go", `package ops

import "context"

func Record(ctx context.Context, failure error) error { return failure }
func Disabled(ctx context.Context, failure error) error { return failure }
func BodyPanic(ctx context.Context, failure error) (returned error) {
    returned = failure
    panic("application body panic")
}
`)
	writeRootFile(t, root, "main.go", errorRecordingProbeSource)
}

func errorRecordingPlan(t *testing.T, code *model.CodeModel) model.ResolvedPlan {
	t.Helper()

	var plan model.ResolvedPlan

	for _, name := range []string{"Record", "Disabled", "BodyPanic"} {
		symbol, ok := code.Symbol(model.SymbolID("example.com/probe/ops." + name))
		if !ok {
			t.Fatalf("error recording symbol %q missing", name)
		}

		target := model.ResolvedTarget{
			SymbolID: symbol.ID, Signature: symbol.Signature, SpanName: strings.ToLower(name), RuleID: name,
			ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0},
			ErrorStrategy:   model.ErrorStrategy{Record: false, Indexes: nil}, Attributes: nil,
		}
		if name != "Disabled" {
			target.ErrorStrategy = model.ErrorStrategy{Record: true, Indexes: []int{0}}
		}

		plan.Targets = append(plan.Targets, target)
	}

	return plan
}

func writeRootFile(t *testing.T, root *os.Root, name, contents string) {
	t.Helper()

	err := root.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func lifecycleInputs(t *testing.T, root *os.Root) map[string]string {
	t.Helper()

	inputs := map[string]string{}

	for _, name := range []string{"go.mod", "go.sum", "rules.yaml"} {
		data, err := root.ReadFile(name)
		if err != nil {
			t.Fatalf("read lifecycle input %s: %v", name, err)
		}

		digest := sha256.Sum256(data)

		inputs[name] = hex.EncodeToString(digest[:])
	}

	for name, digest := range lifecycleSources(t, root) {
		inputs[name] = hex.EncodeToString(digest[:])
	}

	return inputs
}

type errorRecordingResult struct {
	Cases     []errorRecordingCase `json:"Cases"`
	BodyPanic string               `json:"BodyPanic"`
}

type errorRecordingCase struct {
	Name         string          `json:"Name"`
	ErrorCalls   int             `json:"ErrorCalls"`
	ReturnedSame bool            `json:"ReturnedSame"`
	Spans        []recordingSpan `json:"Spans"`
}

type recordingSpan struct {
	Name              string            `json:"Name"`
	ID                string            `json:"ID"`
	Parent            string            `json:"Parent"`
	Trace             string            `json:"Trace"`
	Scope             string            `json:"Scope"`
	Version           string            `json:"Version"`
	Status            string            `json:"Status"`
	StatusDescription string            `json:"StatusDescription"`
	Kind              string            `json:"Kind"`
	Events            []recordingEvent  `json:"Events"`
	Attributes        map[string]string `json:"Attributes"`
}

type recordingEvent struct {
	Name       string            `json:"Name"`
	Attributes map[string]string `json:"Attributes"`
}

func runErrorRecordingProbe(t *testing.T, root *os.Root) errorRecordingResult {
	t.Helper()
	command := exec.CommandContext(t.Context(), "./probe")
	command.Dir = root.Name()

	var stderr strings.Builder

	command.Stderr = &stderr

	output, err := command.Output()
	if err != nil {
		t.Fatalf("run error recording probe: %v; stderr=%s; stdout=%s", err, stderr.String(), output)
	}

	if stderr.Len() != 0 || strings.Contains(string(output), "private-formatting-panic") {
		t.Fatalf("error-recording probe emitted private or unexpected output: stderr=%s stdout=%s", &stderr, output)
	}

	var result errorRecordingResult

	err = json.Unmarshal(output, &result)
	if err != nil {
		t.Fatalf("decode error recording probe: %v; stdout=%s", err, output)
	}

	return result
}

func checkErrorRecordingResult(t *testing.T, result errorRecordingResult) {
	t.Helper()

	if result.BodyPanic != "application body panic" {
		t.Fatalf("application-body panic was changed or swallowed: %q", result.BodyPanic)
	}

	wanted := map[string]bool{"normal": false, "panic": false, "disabled": false, "noop": false}

	for index := range result.Cases {
		item := &result.Cases[index]

		seen, known := wanted[item.Name]
		if !known || seen {
			t.Fatalf("unexpected or duplicate error-recording case %q", item.Name)
		}

		wanted[item.Name] = true

		checkErrorRecordingCase(t, item)
	}

	for name, seen := range wanted {
		if !seen {
			t.Fatalf("missing error-recording case %s", name)
		}
	}
}

func checkErrorRecordingCase(t *testing.T, item *errorRecordingCase) {
	t.Helper()

	expectedCalls := 0
	if item.Name == "normal" || item.Name == "panic" {
		expectedCalls = 1
	}

	if !item.ReturnedSame || item.ErrorCalls != expectedCalls {
		t.Fatalf("%s changed error identity or formatting calls: %+v", item.Name, item)
	}

	if item.Name == "noop" {
		if len(item.Spans) != 0 {
			t.Fatalf("noop case exported spans: %+v", item.Spans)
		}

		return
	}

	business := checkErrorRecordingSpans(t, item)
	checkErrorRecordingEvents(t, item.Name, business)
}

func checkErrorRecordingSpans(t *testing.T, item *errorRecordingCase) *recordingSpan {
	t.Helper()

	if len(item.Spans) != 2 {
		t.Fatalf("%s exported %d spans; want exactly root and one business span", item.Name, len(item.Spans))
	}

	businessName := "record"
	if item.Name == "disabled" {
		businessName = "disabled"
	}

	spans := map[string]*recordingSpan{}

	for index := range item.Spans {
		span := &item.Spans[index]
		indexErrorRecordingSpan(t, item.Name, span, businessName, spans)
	}

	parent, business := spans["root"], spans[businessName]
	checkErrorRecordingRelationship(t, item.Name, parent, business)

	return business
}

func indexErrorRecordingSpan(t *testing.T, name string, span *recordingSpan, businessName string,
	spans map[string]*recordingSpan) {
	t.Helper()

	if spans[span.Name] != nil || (span.Name != "root" && span.Name != businessName) ||
		span.ID == "" || span.ID == "0000000000000000" || len(span.Attributes) != 0 {
		t.Fatalf("invalid or duplicate completed span in %s: %+v", name, span)
	}

	spans[span.Name] = span
}

func checkErrorRecordingRelationship(t *testing.T, name string, parent, business *recordingSpan) {
	t.Helper()

	if parent == nil || business == nil {
		t.Fatalf("missing parent or business span in %s", name)
	}

	checkErrorRecordingIdentifiers(t, name, parent, business)

	if business.Kind != "internal" || business.Scope != lifecycleScope || business.Version != lifecycleRuntimeVersion {
		t.Fatalf("invalid business instrumentation in %s: %+v", name, business)
	}

	if parent.Status != "Unset" || len(parent.Events) != 0 {
		t.Fatalf("manual parent changed in %s: %+v", name, parent)
	}
}

func checkErrorRecordingIdentifiers(t *testing.T, name string, parent, business *recordingSpan) {
	t.Helper()

	if parent.ID == business.ID || parent.Parent != "0000000000000000" ||
		parent.Trace == "" || parent.Trace == "00000000000000000000000000000000" ||
		business.Parent != parent.ID || business.Trace != parent.Trace {
		t.Fatalf("invalid %s trace relationship: parent=%+v business=%+v", name, parent, business)
	}
}

func checkErrorRecordingEvents(t *testing.T, name string, business *recordingSpan) {
	t.Helper()

	if name == "disabled" {
		checkDisabledErrorRecording(t, business)

		return
	}

	checkRecordedErrorStatus(t, name, business)

	attributes := business.Events[0].Attributes
	if name == "panic" {
		if !maps.Equal(attributes, map[string]string{"exception.message": "error message unavailable"}) {
			t.Fatalf("formatting panic did not produce a bounded exception: %+v", attributes)
		}
	} else if attributes["exception.message"] != "normal failure" || attributes["exception.type"] == "" {
		t.Fatalf("normal SDK error event changed: %+v", attributes)
	}
}

func checkDisabledErrorRecording(t *testing.T, business *recordingSpan) {
	t.Helper()

	if business.Status != "Unset" || business.StatusDescription != "" || len(business.Events) != 0 {
		t.Fatalf("disabled error recording changed status or events: %+v", business)
	}
}

func checkRecordedErrorStatus(t *testing.T, name string, business *recordingSpan) {
	t.Helper()

	if business.Status != "Error" || business.StatusDescription != "operation failed" ||
		len(business.Events) != 1 || business.Events[0].Name != "exception" {
		t.Fatalf("invalid %s error recording: %+v", name, business)
	}
}

const errorRecordingProbeSource = `package main

import (
    "context"
    "encoding/json"
    "errors"
    "os"
    "sync/atomic"

    "example.com/probe/ops"
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    "go.opentelemetry.io/otel/sdk/trace/tracetest"
    "go.opentelemetry.io/otel/trace/noop"
)

type formattingError struct { calls atomic.Int32; panicOn bool }
func (e *formattingError) Error() string {
    e.calls.Add(1)
    if e.panicOn { panic("private-formatting-panic") }
    return "normal failure"
}

type errorRecordingResult struct { Cases []errorRecordingCase; BodyPanic string }
type errorRecordingCase struct {
    Name string
    ErrorCalls int
    ReturnedSame bool
    Spans []recordingSpan
}
type recordingSpan struct {
    Name string
    ID string
    Parent string
    Trace string
    Scope string
    Version string
    Status string
    StatusDescription string
    Kind string
    Events []recordingEvent
    Attributes map[string]string
}
type recordingEvent struct { Name string; Attributes map[string]string }

func main() {
    results := []errorRecordingCase{
        runCase("normal", false, false), runCase("panic", true, false),
        runCase("disabled", false, true), runNoopCase(),
    }
    result := errorRecordingResult{Cases: results, BodyPanic: checkBodyPanic()}
    if err := json.NewEncoder(os.Stdout).Encode(result); err != nil { panic(err) }
}

func checkBodyPanic() (value string) {
    defer func() {
        if raised := recover(); raised != nil {
            value, _ = raised.(string)
        }
    }()
    ops.BodyPanic(context.Background(), &formattingError{panicOn: true})
    return "not raised"
}

func runCase(name string, panicOn, disabled bool) errorRecordingCase {
    exporter := tracetest.NewInMemoryExporter()
    provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
    otel.SetTracerProvider(provider)
    expected := &formattingError{panicOn: panicOn}
    ctx, parent := otel.Tracer("probe").Start(context.Background(), "root")
    var returned error
    if disabled { returned = ops.Disabled(ctx, expected) } else { returned = ops.Record(ctx, expected) }
    parent.End()
    return collectCase(name, expected, returned, exporter.GetSpans())
}

func runNoopCase() errorRecordingCase {
    otel.SetTracerProvider(noop.NewTracerProvider())
    expected := &formattingError{}
    returned := ops.Record(context.Background(), expected)
    return collectCase("noop", expected, returned, nil)
}

func collectCase(name string, expected, returned error, spans []tracetest.SpanStub) errorRecordingCase {
    typed, ok := returned.(*formattingError)
    exact := ok && typed != nil && errors.Is(returned, expected)
    result := errorRecordingCase{
        Name: name, ErrorCalls: int(expected.(*formattingError).calls.Load()),
        ReturnedSame: exact,
    }
    for _, span := range spans {
        item := recordingSpan{
            Name: span.Name, ID: span.SpanContext.SpanID().String(), Parent: span.Parent.SpanID().String(),
            Trace: span.SpanContext.TraceID().String(), Scope: span.InstrumentationScope.Name,
            Version: span.InstrumentationScope.Version, Status: span.Status.Code.String(),
            StatusDescription: span.Status.Description, Kind: span.SpanKind.String(),
            Attributes: map[string]string{},
        }
        for _, attribute := range span.Attributes {
            item.Attributes[string(attribute.Key)] = attribute.Value.AsString()
        }
        for _, event := range span.Events {
            attrs := map[string]string{}
            for _, attribute := range event.Attributes {
                attrs[string(attribute.Key)] = attribute.Value.AsString()
            }
            item.Events = append(item.Events, recordingEvent{Name: event.Name, Attributes: attrs})
        }
        result.Spans = append(result.Spans, item)
    }
    return result
}
`
