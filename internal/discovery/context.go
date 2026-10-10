package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

const (
	buildTagsFlag        = "-tags"
	vendorModuleMode     = "vendor"
	disabledGoSetting    = "off"
	isolatedManifestMode = 0o600
)

type buildEnvironment struct {
	GOOS         string `json:"GOOS"`
	GOARCH       string `json:"GOARCH"`
	GOVERSION    string `json:"GOVERSION"`
	GOWORK       string `json:"GOWORK"`
	GOFLAGS      string `json:"GOFLAGS"`
	CGOEnabled   string `json:"CGO_ENABLED"`
	GOEXPERIMENT string `json:"GOEXPERIMENT"`
	GOFIPS140    string `json:"GOFIPS140"`
	GOAMD64      string `json:"GOAMD64"`
	GOARM        string `json:"GOARM"`
	GO386        string `json:"GO386"`
	GOMIPS       string `json:"GOMIPS"`
	GOMIPS64     string `json:"GOMIPS64"`
	GOPPC64      string `json:"GOPPC64"`
	GORISCV64    string `json:"GORISCV64"`
	GOWASM       string `json:"GOWASM"`
	CGOCFLAGS    string `json:"CGO_CFLAGS"`
	CGOCPPFLAGS  string `json:"CGO_CPPFLAGS"`
	CGOLDFLAGS   string `json:"CGO_LDFLAGS"`
	CGOFFLAGS    string `json:"CGO_FFLAGS"`
	GOTOOLCHAIN  string `json:"GOTOOLCHAIN"`
	GOARM64      string `json:"GOARM64"`
	GOMOD        string `json:"GOMOD"`
	CC           string `json:"CC"`
	CXX          string `json:"CXX"`
	CGOCXXFLAGS  string `json:"CGO_CXXFLAGS"`
}

type goFlags struct {
	moduleMode string
	modFile    string
	tags       []string
	semantic   []string
	semanticBy map[string]string
}

func prepare(ctx context.Context, opts *Options) ([]string, []string, error) {
	opts.cleanup = func() {}
	opts.applyDefaults()

	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve project root: %w", err)
	}

	opts.Root = root

	env, err := prepareGoEnvironment(opts)
	if err != nil {
		return nil, nil, err
	}

	build, err := readBuildEnvironment(ctx, root, env)
	if err != nil {
		return nil, nil, err
	}

	parsed, err := parseGOFLAGS(build.GOFLAGS, opts.BuildFlags...)
	if err != nil {
		return nil, nil, err
	}

	env = replaceEnv(env, "GOFLAGS", "")

	mode, err := configureBuildSelection(opts, build, &parsed)
	if err != nil {
		return nil, nil, err
	}

	env, err = prepareBuildMetadata(opts, build, &parsed, mode, env)
	if err != nil {
		return nil, nil, err
	}

	flags := append([]string{"-mod=" + mode}, buildFlags(opts.BuildTags)...)
	if parsed.modFile != "" {
		flags = append(flags, "-modfile="+parsed.modFile)
	}

	flags = append(flags, parsed.semantic...)

	return env, flags, nil
}

func prepareBuildMetadata(opts *Options, build buildEnvironment, parsed *goFlags, mode string,
	env []string) ([]string, error) {
	if mode != vendorModuleMode && opts.workspaceFile == "" {
		original := build.GOMOD
		if opts.effectiveBuild.ModFile != "" {
			original = opts.effectiveBuild.ModFile
		}

		isolated, cleanup, moduleErr := isolateModuleManifest(original)
		if moduleErr != nil {
			return nil, moduleErr
		}

		parsed.modFile, opts.cleanup = isolated, cleanup
	}

	opts.effectiveBuild = recordedBuildEnvironment(build, mode, opts.effectiveBuild.ModFile, opts.BuildTags, *parsed)

	err := expandWorkspacePatterns(opts, build.GOWORK)
	if err != nil {
		return nil, err
	}

	if opts.workspaceFile != "" {
		workspace, cleanup, workspaceErr := isolateWorkspace(opts.workspaceFile)
		if workspaceErr != nil {
			return nil, workspaceErr
		}

		opts.cleanup = cleanup
		env = replaceEnv(env, "GOWORK", workspace)
	}

	return env, nil
}

func prepareGoEnvironment(opts *Options) ([]string, error) {
	env := append(os.Environ(), opts.Env...)
	driver := ""

	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "GOPACKAGESDRIVER="); ok {
			driver = value
		}
	}

	if driver != "" && driver != disabledGoSetting {
		return nil, errCustomPackageDriver
	}

	env = replaceEnv(env, "GOPACKAGESDRIVER", disabledGoSetting)
	if opts.GOOS != "" {
		env = append(env, "GOOS="+opts.GOOS)
	}

	if opts.GOARCH != "" {
		env = append(env, "GOARCH="+opts.GOARCH)
	}

	if opts.Offline {
		env = append(env, "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off", "GOTOOLCHAIN=local")
	}

	return env, nil
}

func readBuildEnvironment(ctx context.Context, root string, env []string) (buildEnvironment, error) {
	var empty buildEnvironment

	command := exec.CommandContext(ctx, "go", "env", "-json",
		"GOOS", "GOARCH", "GOVERSION", "GOWORK", "GOFLAGS", "CGO_ENABLED", "GOEXPERIMENT", "GOFIPS140",
		"GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM",
		"CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_LDFLAGS", "CGO_FFLAGS", "GOTOOLCHAIN", "GOMOD", "CC", "CXX", "CGO_CXXFLAGS",
	)
	command.Dir = root
	command.Env = env

	output, err := command.Output()
	if err != nil {
		return empty, fmt.Errorf("read Go build environment: %w", err)
	}

	var build buildEnvironment

	err = json.Unmarshal(output, &build)
	if err != nil {
		return empty, fmt.Errorf("decode Go build environment: %w", err)
	}

	return build, nil
}

func configureBuildSelection(opts *Options, build buildEnvironment, parsed *goFlags) (string, error) {
	opts.GOOS, opts.GOARCH, opts.goVersion = build.GOOS, build.GOARCH, build.GOVERSION
	if build.GOWORK != disabledGoSetting {
		opts.workspaceFile = build.GOWORK
	}

	mode := selectModuleMode(opts.Root, build, parsed.moduleMode)

	for _, flag := range opts.BuildFlags {
		name, _, _ := strings.Cut(flag, "=")
		if name == buildTagsFlag || name == "--tags" {
			opts.BuildTags = nil
		}
	}

	if len(opts.BuildTags) == 0 {
		opts.BuildTags = parsed.tags
	}

	opts.BuildTags = normalizeTags(opts.BuildTags)

	if parsed.modFile != "" {
		if !strings.HasSuffix(parsed.modFile, ".mod") {
			return "", errModuleManifestExtension
		}

		if !filepath.IsAbs(parsed.modFile) {
			parsed.modFile = filepath.Join(opts.Root, parsed.modFile)
		}

		opts.effectiveBuild.ModFile = filepath.Clean(parsed.modFile)
	}

	return mode, nil
}

func selectModuleMode(root string, build buildEnvironment, explicitMode string) string {
	vendorRoot := root
	if build.GOMOD != "" && build.GOMOD != os.DevNull {
		vendorRoot = filepath.Dir(build.GOMOD)
	}

	if build.GOWORK != "" && build.GOWORK != disabledGoSetting {
		vendorRoot = filepath.Dir(build.GOWORK)
	}

	mode := "readonly"

	_, vendorErr := os.Stat(filepath.Join(vendorRoot, "vendor", "modules.txt"))

	if explicitMode != "" {
		mode = explicitMode
	} else if vendorErr == nil {
		mode = vendorModuleMode
	}

	return mode
}

func recordedBuildEnvironment(build buildEnvironment, mode, modFile string, tags []string,
	parsed goFlags) model.BuildEnvironment {
	return model.BuildEnvironment{
		GoVersion: build.GOVERSION, GOOS: build.GOOS, GOARCH: build.GOARCH,
		BuildTags: append([]string(nil), tags...), ModuleMode: mode,
		ModFile: modFile, Workspace: build.GOWORK != "" && build.GOWORK != disabledGoSetting,
		CGOEnabled: build.CGOEnabled, GOEXPERIMENT: build.GOEXPERIMENT, GOFIPS140: build.GOFIPS140,
		GOAMD64: build.GOAMD64, GOARM: build.GOARM, GOARM64: build.GOARM64, GO386: build.GO386, GOMIPS: build.GOMIPS,
		GOMIPS64: build.GOMIPS64, GOPPC64: build.GOPPC64, GORISCV64: build.GORISCV64,
		GOWASM: build.GOWASM, CGOCFLAGS: build.CGOCFLAGS, CGOCPPFLAGS: build.CGOCPPFLAGS,
		CGOLDFLAGS: build.CGOLDFLAGS, CGOFFLAGS: build.CGOFFLAGS,
		SemanticFlags: append([]string(nil), parsed.semantic...),
		CC:            build.CC, CXX: build.CXX, CGOCXXFLAGS: build.CGOCXXFLAGS,
	}
}

func expandWorkspacePatterns(opts *Options, workspace string) error {
	_, statErr := os.Stat(filepath.Join(opts.Root, "go.mod"))

	if os.IsNotExist(statErr) && filepath.Dir(workspace) == opts.Root && len(opts.Patterns) == 1 &&
		opts.Patterns[0] == "./..." {
		data, err := readBuildMetadata(workspace)
		if err != nil {
			return fmt.Errorf("read workspace: %w", err)
		}

		work, err := modfile.ParseWork(workspace, data, nil)
		if err != nil {
			return fmt.Errorf("parse workspace: %w", err)
		}

		opts.Patterns = nil

		for _, use := range work.Use {
			module := use.Path
			if !filepath.IsAbs(module) {
				module = filepath.Join(opts.Root, module)
			}

			opts.Patterns = append(opts.Patterns, filepath.ToSlash(filepath.Join(module, "...")))
		}
	}

	return nil
}

func companionSum(modfile string) string {
	if before, ok := strings.CutSuffix(modfile, ".mod"); ok {
		return before + ".sum"
	}

	return modfile + ".sum"
}

// parseGOFLAGS uses the same whole-argument quoting accepted by Go's command
// tools, while retaining only flags relevant to package analysis.
func parseGOFLAGS(raw string, overrides ...string) (goFlags, error) {
	tokens, err := splitQuoted(raw)
	if err != nil {
		return goFlags{}, fmt.Errorf("invalid GOFLAGS: %w", err)
	}

	tokens = append(tokens, overrides...)

	var out goFlags

	for _, token := range tokens {
		err := applyGoFlag(&out, token)
		if err != nil {
			return goFlags{}, err
		}
	}

	out.tags = normalizeTags(out.tags)
	for name, value := range out.semanticBy {
		out.semantic = append(out.semantic, name+"="+value)
	}

	sort.Strings(out.semantic)

	return out, nil
}

func splitQuoted(raw string) ([]string, error) {
	var out []string

	for len(raw) > 0 {
		raw = strings.TrimLeft(raw, " \t\n\r")
		if raw == "" {
			break
		}

		if raw[0] == '\'' || raw[0] == '"' {
			quote := raw[0]
			raw = raw[1:]

			quoteEnd := strings.IndexByte(raw, quote)
			if quoteEnd < 0 {
				return nil, errUnterminatedQuote
			}

			out = append(out, raw[:quoteEnd])
			raw = raw[quoteEnd+1:]

			continue
		}

		tokenEnd := 0
		for tokenEnd < len(raw) && !strings.ContainsRune(" \t\n\r", rune(raw[tokenEnd])) {
			tokenEnd++
		}

		out = append(out, raw[:tokenEnd])
		raw = raw[tokenEnd:]
	}

	return out, nil
}

func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="

	out := make([]string, 0, len(env)+1)

	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}

	return append(out, prefix+value)
}

func normalizeTags(tags []string) []string {
	set := make(map[string]struct{}, len(tags))

	for _, value := range tags {
		for _, tag := range strings.FieldsFunc(value, func(c rune) bool { return c == ',' || unicode.IsSpace(c) }) {
			set[tag] = struct{}{}
		}
	}

	out := make([]string, 0, len(set))
	for tag := range set {
		out = append(out, tag)
	}

	sort.Strings(out)

	return out
}

func applyGoFlag(flags *goFlags, token string) error {
	name, value, hasValue := strings.Cut(token, "=")
	if strings.HasPrefix(name, "--") {
		name = name[1:]
	}

	if !hasValue && slices.Contains([]string{"-mod", "-modfile", buildTagsFlag}, name) {
		return fmt.Errorf("%w: %s requires =value", errInvalidGoFlags, name)
	}

	if flags.semanticBy == nil {
		flags.semanticBy = make(map[string]string)
	}

	switch name {
	case "-mod":
		return applyModuleMode(flags, value)
	case "-modfile":
		if value == "" {
			return errEmptyModuleManifest
		}

		flags.modFile = value
	case buildTagsFlag:
		flags.tags = strings.Split(value, ",")
	default:
		return applyNonModuleFlag(flags, name, value, hasValue)
	}

	return nil
}

func applyModuleMode(flags *goFlags, value string) error {
	if value != "mod" && value != "readonly" && value != vendorModuleMode {
		return errUnsupportedModuleMode
	}

	flags.moduleMode = value

	return nil
}

func applyNonModuleFlag(flags *goFlags, name, value string, hasValue bool) error {
	switch name {
	case "-race", "-msan", "-asan", "-trimpath", "-buildvcs":
		return applyBooleanFlag(flags, name, value, hasValue)
	case "", "-n", "-v", "-x", "-work", "-json", "-p", "-modcacherw":
		// Output, diagnostic, or cache flags do not affect package selection.
		return nil
	default:
		if strings.HasPrefix(name, "-") {
			return fmt.Errorf("%w %s", errUnsupportedGoFlag, name)
		}

		return errInvalidFlagToken
	}
}

func applyBooleanFlag(flags *goFlags, name, value string, hasValue bool) error {
	if !hasValue {
		value = "true"
	}

	if value != "true" && value != "false" && (name != "-buildvcs" || value != "auto") {
		return fmt.Errorf("%w: boolean option %s", errInvalidGoFlags, name)
	}

	flags.semanticBy[name] = value

	return nil
}
