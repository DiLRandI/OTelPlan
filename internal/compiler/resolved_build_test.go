package compiler_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type buildResolvedFailureTest struct {
	name        string
	change      func(*compiler.ResolvedBuildRequest)
	wantMessage string
}

func TestBuildResolvedFailureCleanup(t *testing.T) {
	t.Parallel()

	manifest := []byte("module example.com/app\n\ngo 1.27.0\n")

	cases := []buildResolvedFailureTest{
		{
			name: "caller output flag",
			change: func(request *compiler.ResolvedBuildRequest) {
				request.GoArgs = []string{"-o", "elsewhere"}
			},
			wantMessage: "resolved build owns its temporary output path",
		},
		{
			name: "nil code",
			change: func(request *compiler.ResolvedBuildRequest) {
				request.Code = nil
			},
			wantMessage: "build requires analysis",
		},
		{
			name: "conflicting output modes",
			change: func(request *compiler.ResolvedBuildRequest) {
				request.DefaultOutput, request.DirectoryOutput = true, true
			},
			wantMessage: "build output modes are mutually exclusive",
		},
		{
			name:        "missing working directory",
			wantMessage: "",
			change: func(request *compiler.ResolvedBuildRequest) {
				request.WorkingDir = filepath.Join(request.Code.ModuleRoot, "missing")
			},
		},
		{
			name: "missing backend",
			change: func(request *compiler.ResolvedBuildRequest) {
				request.Backend.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(nil))
			},
			wantMessage: "",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runBuildResolvedFailureCase(t, testCase, manifest)
		})
	}
}

func runBuildResolvedFailureCase(t *testing.T, testCase buildResolvedFailureTest, manifest []byte) {
	t.Helper()

	source, parent := t.TempDir(), t.TempDir()
	sourceRoot, parentRoot, request := newBuildResolvedFailureFixture(t, source, parent, manifest)

	testCase.change(&request)

	checkBuildResolvedFailure(t, request, testCase.wantMessage)
	checkBuildResolvedFailureCleanup(t, sourceRoot, parentRoot, manifest)
}

func newBuildResolvedFailureFixture(
	t *testing.T,
	source, parent string,
	manifest []byte,
) (*os.Root, *os.Root, compiler.ResolvedBuildRequest) {
	t.Helper()

	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		t.Fatalf("open source fixture root: %v", err)
	}

	t.Cleanup(func() { closeBuildResolvedFixtureRoot(t, sourceRoot, "source") })

	err = sourceRoot.WriteFile("go.mod", manifest, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatalf("create backend identity: %v", err)
	}

	parentRoot, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatalf("open staging parent root: %v", err)
	}

	t.Cleanup(func() { closeBuildResolvedFixtureRoot(t, parentRoot, "staging parent") })

	var code model.CodeModel

	code.ModuleRoot = source

	var module model.ModuleInfo

	module.Main = true

	module.Dir = source

	module.Path = "example.com/app"
	code.Modules = []model.ModuleInfo{module}
	code.EffectiveBuild.GoVersion = runtime.Version()
	code.EffectiveBuild.GOOS = runtime.GOOS

	code.EffectiveBuild.GOARCH = runtime.GOARCH
	code.EffectiveBuild.ModuleMode = "readonly"

	var request compiler.ResolvedBuildRequest

	request.Code = &code
	request.Backend = backend

	request.Executable = filepath.Join(parent, "missing-backend")
	request.RuntimeVersion = "test"
	request.Parent = parent
	request.Env = os.Environ()
	request.Offline = true

	return sourceRoot, parentRoot, request
}

func closeBuildResolvedFixtureRoot(t *testing.T, root *os.Root, description string) {
	t.Helper()

	err := root.Close()
	if err != nil {
		t.Errorf("close %s fixture root: %v", description, err)
	}
}

func checkBuildResolvedFailure(t *testing.T, request compiler.ResolvedBuildRequest, wantMessage string) {
	t.Helper()

	_, firstErr := compiler.BuildResolved(t.Context(), request)
	if firstErr == nil {
		t.Fatal("accepted invalid build request")
	}

	if wantMessage == "" {
		return
	}

	if firstErr.Error() != wantMessage {
		t.Fatalf("BuildResolved() error = %q, want %q", firstErr, wantMessage)
	}

	_, secondErr := compiler.BuildResolved(t.Context(), request)
	if secondErr == nil {
		t.Fatal("accepted invalid build request")
	}

	if !errors.Is(secondErr, firstErr) {
		t.Fatalf("BuildResolved() error cause = %v, want stable cause %v", secondErr, firstErr)
	}
}

func checkBuildResolvedFailureCleanup(t *testing.T, sourceRoot, parentRoot *os.Root, manifest []byte) {
	t.Helper()

	parentDirectory, err := parentRoot.Open(".")
	if err != nil {
		t.Fatalf("open staging parent directory: %v", err)
	}

	entries, err := parentDirectory.ReadDir(-1)
	closeErr := parentDirectory.Close()

	if err != nil {
		t.Fatalf("read staging parent: %v", err)
	}

	if closeErr != nil {
		t.Fatalf("close staging parent directory: %v", closeErr)
	}

	gotManifest, err := sourceRoot.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read source module manifest: %v", err)
	}

	if string(gotManifest) != string(manifest) {
		t.Fatal("failed build changed source module manifest")
	}

	if len(entries) != 0 {
		t.Fatalf("failed build left temporary output: %v", entries)
	}
}
