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

func TestGRPCEntrypointsThroughCLI(t *testing.T) {
	if os.Getenv("OTELPLAN_OTELC") == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned integration tests")
	}

	cache := t.TempDir()
	t.Setenv("GOMODCACHE", cache)
	testenv.ConfigureGoCache(t, cache)

	root := grpcCLIEntrypointFixture(t)
	prepareGRPCFixture(t, root)
	testenv.DenyNetwork(t)

	before := testenv.SnapshotFiles(t, root)

	jsonOutput := runGRPCCLI(t, root, "scan", "--offline", "--format=json")
	checkGRPCScanJSON(t, jsonOutput)

	textOutput := runGRPCCLI(t, root, "scan", "--offline", "--format=text")
	checkGRPCScanText(t, textOutput)

	firstInit := runGRPCCLI(t, root, "init", "--offline", "--non-interactive", "--dry-run", "--format=json")
	checkGRPCInitJSON(t, firstInit)

	secondInit := runGRPCCLI(t, root, "init", "--offline", "--non-interactive", "--dry-run", "--format=json")
	if !bytes.Equal(firstInit, secondInit) {
		t.Fatal("repeated gRPC init dry-runs were not deterministic")
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
	checkGRPCResolutionLock(t, root)
}

func runGRPCCLI(t *testing.T, root string, command ...string) []byte {
	t.Helper()

	args := append([]string{"--root", root}, command...)

	var stdout, stderr bytes.Buffer
	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 || stderr.Len() != 0 {
		t.Fatalf("gRPC CLI %v exit=%d stdout=%s stderr=%s", command, exit, &stdout, &stderr)
	}

	return slices.Clone(stdout.Bytes())
}

func checkGRPCScanJSON(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid gRPC scan JSON: %v %s", err, output)
	}

	var code model.CodeModel

	err = json.Unmarshal(reply.Data, &code)
	if err != nil {
		t.Fatalf("decode gRPC scan model: %v %s", err, output)
	}

	implemented := grpcSymbol(t, &code, "example.com/grpcintegration.(*Implementation).Check")

	unregistered := grpcSymbol(t, &code, "example.com/grpcintegration.(*Unregistered).Check")
	if len(implemented.Entrypoints) == 0 {
		t.Fatalf("implemented Check has no registration identity: %+v", implemented)
	}

	if len(unregistered.Entrypoints) != 0 {
		t.Fatalf("unregistered Check has registration identity: %+v", unregistered)
	}

	wantEntrypoints := []string{
		"google.golang.org/grpc.(*Server).RegisterService",
		"google.golang.org/grpc.(ServiceRegistrar).RegisterService",
	}
	gotEntrypoints := slices.Clone(implemented.Entrypoints)
	slices.Sort(gotEntrypoints)
	slices.Sort(wantEntrypoints)

	if !slices.Equal(gotEntrypoints, wantEntrypoints) {
		t.Fatalf("implemented Check registration identities = %v, want %v", implemented.Entrypoints, wantEntrypoints)
	}
}

func checkGRPCScanText(t *testing.T, output []byte) {
	t.Helper()

	text := string(output)
	implementedID := "example.com/grpcintegration.(*Implementation).Check"
	unregisteredID := "example.com/grpcintegration.(*Unregistered).Check"

	if !strings.Contains(text, implementedID) ||
		!strings.Contains(text, "entrypoints google.golang.org/grpc.(*Server).RegisterService") {
		t.Fatalf("text scan lost implemented gRPC registration: %s", output)
	}

	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if !strings.Contains(line, unregisteredID) {
			continue
		}

		for _, following := range lines[index+1:] {
			if following == "" || strings.HasPrefix(following, " ") {
				if strings.HasPrefix(following, "  entrypoints ") {
					t.Fatalf("text scan exposed unregistered gRPC Check registration: %s", output)
				}

				continue
			}

			break
		}
	}
}

func checkGRPCInitJSON(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid gRPC init JSON: %v %s", err, output)
	}

	var summary struct {
		Suggestions []struct {
			Symbol   model.SymbolID `json:"symbol"`
			Evidence []string       `json:"evidence"`
		} `json:"suggestions"`
	}

	err = json.Unmarshal(reply.Data, &summary)
	if err != nil {
		t.Fatalf("decode gRPC init suggestions: %v %s", err, output)
	}

	const implementedID = model.SymbolID("example.com/grpcintegration.(*Implementation).Check")

	var found bool

	for _, suggestion := range summary.Suggestions {
		if suggestion.Symbol != implementedID {
			continue
		}

		found = true

		if !slices.ContainsFunc(suggestion.Evidence, func(evidence string) bool {
			return strings.Contains(evidence, "registered entrypoint") &&
				strings.Contains(evidence, "google.golang.org/grpc.(*Server).RegisterService") &&
				strings.Contains(evidence, "google.golang.org/grpc.(ServiceRegistrar).RegisterService")
		}) {
			t.Fatalf("implemented gRPC Check suggestion lost registration evidence: %s", output)
		}
	}

	if !found {
		t.Fatalf("init omitted implemented gRPC Check suggestion: %s", output)
	}
}

func grpcSymbol(t *testing.T, code *model.CodeModel, id model.SymbolID) *model.Symbol {
	t.Helper()

	symbol, found := code.Symbol(id)
	if !found {
		t.Fatalf("missing gRPC symbol %s", id)
	}

	return symbol
}

func prepareGRPCFixture(t *testing.T, root string) {
	t.Helper()

	for _, command := range []*exec.Cmd{
		exec.CommandContext(t.Context(), "go", "mod", "tidy"),
		exec.CommandContext(t.Context(), "go", "mod", "download", "all"),
	} {
		command.Dir = root

		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("prepare gRPC fixture dependencies: %v: %s", err, output)
		}
	}
}

func checkGRPCResolutionLock(t *testing.T, root string) {
	t.Helper()

	directory := openLockTestRoot(t, root)
	writeLockTestFile(t, directory, "otelplan.yaml", `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: rpc
  match:
    symbols: ["example.com/grpcintegration.(*Implementation).Check"]
`)
	runGRPCCLI(t, root, "lock", "--offline", "--format=json")

	source, err := directory.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}

	changed := strings.NewReplacer(
		"server.RegisterService(&healthpb.Health_ServiceDesc, &Implementation{})", "_ = server",
		"healthpb.RegisterHealthServer(server, &Implementation{})", "_ = server",
	).Replace(string(source))
	writeLockTestFile(t, directory, "app.go", changed)
	before := testenv.SnapshotFiles(t, root)
	runGRPCCLI(t, root, "lock", "--check", "--offline", "--format=json")
	runGRPCCLI(t, root, "diff", "--check", "--offline", "--format=json")
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
}

func grpcCLIEntrypointFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	files := map[string]string{
		"go.mod": `module example.com/grpcintegration

go 1.27

require google.golang.org/grpc v1.81.1
`,
		"app.go": `package grpcintegration

import (
	"context"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Implementation struct {
	healthpb.UnimplementedHealthServer
}

func (*Implementation) Check(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{}, nil
}

type Unregistered struct {
	healthpb.UnimplementedHealthServer
}

func (*Unregistered) Check(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{}, nil
}

func registerDirect(server *grpc.Server) {
	server.RegisterService(&healthpb.Health_ServiceDesc, &Implementation{})
}

func registerWrapper(server *grpc.Server) {
	healthpb.RegisterHealthServer(server, &Implementation{})
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
