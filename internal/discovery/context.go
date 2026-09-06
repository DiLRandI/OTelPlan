package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

type buildEnvironment struct {
	GOOS         string
	GOARCH       string
	GOVERSION    string
	GOWORK       string
	GOFLAGS      string
	CGO_ENABLED  string
	GOEXPERIMENT string
	GOFIPS140    string
	GOAMD64      string
	GOARM        string
	GO386        string
	GOMIPS       string
	GOMIPS64     string
	GOPPC64      string
	GORISCV64    string
	GOWASM       string
	CGO_CFLAGS   string
	CGO_CPPFLAGS string
	CGO_LDFLAGS  string
	CGO_FFLAGS   string
	GOTOOLCHAIN  string
	GOARM64      string
	GOMOD        string
	CC           string
	CXX          string
	CGO_CXXFLAGS string
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
	env := append(os.Environ(), opts.Env...)
	driver := ""
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "GOPACKAGESDRIVER="); ok {
			driver = value
		}
	}
	if driver != "" && driver != "off" {
		return nil, nil, fmt.Errorf("custom GOPACKAGESDRIVER is unsupported for reproducible analysis")
	}
	env = replaceEnv(env, "GOPACKAGESDRIVER", "off")
	if opts.GOOS != "" {
		env = append(env, "GOOS="+opts.GOOS)
	}
	if opts.GOARCH != "" {
		env = append(env, "GOARCH="+opts.GOARCH)
	}
	if opts.Offline {
		env = append(env, "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off", "GOTOOLCHAIN=local")
	}
	command := exec.CommandContext(ctx, "go", "env", "-json", "GOOS", "GOARCH", "GOVERSION", "GOWORK", "GOFLAGS", "CGO_ENABLED", "GOEXPERIMENT", "GOFIPS140", "GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_LDFLAGS", "CGO_FFLAGS", "GOTOOLCHAIN", "GOMOD", "CC", "CXX", "CGO_CXXFLAGS")
	command.Dir = root
	command.Env = env
	output, err := command.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("read Go build environment: %w", err)
	}
	var build buildEnvironment
	if err := json.Unmarshal(output, &build); err != nil {
		return nil, nil, fmt.Errorf("decode Go build environment: %w", err)
	}
	parsed, err := parseGOFLAGS(build.GOFLAGS)
	if err != nil {
		return nil, nil, err
	}
	env = replaceEnv(env, "GOFLAGS", "")
	opts.GOOS, opts.GOARCH, opts.goVersion = build.GOOS, build.GOARCH, build.GOVERSION
	if build.GOWORK != "off" {
		opts.workspaceFile = build.GOWORK
	}
	vendorRoot := root
	if build.GOMOD != "" && build.GOMOD != os.DevNull {
		vendorRoot = filepath.Dir(build.GOMOD)
	}
	if build.GOWORK != "" && build.GOWORK != "off" {
		vendorRoot = filepath.Dir(build.GOWORK)
	}
	mode := "readonly"
	_, vendorErr := os.Stat(filepath.Join(vendorRoot, "vendor", "modules.txt"))
	if parsed.moduleMode != "" {
		mode = parsed.moduleMode
	} else if vendorErr == nil {
		mode = "vendor"
	}
	if len(opts.BuildTags) == 0 {
		opts.BuildTags = parsed.tags
	}
	opts.BuildTags = normalizeTags(opts.BuildTags)
	if parsed.modFile != "" {
		if !strings.HasSuffix(parsed.modFile, ".mod") {
			return nil, nil, fmt.Errorf("alternate module manifest must have a .mod extension")
		}
		if !filepath.IsAbs(parsed.modFile) {
			parsed.modFile = filepath.Join(root, parsed.modFile)
		}
		opts.effectiveBuild.ModFile = filepath.Clean(parsed.modFile)
	}
	if mode != "vendor" && opts.workspaceFile == "" {
		original := build.GOMOD
		if opts.effectiveBuild.ModFile != "" {
			original = opts.effectiveBuild.ModFile
		}
		data, readErr := os.ReadFile(original)
		if readErr != nil {
			return nil, nil, fmt.Errorf("read effective module manifest: %w", readErr)
		}
		tmp, createErr := os.CreateTemp("", "otelplan-effective-*.mod")
		if createErr != nil {
			return nil, nil, fmt.Errorf("create isolated module manifest: %w", createErr)
		}
		tmpName := tmp.Name()
		if _, writeErr := tmp.Write(data); writeErr != nil {
			tmp.Close()
			os.Remove(tmpName)
			return nil, nil, fmt.Errorf("write isolated module manifest: %w", writeErr)
		}
		if closeErr := tmp.Close(); closeErr != nil {
			os.Remove(tmpName)
			return nil, nil, fmt.Errorf("close isolated module manifest: %w", closeErr)
		}
		if sum, sumErr := os.ReadFile(companionSum(original)); sumErr == nil {
			if writeErr := os.WriteFile(strings.TrimSuffix(tmpName, ".mod")+".sum", sum, 0600); writeErr != nil {
				os.Remove(tmpName)
				return nil, nil, fmt.Errorf("write isolated module checksums: %w", writeErr)
			}
		} else if !os.IsNotExist(sumErr) {
			os.Remove(tmpName)
			return nil, nil, fmt.Errorf("read effective module checksums: %w", sumErr)
		}
		opts.cleanup = func() {
			os.Remove(tmpName)
			os.Remove(strings.TrimSuffix(tmpName, ".mod") + ".sum")
		}
		parsed.modFile = tmpName
	}
	opts.effectiveBuild = model.BuildEnvironment{
		GoVersion: build.GOVERSION, GOOS: build.GOOS, GOARCH: build.GOARCH,
		BuildTags: append([]string(nil), opts.BuildTags...), ModuleMode: mode,
		ModFile: opts.effectiveBuild.ModFile, Workspace: build.GOWORK != "" && build.GOWORK != "off",
		CGOEnabled: build.CGO_ENABLED, GOEXPERIMENT: build.GOEXPERIMENT, GOFIPS140: build.GOFIPS140,
		GOAMD64: build.GOAMD64, GOARM: build.GOARM, GOARM64: build.GOARM64, GO386: build.GO386, GOMIPS: build.GOMIPS,
		GOMIPS64: build.GOMIPS64, GOPPC64: build.GOPPC64, GORISCV64: build.GORISCV64,
		GOWASM: build.GOWASM, CGOCFLAGS: build.CGO_CFLAGS, CGOCPPFLAGS: build.CGO_CPPFLAGS,
		CGOLDFLAGS: build.CGO_LDFLAGS, CGOFFLAGS: build.CGO_FFLAGS,
		SemanticFlags: append([]string(nil), parsed.semantic...),
		CC:            build.CC, CXX: build.CXX, CGOCXXFLAGS: build.CGO_CXXFLAGS,
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); os.IsNotExist(err) && filepath.Dir(build.GOWORK) == root && len(opts.Patterns) == 1 && opts.Patterns[0] == "./..." {
		data, err := os.ReadFile(build.GOWORK)
		if err != nil {
			return nil, nil, fmt.Errorf("read workspace: %w", err)
		}
		work, err := modfile.ParseWork(build.GOWORK, data, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("parse workspace: %w", err)
		}
		opts.Patterns = nil
		for _, use := range work.Use {
			module := use.Path
			if !filepath.IsAbs(module) {
				module = filepath.Join(root, module)
			}
			opts.Patterns = append(opts.Patterns, filepath.ToSlash(filepath.Join(module, "...")))
		}
	}
	if opts.workspaceFile != "" {
		workspace, cleanup, err := isolateWorkspace(opts.workspaceFile)
		if err != nil {
			return nil, nil, err
		}
		opts.cleanup = cleanup
		env = replaceEnv(env, "GOWORK", workspace)
	}
	flags := append([]string{"-mod=" + mode}, buildFlags(opts.BuildTags)...)
	if parsed.modFile != "" {
		flags = append(flags, "-modfile="+parsed.modFile)
	}
	flags = append(flags, parsed.semantic...)
	return env, flags, nil
}

func companionSum(modfile string) string {
	if strings.HasSuffix(modfile, ".mod") {
		return strings.TrimSuffix(modfile, ".mod") + ".sum"
	}
	return modfile + ".sum"
}

// parseGOFLAGS uses the same whole-argument quoting accepted by Go's command
// tools, while retaining only flags relevant to package analysis.
func parseGOFLAGS(raw string) (goFlags, error) {
	tokens, err := splitQuoted(raw)
	if err != nil {
		return goFlags{}, fmt.Errorf("invalid GOFLAGS: %w", err)
	}
	var out goFlags
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		name, value, hasValue := strings.Cut(token, "=")
		if strings.HasPrefix(name, "--") {
			name = name[1:]
		}
		if !hasValue && (name == "-mod" || name == "-modfile" || name == "-tags") {
			return goFlags{}, fmt.Errorf("invalid GOFLAGS: %s requires =value", name)
		}
		if out.semanticBy == nil {
			out.semanticBy = make(map[string]string)
		}
		switch name {
		case "-mod":
			if value != "mod" && value != "readonly" && value != "vendor" {
				return goFlags{}, fmt.Errorf("invalid GOFLAGS: unsupported module mode")
			}
			out.moduleMode = value
		case "-modfile":
			if value == "" {
				return goFlags{}, fmt.Errorf("invalid GOFLAGS: empty modfile")
			}
			out.modFile = value
		case "-tags":
			out.tags = strings.Split(value, ",")
		case "-race", "-msan", "-asan", "-trimpath", "-buildvcs":
			if !hasValue {
				value = "true"
			}
			if value != "true" && value != "false" && !(name == "-buildvcs" && value == "auto") {
				return goFlags{}, fmt.Errorf("invalid GOFLAGS: boolean option %s", name)
			}
			out.semanticBy[name] = value
		case "", "-n", "-v", "-x", "-work", "-json", "-p", "-modcacherw":
			// Output, diagnostic, or cache flags do not affect package selection.
		case "-gcflags", "-asmflags", "-ldflags", "-gccgoflags", "-overlay", "-toolexec", "-pkgdir", "-exec", "-installsuffix":
			return goFlags{}, fmt.Errorf("unsupported GOFLAGS option %s", name)
		default:
			if strings.HasPrefix(name, "-") {
				return goFlags{}, fmt.Errorf("unsupported GOFLAGS option %s", name)
			}
			return goFlags{}, fmt.Errorf("invalid GOFLAGS token")
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
			i := strings.IndexByte(raw, quote)
			if i < 0 {
				return nil, fmt.Errorf("unterminated quoted string")
			}
			out = append(out, raw[:i])
			raw = raw[i+1:]
			continue
		}
		i := 0
		for i < len(raw) && !strings.ContainsRune(" \t\n\r", rune(raw[i])) {
			i++
		}
		out = append(out, raw[:i])
		raw = raw[i:]
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
