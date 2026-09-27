package lockfile_test

import (
	"encoding/json"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
)

func TestGraphEnvironmentFingerprintCompatibility(t *testing.T) {
	t.Parallel()

	code := graphFixture(t)
	code.EffectiveBuild.GoVersion = "go1.27.0"
	code.EffectiveBuild.ModuleMode = "readonly"
	code.EffectiveBuild.BuildTags = []string{"production", "custom"}
	code.EffectiveBuild.SemanticFlags = []string{"-race", "-gcflags=all=-N"}
	code.EffectiveBuild.GOAMD64 = "v3"
	code.EffectiveBuild.GOARM = "7"
	code.EffectiveBuild.CGOEnabled = "0"
	code.EffectiveBuild.CC = "/unused/compiler"

	before, err := json.Marshal(code)
	if err != nil {
		t.Fatalf("encode original code model: %v", err)
	}

	digest, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatalf("fingerprint effective environment: %v", err)
	}

	const want = "sha256:cb50a52a23113a771c14c4dccc1aa02522e8607409c44897c36e204f98cfef04"
	if digest != want {
		t.Fatalf("fingerprint = %q; want %q", digest, want)
	}

	after, err := json.Marshal(code)
	if err != nil {
		t.Fatalf("encode code model after fingerprinting: %v", err)
	}

	if string(before) != string(after) {
		t.Fatal("fingerprinting mutated the caller's build environment")
	}
}
