package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
)

func TestInspectionWireContracts(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOPACKAGESDRIVER", "off")
	fixture := openLockTestRoot(t, "testdata/wire-contracts/project")
	project := t.TempDir()
	err := os.CopyFS(project, fixture.FS())
	requireTestSuccess(t, err, "copy wire-contract project")
	before := testenv.SnapshotFiles(t, project)
	goldens := openLockTestRoot(t, "testdata/wire-contracts")

	for _, testCase := range []struct {
		golden string
		args   []string
		exit   int
	}{
		{golden: "inspect.json", args: []string{"inspect"}, exit: 0},
		{golden: "explain-selected.json", args: []string{"explain", "example.com/wire.(*Worker).Run"}, exit: 0},
		{golden: "explain-excluded.json", args: []string{"explain", "example.com/wire.Skip"}, exit: 0},
		{golden: "explain-local-excluded.json", args: []string{"explain", "example.com/wire.localSkip"}, exit: 0},
		{golden: "inspect-unsafe.json", args: []string{"inspect", "--config", "unsafe.yaml"}, exit: 5},
	} {
		output := runInspectionWireCommand(t, project, testCase.exit, testCase.args)
		want, readErr := goldens.ReadFile(testCase.golden)
		requireTestSuccess(t, readErr, "read expected wire contract")

		if canonicalWireJSON(t, output) != canonicalWireJSON(t, want) {
			t.Fatalf("%s wire contract changed\ngot:\n%s\nwant:\n%s", testCase.golden, output, want)
		}

		testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, project))
	}
}

func runInspectionWireCommand(t *testing.T, project string, expectedExit int, args []string) []byte {
	t.Helper()

	arguments := append([]string{"--root", project, "--offline", "--strict", "--format=json"}, args...)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), arguments, &stdout, &stderr)
	if exit != expectedExit || stderr.Len() != 0 {
		t.Fatalf("wire-contract command %v failed: exit=%d stdout=%s stderr=%s", args, exit, &stdout, &stderr)
	}

	return stdout.Bytes()
}

func canonicalWireJSON(t *testing.T, data []byte) string {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any

	err := decoder.Decode(&value)
	requireTestSuccess(t, err, "decode wire contract")

	var trailing any

	err = decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		t.Fatal("wire contract must contain one JSON document", err)
	}

	encoded, err := json.Marshal(value)
	requireTestSuccess(t, err, "canonicalize wire contract")

	return string(encoded)
}
