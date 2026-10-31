// Command bench verifies and measures generated business spans.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"testing"

	"example.com/runtimebench/internal/operation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const businessScope = "otelplan.io/business"

const (
	defaultSamples = 3
	usageFailure   = 2
	benchmarkCount = 42
)

var (
	errInvalidSamples   = errors.New("samples must be positive")
	errUnexpectedArgs   = errors.New("unexpected positional arguments")
	errBenchmarkFailure = errors.New("benchmark failure")
	errUnknownOperation = errors.New("unknown operation")
	errInvalidSpan      = errors.New("invalid business span")
)

type options struct {
	instrumented bool
	verifyOnly   bool
	samples      int
}

type operationKind string

const (
	zeroOperation  operationKind = "zero"
	twoOperation   operationKind = "two"
	errorOperation operationKind = "error"
)

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		writeError(err)
		os.Exit(usageFailure)
	}

	err = verifyRuntime(opts.instrumented)
	if err != nil {
		writeError(err)
		os.Exit(1)
	}

	if opts.verifyOnly {
		return
	}

	for _, sampling := range []struct {
		name    string
		sampler sdktrace.Sampler
	}{
		{name: "nonrecording", sampler: sdktrace.NeverSample()},
		{name: "recording", sampler: sdktrace.AlwaysSample()},
	} {
		for _, kind := range []operationKind{zeroOperation, twoOperation, errorOperation} {
			for range opts.samples {
				result, err := runBenchmark(sampling.sampler, kind)
				if err != nil {
					writeError(err)
					os.Exit(1)
				}

				name := fmt.Sprintf("BenchmarkBusinessSpan/%s/%s/%s-%d",
					benchmarkMode(opts.instrumented), sampling.name, kind, runtime.GOMAXPROCS(0))

				line := fmt.Sprintf("%s %s %s\n", name, result.String(), result.MemString())

				_, err = os.Stdout.WriteString(line)
				if err != nil {
					writeError(err)
					os.Exit(1)
				}
			}
		}
	}
}

func parseOptions(args []string) (options, error) {
	var opts options

	flags := flag.NewFlagSet("bench", flag.ContinueOnError)
	flags.BoolVar(&opts.instrumented, "instrumented", false, "run instrumented operations")
	flags.BoolVar(&opts.verifyOnly, "verify-only", false, "verify runtime behavior without benchmarks")
	flags.IntVar(&opts.samples, "samples", defaultSamples, "number of benchmark samples")

	err := flags.Parse(args)
	if err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	if opts.samples <= 0 {
		return options{}, errInvalidSamples
	}

	if flags.NArg() != 0 {
		return options{}, errUnexpectedArgs
	}

	return opts, nil
}

func verifyRuntime(instrumented bool) error {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	tracer := provider.Tracer("runtimebench/parent")
	ctx, parent := tracer.Start(context.Background(), "parent")
	failure := errBenchmarkFailure
	operationErr := verifyOperations(ctx, failure)

	parent.End()

	spanErr := operationErr
	if spanErr == nil {
		spanErr = verifySpans(exporter.GetSpans(), instrumented, parent.SpanContext())
	}

	shutdownErr := provider.Shutdown(context.Background())
	if shutdownErr != nil {
		return fmt.Errorf("shutdown tracing provider: %w", shutdownErr)
	}

	return spanErr
}

func verifyOperations(ctx context.Context, failure error) error {
	err := callOperation(ctx, zeroOperation, failure)
	if err != nil {
		return fmt.Errorf("zero returned: %w", err)
	}

	err = callOperation(ctx, twoOperation, failure)
	if err != nil {
		return fmt.Errorf("two returned: %w", err)
	}

	got := callOperation(ctx, errorOperation, failure)
	if !sameFailure(got, failure) {
		return fmt.Errorf("%w: returned error identity changed", errInvalidSpan)
	}

	return nil
}

func verifySpans(spans []tracetest.SpanStub, instrumented bool, parent trace.SpanContext) error {
	want := 1
	if instrumented {
		want += 3
	}

	if len(spans) != want {
		return fmt.Errorf("%w: span count=%d; want %d", errInvalidSpan, len(spans), want)
	}

	if !instrumented {
		return nil
	}

	err := verifyBusinessRelationships(spans, parent)
	if err != nil {
		return err
	}

	err = verifyBusinessSpan(spans, "business.zero", nil, false)
	if err != nil {
		return err
	}

	err = verifyBusinessSpan(spans, "business.two", []attribute.KeyValue{
		attribute.Bool("operation.enabled", true), attribute.Int64("operation.count", benchmarkCount),
	}, false)
	if err != nil {
		return err
	}

	return verifyBusinessSpan(spans, "business.error", nil, true)
}

func verifyBusinessRelationships(spans []tracetest.SpanStub, parent trace.SpanContext) error {
	for _, span := range spans {
		if span.Name == "parent" {
			continue
		}

		if span.InstrumentationScope.Name != businessScope || span.SpanKind != trace.SpanKindInternal ||
			span.Parent.SpanID() != parent.SpanID() || span.SpanContext.TraceID() != parent.TraceID() {
			return fmt.Errorf("%w: relationship", errInvalidSpan)
		}
	}

	return nil
}

func verifyBusinessSpan(spans []tracetest.SpanStub, name string, attributes []attribute.KeyValue, failed bool) error {
	for _, span := range spans {
		if span.Name != name {
			continue
		}

		if len(span.Attributes) != len(attributes) || len(span.Events) != boolInt(failed) {
			return fmt.Errorf("%w: %s attributes/events mismatch", errInvalidSpan, name)
		}

		err := verifyAttributes(span.Attributes, attributes)
		if err != nil {
			return fmt.Errorf("%w: %s attributes", errInvalidSpan, name)
		}

		if !validOperationStatus(span, failed) {
			return fmt.Errorf("%w: %s operation status", errInvalidSpan, name)
		}

		return nil
	}

	return fmt.Errorf("%w: missing span %s", errInvalidSpan, name)
}

func validOperationStatus(span tracetest.SpanStub, failed bool) bool {
	if failed {
		return len(span.Events) == 1 && span.Events[0].Name == "exception" && span.Status.Code == codes.Error &&
			span.Status.Description == "operation failed"
	}

	return span.Status.Code == codes.Unset && span.Status.Description == ""
}

func verifyAttributes(got, want []attribute.KeyValue) error {
	for _, expected := range want {
		found := false

		for _, actual := range got {
			if actual.Key == expected.Key && actual.Value.AsInterface() == expected.Value.AsInterface() {
				found = true

				break
			}
		}

		if !found {
			return errInvalidSpan
		}
	}

	return nil
}

func runBenchmark(sampler sdktrace.Sampler, kind operationKind) (testing.BenchmarkResult, error) {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler))
	otel.SetTracerProvider(provider)
	tracer := provider.Tracer("runtimebench/parent")
	ctx, parent := tracer.Start(context.Background(), "parent")
	failure := errBenchmarkFailure

	var benchmarkErr error

	result := testing.Benchmark(func(b *testing.B) {
		b.Helper()
		b.ReportAllocs()

		for b.Loop() {
			err := callOperation(ctx, kind, failure)
			if kind == errorOperation {
				if !sameFailure(err, failure) {
					benchmarkErr = errInvalidSpan

					b.StopTimer()

					return
				}
			} else if err != nil {
				benchmarkErr = fmt.Errorf("benchmark operation failed: %w", err)

				b.StopTimer()

				return
			}
		}
	})

	parent.End()

	err := provider.Shutdown(context.Background())
	if err != nil {
		return result, fmt.Errorf("shutdown tracing provider: %w", err)
	}

	return result, benchmarkErr
}

func callOperation(ctx context.Context, kind operationKind, failure error) error {
	switch kind {
	case zeroOperation:
		err := operation.Zero(ctx)
		if err != nil {
			return fmt.Errorf("zero: %w", err)
		}

		return nil
	case twoOperation:
		err := operation.Two(ctx, true, benchmarkCount)
		if err != nil {
			return fmt.Errorf("two: %w", err)
		}

		return nil
	case errorOperation:
		err := operation.Fail(ctx, failure)
		if sameFailure(err, failure) {
			return failure
		}

		if err != nil {
			return fmt.Errorf("error: %w", err)
		}

		return nil
	default:
		return errUnknownOperation
	}
}

func sameFailure(got, want error) bool {
	return got == want
}

func benchmarkMode(instrumented bool) string {
	if instrumented {
		return "instrumented"
	}

	return "baseline"
}

func boolInt(value bool) int {
	if value {
		return 1
	}

	return 0
}

func writeError(err error) {
	_, _ = os.Stderr.WriteString(err.Error() + "\n")
}
