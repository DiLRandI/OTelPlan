package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestNATSEntrypointsThroughCLI(t *testing.T) {
	if os.Getenv("OTELPLAN_OTELC") == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned integration tests")
	}

	cache := t.TempDir()
	t.Setenv("GOMODCACHE", cache)
	testenv.ConfigureGoCache(t, cache)

	root := natsCLIEntrypointFixture(t)
	prepareNATSFixture(t, root)
	testenv.DenyNetwork(t)

	before := testenv.SnapshotFiles(t, root)

	jsonOutput := runNATSCLI(t, root, "scan", "--offline", "--format=json")
	checkNATSScanJSON(t, jsonOutput)

	textOutput := runNATSCLI(t, root, "scan", "--offline", "--format=text")
	checkNATSScanText(t, textOutput)

	firstInit := runNATSCLI(t, root, "init", "--offline", "--non-interactive",
		"--dry-run", "--format=json", "--output=starter.yaml")
	checkNATSInitJSON(t, firstInit)

	secondInit := runNATSCLI(t, root, "init", "--offline", "--non-interactive",
		"--dry-run", "--format=json", "--output=starter.yaml")
	if !bytes.Equal(firstInit, secondInit) {
		t.Fatal("repeated NATS init dry-runs were not deterministic")
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
	checkNATSResolutionLock(t, root)
}

func checkNATSResolutionLock(t *testing.T, root string) {
	t.Helper()

	directory := openLockTestRoot(t, root)
	writeLockTestFile(t, directory, "otelplan.yaml", `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: operation
  match: {symbols: ["example.com/natscli.Run"]}
`)
	runNATSCLI(t, root, "lock", "--offline", "--format=json")

	source, err := directory.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}

	prefix, _, found := strings.Cut(string(source), "func register()")
	if !found {
		t.Fatal("missing NATS registration fixture")
	}

	writeLockTestFile(t, directory, "app.go", prefix+"func register() {}\n")
	before := testenv.SnapshotFiles(t, root)
	runNATSCLI(t, root, "lock", "--check", "--offline", "--format=json")
	runNATSCLI(t, root, "diff", "--check", "--offline", "--format=json")
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
}

func runNATSCLI(t *testing.T, root string, command ...string) []byte {
	t.Helper()

	args := append([]string{"--root", root}, command...)

	var stdout, stderr bytes.Buffer
	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 || stderr.Len() != 0 {
		t.Fatalf("NATS CLI %v exit=%d stdout=%s stderr=%s", command, exit, &stdout, &stderr)
	}

	checkNATSCLIPrivacy(t, stdout.Bytes())

	return slices.Clone(stdout.Bytes())
}

func checkNATSScanJSON(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid NATS scan JSON: %v %s", err, output)
	}

	var code model.CodeModel

	err = json.Unmarshal(reply.Data, &code)
	if err != nil {
		t.Fatalf("decode NATS scan model: %v %s", err, output)
	}

	want := map[model.SymbolID][]string{
		"example.com/natscli.Core": {
			"github.com/nats-io/nats.go.(*Conn).Subscribe",
		},
		"example.com/natscli.Queue": {
			"github.com/nats-io/nats.go.(*Conn).QueueSubscribe",
		},
		"example.com/natscli.Legacy": {
			"github.com/nats-io/nats.go.(JetStream).Subscribe",
		},
		"example.com/natscli.LegacyQueue": {
			"github.com/nats-io/nats.go.(JetStream).QueueSubscribe",
		},
		"example.com/natscli.Pull": {
			"github.com/nats-io/nats.go/jetstream.(Consumer).Consume",
		},
		"example.com/natscli.Push": {
			"github.com/nats-io/nats.go/jetstream.(PushConsumer).Consume",
		},
	}
	for id, entrypoints := range want {
		symbol := natsCLISymbol(t, &code, id)
		if !slices.Equal(symbol.Entrypoints, entrypoints) {
			t.Fatalf("NATS entrypoints for %s = %v, want %v", id, symbol.Entrypoints, entrypoints)
		}
	}

	for _, symbolID := range []model.SymbolID{
		"example.com/natscli.Unregistered",
		"example.com/natscli.FakeTarget",
		"example.com/natscli.Stored",
		"example.com/natscli.ErrorCallback",
	} {
		if symbol := natsCLISymbol(t, &code, symbolID); len(symbol.Entrypoints) != 0 {
			t.Fatalf("unregistered NATS callback %s has metadata: %v", symbolID, symbol.Entrypoints)
		}
	}
}

func checkNATSScanText(t *testing.T, output []byte) {
	t.Helper()

	text := string(output)
	for _, expected := range []string{
		"example.com/natscli.Core",
		"entrypoints github.com/nats-io/nats.go.(*Conn).Subscribe",
		"example.com/natscli.Legacy",
		"entrypoints github.com/nats-io/nats.go.(JetStream).Subscribe",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("NATS text scan missing %q: %s", expected, output)
		}
	}
}

func checkNATSInitJSON(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid NATS init JSON: %v %s", err, output)
	}

	var summary struct {
		Suggestions []struct {
			Symbol   model.SymbolID `json:"symbol"`
			Evidence []string       `json:"evidence"`
		} `json:"suggestions"`
	}

	err = json.Unmarshal(reply.Data, &summary)
	if err != nil {
		t.Fatalf("decode NATS init suggestions: %v %s", err, output)
	}

	if len(summary.Suggestions) != 1 || summary.Suggestions[0].Symbol != "example.com/natscli.Run" {
		t.Fatalf("NATS starter selected callbacks without context or lost downstream Run: %s", output)
	}

	evidence := strings.Join(summary.Suggestions[0].Evidence, " ")
	if !strings.Contains(evidence, "reachable from") || !strings.Contains(evidence, "example.com/natscli.Core") ||
		!strings.Contains(evidence, "github.com/nats-io/nats.go.(*Conn).Subscribe") {
		t.Fatalf("NATS Run suggestion lost static registration provenance: %s", output)
	}
}

func checkNATSCLIPrivacy(t *testing.T, output []byte) {
	t.Helper()

	for _, privateValue := range []string{"private.subject", "private.queue", "private-marker"} {
		if bytes.Contains(output, []byte(privateValue)) {
			t.Fatalf("NATS CLI exposed registration or payload values: %s", output)
		}
	}
}

func natsCLISymbol(t *testing.T, code *model.CodeModel, id model.SymbolID) *model.Symbol {
	t.Helper()

	symbol, found := code.Symbol(id)
	if !found {
		t.Fatalf("missing NATS CLI symbol %s", id)
	}

	return symbol
}

func prepareNATSFixture(t *testing.T, root string) {
	t.Helper()

	tidy := exec.CommandContext(t.Context(), "go", "mod", "tidy")
	tidy.Dir = root

	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare NATS fixture with go mod tidy: %v: %s", err, output)
	}

	download := exec.CommandContext(t.Context(), "go", "mod", "download", "all")
	download.Dir = root

	output, err = download.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare NATS fixture with go mod download: %v: %s", err, output)
	}
}

func natsCLIEntrypointFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	files := map[string]string{
		"go.mod": `module example.com/natscli

go 1.27

require github.com/nats-io/nats.go v1.53.1
`,
		"app.go": `package natscli

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func Run(context.Context) error { return nil }

func Core(*nats.Msg) { _ = Run(context.Background()); _ = "private-marker" }
func Queue(*nats.Msg) { _ = Run(context.Background()) }
func Legacy(*nats.Msg) { _ = Run(context.Background()) }
func LegacyQueue(*nats.Msg) { _ = Run(context.Background()) }
func Pull(jetstream.Msg) { _ = Run(context.Background()) }
func Push(jetstream.Msg) { _ = Run(context.Background()) }

func Unregistered(*nats.Msg) {}
func Stored(*nats.Msg) {}
var stored = Stored
func FakeTarget(*nats.Msg) {}
func ErrorCallback(*nats.Conn, *nats.Subscription, error) {}

type FakeConn struct{}
func (FakeConn) Subscribe(string, nats.MsgHandler) (*nats.Subscription, error) { return nil, nil }

func register() {
	conn := new(nats.Conn)
	conn.Subscribe("private.subject", Core)
	conn.QueueSubscribe("private.subject", "private.queue", Queue)

	var legacy nats.JetStreamContext
	legacy.Subscribe("private.subject", Legacy, nil)
	legacy.QueueSubscribe("private.subject", "private.queue", LegacyQueue, nil)

	var consumer jetstream.Consumer
	consumer.Consume(Pull, nil)
	var push jetstream.PushConsumer
	push.Consume(Push, nil)

	conn.Subscribe("private.subject", stored)
	conn.Subscribe("private.subject", func(*nats.Msg) {})
	conn.SetErrorHandler(ErrorCallback)
	FakeConn{}.Subscribe("private.subject", FakeTarget)
}
`,
	}
	for name, contents := range files {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}
