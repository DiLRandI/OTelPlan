package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestConnectEntrypointsThroughCLI(t *testing.T) {
	if os.Getenv("OTELPLAN_OTELC") == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned integration tests")
	}

	cache := t.TempDir()
	t.Setenv("GOMODCACHE", cache)
	testenv.ConfigureGoCache(t, cache)
	root := connectCLIFixture(t)
	prepareConnectFixture(t, root)
	testenv.DenyNetwork(t)

	before := testenv.SnapshotFiles(t, root)
	inventory := runConnectCLI(t, root, "scan", "--format=json")
	checkConnectInventory(t, inventory)

	text := runConnectCLI(t, root, "scan", "--format=text")
	if !bytes.Contains(text, []byte("entrypoints connectrpc.com/connect.NewUnaryHandler")) {
		t.Fatalf("text scan lost Connect identity: %s", text)
	}

	first := runConnectCLI(t, root, "init", "--dry-run", "--output=starter.yaml", "--format=json")

	second := runConnectCLI(t, root, "init", "--dry-run", "--output=starter.yaml", "--format=json")
	if !bytes.Equal(first, second) {
		t.Fatal("Connect init preview is not deterministic")
	}

	checkConnectStarter(t, first)
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
}

func runConnectCLI(t *testing.T, root string, command ...string) []byte {
	t.Helper()

	args := append([]string{"--root", root, "--offline"}, command...)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("Connect CLI %v exit=%d stdout=%s stderr=%s", command, exit, &stdout, &stderr)
	}

	if bytes.Contains(stdout.Bytes(), []byte("private-connect-procedure")) {
		t.Fatalf("Connect CLI retained a procedure value: %s", &stdout)
	}

	return slices.Clone(stdout.Bytes())
}

func checkConnectInventory(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("decode Connect scan: %v %s", err, output)
	}

	var code model.CodeModel

	err = json.Unmarshal(reply.Data, &code)
	if err != nil {
		t.Fatal(err)
	}

	for name, constructor := range map[string]string{
		"Unary":        "NewUnaryHandler",
		"UnarySimple":  "NewUnaryHandlerSimple",
		"Client":       "NewClientStreamHandler",
		"ClientSimple": "NewClientStreamHandlerSimple",
		"Server":       "NewServerStreamHandler",
		"ServerSimple": "NewServerStreamHandlerSimple",
		"Bidi":         "NewBidiStreamHandler",
	} {
		assertConnectCLIIdentity(t, &code, "example.com/connectcli."+name,
			[]string{"connectrpc.com/connect." + constructor})
	}

	assertConnectCLIIdentity(t, &code, "example.com/connectcli.(*Endpoint).Wrapped",
		[]string{"connectrpc.com/connect.NewUnaryHandler"})
	assertConnectCLIIdentity(t, &code, "example.com/connectcli.Unregistered", nil)
}

func assertConnectCLIIdentity(t *testing.T, code *model.CodeModel, name string, expected []string) {
	t.Helper()

	symbol, found := code.Symbol(model.SymbolID(name))
	if !found || !slices.Equal(symbol.Entrypoints, expected) {
		t.Fatalf("Connect identity for %s = %+v, expected %v", name, symbol, expected)
	}
}

func checkConnectStarter(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("decode Connect starter: %v %s", err, output)
	}

	var summary struct {
		Policy      string `json:"policy"`
		Suggestions []struct {
			Symbol   model.SymbolID `json:"symbol"`
			Evidence []string       `json:"evidence"`
		} `json:"suggestions"`
	}

	err = json.Unmarshal(reply.Data, &summary)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(summary.Policy, "attributes:") {
		t.Fatalf("Connect starter enabled capture: %s", summary.Policy)
	}

	for _, suggestion := range summary.Suggestions {
		if suggestion.Symbol == "example.com/connectcli.Run" &&
			strings.Contains(strings.Join(suggestion.Evidence, " "), "connectrpc.com/connect.NewUnaryHandler") {
			return
		}
	}

	t.Fatalf("Connect starter lost downstream business provenance: %s", output)
}

func prepareConnectFixture(t *testing.T, root string) {
	t.Helper()

	for _, command := range []*exec.Cmd{
		exec.CommandContext(t.Context(), "go", "mod", "tidy"),
		exec.CommandContext(t.Context(), "go", "mod", "download", "all"),
	} {
		command.Dir = root

		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("prepare Connect dependencies: %v: %s", err, output)
		}
	}
}

func connectCLIFixture(t *testing.T) string {
	t.Helper()

	directory := openLockTestRoot(t, t.TempDir())
	writeLockTestFile(t, directory, "go.mod", `module example.com/connectcli

go 1.27

require connectrpc.com/connect v1.19.1
`)
	writeLockTestFile(t, directory, "app.go", `package connectcli
import("context"; "connectrpc.com/connect"; "example.com/connectcli/api")
type Request struct{}
type Response struct{}
func Run(context.Context) error { return nil }
func Unary(ctx context.Context, _ *connect.Request[Request]) (*connect.Response[Response], error) {return nil, Run(ctx)}
func UnarySimple(context.Context, *Request) (*Response,error){return nil,nil}
func Client(context.Context,*connect.ClientStream[Request])(*connect.Response[Response],error){return nil,nil}
func ClientSimple(context.Context,*connect.ClientStream[Request])(*Response,error){return nil,nil}
func Server(context.Context,*connect.Request[Request],*connect.ServerStream[Response])error{return nil}
func ServerSimple(context.Context,*Request,*connect.ServerStream[Response])error{return nil}
func Bidi(context.Context,*connect.BidiStream[Request,Response])error{return nil}
func Unregistered(context.Context,*connect.Request[Request])(*connect.Response[Response],error){return nil,nil}
type Endpoint struct{}
func (*Endpoint) Wrapped(ctx context.Context, _ *connect.Request[api.Request])(*connect.Response[api.Response],error){
 return nil,Run(ctx)
}
func construct(){
 connect.NewUnaryHandler("private-connect-procedure",Unary)
 connect.NewUnaryHandlerSimple("private-connect-procedure",UnarySimple)
 connect.NewClientStreamHandler("private-connect-procedure",Client)
 connect.NewClientStreamHandlerSimple("private-connect-procedure",ClientSimple)
 connect.NewServerStreamHandler("private-connect-procedure",Server)
 connect.NewServerStreamHandlerSimple("private-connect-procedure",ServerSimple)
 connect.NewBidiStreamHandler("private-connect-procedure",Bidi)
 api.Factory(&Endpoint{}, nil)
}
`)

	err := directory.Mkdir("api", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	writeLockTestFile(t, directory, "api/api.go", `package api
import("context";"connectrpc.com/connect")
type Request struct{}
type Response struct{}
type Service interface{Wrapped(context.Context,*connect.Request[Request])(*connect.Response[Response],error)}
func Factory(s Service,opts ...connect.HandlerOption)*connect.Handler{
 return connect.NewUnaryHandler("private-connect-procedure",s.Wrapped,opts...)
}
`)

	return directory.Name()
}
