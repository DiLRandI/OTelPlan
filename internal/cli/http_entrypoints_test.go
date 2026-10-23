package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const httpStarterSource = `package app
import("context";"net/http")
func Run(context.Context) error { return nil }
func Incoming(_ http.ResponseWriter, request *http.Request) { _ = Run(request.Context()) }
func register(){ http.HandleFunc("/caller-private-value", Incoming) }
`

func TestHTTPEntrypointStarterAndScan(t *testing.T) {
	t.Parallel()
	root, _, _ := policyExecutionFixture(t, httpStarterSource, executionPolicy)
	before := testenv.SnapshotFiles(t, root)

	for _, format := range []string{"json", "text"} {
		var stdout, stderr bytes.Buffer

		args := []string{"init", "--root", root, "--offline", "--dry-run", "--format=" + format, "--output=starter.yaml"}

		exit := cli.Run(t.Context(), args, &stdout, &stderr)
		if exit != 0 || stderr.Len() != 0 || strings.Contains(stdout.String(), "caller-private-value") {
			t.Fatalf("HTTP starter exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
		}

		if !strings.Contains(stdout.String(), "example.com/app.Incoming") ||
			!strings.Contains(stdout.String(), "net/http.HandleFunc") {
			t.Fatalf("starter lost HTTP provenance: %s", &stdout)
		}

		if format == "json" {
			checkHTTPStarterJSON(t, stdout.Bytes())
		}
	}

	checkHTTPScanText(t, root)

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
}

func checkHTTPScanText(t *testing.T, root string) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{"scan", "--root", root, "--offline"}, &stdout, &stderr)
	if exit != 0 || !strings.Contains(stdout.String(), "entrypoints net/http.HandleFunc") ||
		strings.Contains(stdout.String(), "caller-private-value") {
		t.Fatalf("scan entrypoints exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
	}
}

func checkHTTPStarterJSON(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid starter JSON: %v %s", err, output)
	}

	var summary struct {
		Suggestions []struct {
			Symbol model.SymbolID `json:"symbol"`
		} `json:"suggestions"`
	}

	err = json.Unmarshal(reply.Data, &summary)
	if err != nil || len(summary.Suggestions) != 1 || summary.Suggestions[0].Symbol != "example.com/app.Run" {
		t.Fatalf("HTTP handler was silently selected or business target lost: %v %s", err, output)
	}
}

func TestHTTPRegistrationMetadataDoesNotDriftExplicitLock(t *testing.T) {
	t.Parallel()

	source := strings.Replace(httpStarterSource, `http.HandleFunc("/caller-private-value", Incoming)`, "", 1)
	root, directory, _ := policyExecutionFixture(t, source, executionPolicy)
	runGlobalArgumentsJSON(t, []string{"lock", "--root", root, "--offline", "--format=json"}, 0)

	err := directory.WriteFile("app.go", []byte(httpStarterSource), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	before := testenv.SnapshotFiles(t, root)
	runGlobalArgumentsJSON(t, []string{"lock", "--check", "--root", root, "--offline", "--format=json"}, 0)
	runGlobalArgumentsJSON(t, []string{"diff", "--check", "--root", root, "--offline", "--format=json"}, 0)
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
}
