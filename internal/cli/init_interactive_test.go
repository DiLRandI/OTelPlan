package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/policy"
)

func TestInteractiveInitSelectsReviewedSuggestions(t *testing.T) {
	t.Parallel()

	root, _, _ := callGraphFixture(t)

	var stdout, stderr bytes.Buffer

	args := []string{"init", "--root", root, "--offline", "--interactive"}

	input := strings.NewReader("maybe\nyes\nno\n")
	if exit := cli.RunWithInput(t.Context(), args, input, &stdout, &stderr); exit != 0 {
		t.Fatalf("interactive init exit=%d: %s %s", exit, &stdout, &stderr)
	}

	if !strings.Contains(stderr.String(), "example.com/app.Run") ||
		!strings.Contains(stderr.String(), "has one context.Context argument") ||
		!strings.Contains(stderr.String(), "[y/N]") ||
		!strings.Contains(stderr.String(), "Enter y or n.") {
		t.Fatalf("prompt did not explain the suggested target: %s", &stderr)
	}

	generated, err := policy.Load(filepath.Join(root, "otelplan.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if len(generated.Rules) != 1 || generated.Rules[0].Match.Symbols[0] != "example.com/app.Run" {
		t.Fatalf("interactive choices were not respected: %+v", generated.Rules)
	}
}

func TestInteractiveInitDeclineAndInputErrorsDoNotWrite(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		input string
	}{
		{name: "declined", input: "no\nn\n"},
		{name: "missing input", input: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root, _, _ := callGraphFixture(t)

			var stdout, stderr bytes.Buffer

			args := []string{"init", "--root", root, "--offline", "--interactive"}
			if exit := cli.RunWithInput(t.Context(), args, strings.NewReader(testCase.input), &stdout, &stderr); exit == 0 {
				t.Fatalf("interactive init unexpectedly succeeded: %s %s", &stdout, &stderr)
			}

			_, err := os.Stat(filepath.Join(root, "otelplan.yaml"))
			if !os.IsNotExist(err) {
				t.Fatal("rejected interactive init wrote a policy")
			}
		})
	}
}

func TestInteractiveInitRejectsIncompatibleModes(t *testing.T) {
	t.Parallel()

	var noInputOut, noInputErr bytes.Buffer

	if exit := cli.Run(t.Context(), []string{"init", "--interactive"}, &noInputOut, &noInputErr); exit != 2 {
		t.Fatalf("interactive init without input exit=%d: %s %s", exit, &noInputOut, &noInputErr)
	}

	for _, args := range [][]string{
		{"init", "--interactive", "--non-interactive"},
		{"init", "--interactive", "--format=json"},
		{"scan", "--interactive"},
	} {
		var stdout, stderr bytes.Buffer

		if exit := cli.RunWithInput(t.Context(), args, strings.NewReader("yes\n"), &stdout, &stderr); exit != 2 {
			t.Fatalf("invalid interactive usage %v exit=%d: %s %s", args, exit, &stdout, &stderr)
		}
	}
}

func TestInteractiveInitCanBeCanceledWhileWaitingForInput(t *testing.T) {
	t.Parallel()

	root, _, _ := callGraphFixture(t)

	input, inputWriter := io.Pipe()
	defer func() { _ = inputWriter.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var stdout bytes.Buffer

	prompts := &promptSignal{ready: make(chan struct{}), once: sync.Once{}}

	done := make(chan int, 1)
	go func() {
		done <- cli.RunWithInput(ctx, []string{"init", "--root", root, "--offline", "--interactive"},
			input, &stdout, prompts)
	}()

	select {
	case <-prompts.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("interactive prompt was not shown")
	}

	cancel()

	select {
	case exit := <-done:
		if exit == 0 {
			t.Fatalf("canceled interactive init succeeded: %s", &stdout)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("interactive input blocked cancellation")
	}

	_, err := os.Stat(filepath.Join(root, "otelplan.yaml"))
	if !os.IsNotExist(err) {
		t.Fatal("canceled interactive init wrote a policy")
	}
}

type promptSignal struct {
	ready chan struct{}
	once  sync.Once
}

func (signal *promptSignal) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("[y/N]")) {
		signal.once.Do(func() { close(signal.ready) })
	}

	return len(data), nil
}
