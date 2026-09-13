package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"go/version"
	"os/exec"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// RecordedBuildEnvironment restores discovery's process settings. Module and
// workspace paths are supplied separately after their isolated copies exist.
func RecordedBuildEnvironment(base []string, build model.BuildEnvironment) ([]string, []string, error) {
	if !version.IsValid(build.GoVersion) || build.GOOS == "" || build.GOARCH == "" {
		return nil, nil, fmt.Errorf("complete analyzed Go environment is required")
	}
	values := recordedGoValues(build)
	env := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := values[key]; !overridden {
			env = append(env, entry)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	tags := append([]string(nil), build.BuildTags...)
	sort.Strings(tags)
	flags := append([]string(nil), build.SemanticFlags...)
	if len(tags) > 0 {
		flags = append(flags, "-tags="+strings.Join(tags, ","))
	}
	return env, flags, nil
}

func recordedGoValues(build model.BuildEnvironment) map[string]string {
	return map[string]string{
		"GOFLAGS": "", "GOTOOLCHAIN": build.GoVersion, "GOPACKAGESDRIVER": "off",
		"GOOS": build.GOOS, "GOARCH": build.GOARCH, "CGO_ENABLED": build.CGOEnabled,
		"GOEXPERIMENT": build.GOEXPERIMENT, "GOFIPS140": build.GOFIPS140,
		"GOAMD64": build.GOAMD64, "GOARM": build.GOARM, "GOARM64": build.GOARM64, "GO386": build.GO386,
		"GOMIPS": build.GOMIPS, "GOMIPS64": build.GOMIPS64, "GOPPC64": build.GOPPC64, "GORISCV64": build.GORISCV64, "GOWASM": build.GOWASM,
		"CGO_CFLAGS": build.CGOCFLAGS, "CGO_CPPFLAGS": build.CGOCPPFLAGS, "CGO_CXXFLAGS": build.CGOCXXFLAGS, "CGO_LDFLAGS": build.CGOLDFLAGS, "CGO_FFLAGS": build.CGOFFLAGS, "CC": build.CC, "CXX": build.CXX,
	}
}

func verifyRecordedGoEnvironment(ctx context.Context, dir string, env []string, build model.BuildEnvironment) error {
	expected := recordedGoValues(build)
	delete(expected, "GOPACKAGESDRIVER")
	expected["GOVERSION"] = build.GoVersion
	keys := make([]string, 0, len(expected))
	for key := range expected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	command := exec.CommandContext(ctx, "go", append([]string{"env", "-json"}, keys...)...)
	command.Dir, command.Env = dir, env
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("verify recorded Go environment: %w", err)
	}
	var actual map[string]string
	if err := json.Unmarshal(output, &actual); err != nil {
		return fmt.Errorf("decode effective Go environment: %w", err)
	}
	for _, key := range keys {
		if actual[key] != expected[key] {
			return fmt.Errorf("effective Go setting %s differs from analysis", key)
		}
	}
	return nil
}
