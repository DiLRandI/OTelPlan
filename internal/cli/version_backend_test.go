package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestVersionAvailableBackend(t *testing.T) {
	backend := versionCLIBackend(t)
	t.Setenv("PATH", backend.Name())
	root := t.TempDir()

	for _, version := range []string{"v1.1.0", "v1.2.0+caller-private-value"} {
		err := backend.WriteFile("output", []byte("otelc version "+version+" ignored caller-private-value"), 0o600)
		if err != nil {
			t.Fatal(err)
		}

		reply, versions := runVersionJSON(t.Context(), t, root, nil, 0)
		if !reply.OK || versions["backendStatus"] != "available" ||
			versions["backendAvailableVersion"] != strings.Split(version, "+")[0] {
			t.Fatalf("available backend data=%v", versions)
		}
	}

	checkVersionAvailableText(t, root)
	checkVersionInvalidProbe(t, backend, root)
}

func checkVersionAvailableText(t *testing.T, root string) {
	t.Helper()

	for _, quiet := range []bool{false, true} {
		var stdout, stderr bytes.Buffer

		args := []string{"version", "--root", root, "--quiet=" + strconv.FormatBool(quiet)}

		exit := cli.Run(t.Context(), args, &stdout, &stderr)
		if exit != 0 || stderr.Len() != 0 || (quiet && stdout.Len() != 0) ||
			(!quiet && !strings.Contains(stdout.String(), "available v1.2.0")) ||
			strings.Contains(stdout.String(), "caller-private-value") {
			t.Fatalf("unexpected version text: %d %s %s", exit, &stdout, &stderr)
		}
	}
}

func checkVersionInvalidProbe(t *testing.T, backend *os.Root, root string) {
	t.Helper()

	err := backend.WriteFile("output", []byte("caller-private-value"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	reply, versions := runVersionJSON(t.Context(), t, root, []string{"--verbose"}, 7)
	checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeBackendUnsupported})

	if versions["backendStatus"] != "unverified" {
		t.Fatalf("unverified backend data=%v", versions)
	}
}

func versionCLIBackend(t *testing.T) *os.Root {
	t.Helper()

	directory, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	err = directory.WriteFile("otelc.go", []byte(versionCLIBackendSource), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(t.Context(), "go", "build", "-o", ".", "otelc.go")
	command.Dir = directory.Name()

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build version backend: %v\n%s", err, output)
	}

	return directory
}

const versionCLIBackendSource = `package main
import("fmt";"os";"path/filepath")
func main(){
 if len(os.Args)!=2 || os.Args[1]!="version" { os.Exit(2) }
 executable,err:=os.Executable()
 if err!=nil { os.Exit(3) }
 output,err:=os.ReadFile(filepath.Join(filepath.Dir(executable),"output"))
 if err!=nil { os.Exit(4) }
 fmt.Fprintln(os.Stderr,"caller-private-value")
 fmt.Print(string(output))
}
`
