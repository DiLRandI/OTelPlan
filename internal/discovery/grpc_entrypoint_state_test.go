package discovery_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestGRPCEntrypointCacheAndRelocation(t *testing.T) {
	t.Parallel()

	root := grpcEntrypointFixture(t)

	var options discovery.Options

	options.Root, options.Offline, options.CacheDir = root, true, t.TempDir()
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	for _, readOnly := range []bool{false, true} {
		options.CacheReadOnly = readOnly

		code, err := discovery.LoadContext(t.Context(), options)
		if err != nil {
			t.Fatal(err)
		}

		assertGRPCEntrypoints(t, code, "example.com/grpcentrypoints.(Implementation).Unary", []string{
			"google.golang.org/grpc.(*Server).RegisterService",
			"google.golang.org/grpc.(ServiceRegistrar).RegisterService",
		})
	}

	relocated := t.TempDir()

	err := os.CopyFS(relocated, os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}

	code := loadGRPCEntrypointModel(t, relocated)
	assertGRPCEntrypoints(t, code, "example.com/grpcentrypoints.(Implementation).Unary", []string{
		"google.golang.org/grpc.(*Server).RegisterService",
		"google.golang.org/grpc.(ServiceRegistrar).RegisterService",
	})
}

func TestGRPCEntrypointsRejectChangedRegistrationInputs(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		registration string
		changeAPI    bool
	}{
		{
			name:      "mutated descriptor",
			changeAPI: false,
			registration: `api.ServiceDesc.Methods = nil
				server.RegisterService(&api.ServiceDesc, &Implementation{})`,
		},
		{
			name: "reassigned forwarded service", changeAPI: true,
			registration: `api.RegisterService(&server, &Implementation{})`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := grpcEntrypointFixture(t)
			directory := openGRPCStateFixture(t, root)

			source, err := directory.ReadFile("app.go")
			if err != nil {
				t.Fatal(err)
			}

			prefix, _, found := strings.Cut(string(source), "func registerAll()")
			if !found {
				t.Fatal("missing registration fixture")
			}

			body := "func registerAll() { var server aliasgrpc.Server; " + test.registration + " }\n"

			err = directory.WriteFile("app.go", []byte(prefix+body), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			if test.changeAPI {
				reassignGRPCWrapper(t, directory)
			}

			code := loadGRPCEntrypointModel(t, root)
			assertGRPCEntrypoints(t, code, "example.com/grpcentrypoints.(Implementation).Unary", nil)
			assertGRPCEntrypoints(t, code, "example.com/grpcentrypoints.(Implementation).Stream", nil)
		})
	}
}

func reassignGRPCWrapper(t *testing.T, directory *os.Root) {
	t.Helper()

	path := filepath.Join("api", "generated.go")

	source, err := directory.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	changed := strings.Replace(string(source), "s.RegisterService(&ServiceDesc, srv)",
		"srv = nil; s.RegisterService(&ServiceDesc, srv)", 1)

	err = directory.WriteFile(path, []byte(changed), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func openGRPCStateFixture(t *testing.T, path string) *os.Root {
	t.Helper()

	directory, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Errorf("close gRPC state fixture: %v", err)
		}
	})

	return directory
}
