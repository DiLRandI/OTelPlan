package compiler_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPreviewArtifactsMatchesPublicationWithoutWrites(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	destination := filepath.Join(parent, "nested", "generated")
	files := publicationBundle(t, "preview")
	staged := stagePreviewFiles(t, parent, files)

	slices.Reverse(staged.Files)
	stagedBefore := slices.Clone(staged.Files)

	preview, err := compiler.PreviewArtifacts(destination, staged, false)
	if err != nil {
		t.Fatal(err)
	}

	wantDestination, err := filepath.Abs(destination)
	if err != nil {
		t.Fatal(err)
	}

	assertPreviewInventory(t, preview, stagedBefore, wantDestination)
	assertPreviewDidNotCreate(t, destination)
	assertStagedUnchanged(t, staged, stagedBefore)

	published, err := compiler.PublishArtifacts(destination, files, false)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(preview, published) {
		t.Fatalf("preview differs from subsequent publication: preview=%+v published=%+v", preview, published)
	}
}

func TestPreviewArtifactsHonorsCleanAndOwnedOutput(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	destination := filepath.Join(parent, "build")
	first := publishBundle(t, destination, "first", false)
	staged := stagePreviewFiles(t, parent, publicationBundle(t, "second"))

	assertReplacementRequiresClean(t, destination, staged)

	preview, err := compiler.PreviewArtifacts(destination, staged, true)
	if err != nil {
		t.Fatal(err)
	}

	assertPreviewMatches(t, preview, destination, staged.Files)
	assertExistingUnchanged(t, destination, first)
	assertEditedOutputRejected(t, destination, staged)
	assertUnrelatedOutputRejected(t, destination, staged)
}

func TestPreviewArtifactsRejectsSymlinkDestinationAndDanglingParent(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	staged := stagePreviewFiles(t, parent, publicationBundle(t, "preview"))

	owned := filepath.Join(parent, "owned")

	_, err := compiler.PublishArtifacts(owned, publicationBundle(t, "preview"), false)
	if err != nil {
		t.Fatal(err)
	}

	assertSymlinkDestinationRejected(t, parent, owned, staged)
	assertValidParentSymlink(t, parent, staged)
	assertDanglingParentRejected(t, parent, staged)
}

func TestPreviewArtifactsRejectsTamperedStagedBundle(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(*os.Root, *model.Artifacts) error
	}{
		{name: "inventory", mutate: func(_ *os.Root, staged *model.Artifacts) error {
			staged.Files[0].Digest = ""

			return nil
		}},
		{name: "manifest", mutate: func(root *os.Root, _ *model.Artifacts) error {
			return root.WriteFile("manifest.json", []byte("changed"), 0o600)
		}},
		{name: "extra", mutate: func(root *os.Root, _ *model.Artifacts) error {
			return root.WriteFile("extra", []byte("unexpected"), 0o600)
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			staged := stagePreviewFiles(t, t.TempDir(), publicationBundle(t, "preview"))

			root, err := os.OpenRoot(staged.Dir)
			if err != nil {
				t.Fatal(err)
			}

			err = testCase.mutate(root, &staged)
			_ = root.Close()

			if err != nil {
				t.Fatal(err)
			}

			_, err = compiler.PreviewArtifacts(filepath.Join(t.TempDir(), "generated"), staged, false)
			if err == nil {
				t.Fatal("preview accepted tampered staged bundle")
			}
		})
	}
}

func TestPreviewArtifactsRejectsInvalidDestinationParent(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	staged := stagePreviewFiles(t, parent, publicationBundle(t, "preview"))
	parentFile := filepath.Join(parent, "parent-file")
	writePreviewFile(t, parentFile, "file")

	_, err := compiler.PreviewArtifacts(filepath.Join(parentFile, "generated"), staged, false)
	if err == nil {
		t.Fatal("preview accepted non-directory parent")
	}

	destinationFile := filepath.Join(parent, "destination-file")
	writePreviewFile(t, destinationFile, "file")

	_, err = compiler.PreviewArtifacts(destinationFile, staged, false)
	if err == nil {
		t.Fatal("preview accepted non-directory destination")
	}
}

func stagePreviewFiles(t *testing.T, parent string, files []otelc.GeneratedFile) model.Artifacts {
	t.Helper()

	staged, err := compiler.StageArtifacts(parent, files)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(staged.Dir) })

	return staged
}

func assertPreviewInventory(t *testing.T, preview model.Artifacts, staged []model.ArtifactFile, destination string) {
	t.Helper()

	if preview.Dir != destination {
		t.Fatalf("preview destination=%q, want %q", preview.Dir, destination)
	}

	if !sort.SliceIsSorted(preview.Files, func(i, j int) bool { return preview.Files[i].Path < preview.Files[j].Path }) {
		t.Fatal("preview inventory is not sorted")
	}

	wantFiles := slices.Clone(staged)
	sort.Slice(wantFiles, func(i, j int) bool { return wantFiles[i].Path < wantFiles[j].Path })

	if !reflect.DeepEqual(preview.Files, wantFiles) {
		t.Fatalf("preview inventory differs from staged inventory: got=%+v want=%+v", preview.Files, wantFiles)
	}
}

func assertPreviewDidNotCreate(t *testing.T, destination string) {
	t.Helper()

	assertMissingPath(t, destination, "preview created destination")
	assertMissingPath(t, filepath.Dir(destination), "preview created destination parent")
}

func assertMissingPath(t *testing.T, path, message string) {
	t.Helper()

	_, err := os.Stat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s: %v", message, err)
	}
}

func assertStagedUnchanged(t *testing.T, staged model.Artifacts, before []model.ArtifactFile) {
	t.Helper()

	if !slices.Equal(before, staged.Files) {
		t.Fatal("preview mutated staged inventory")
	}

	err := compiler.VerifyArtifacts(staged)
	if err != nil {
		t.Fatalf("preview changed staged files: %v", err)
	}
}

func assertReplacementRequiresClean(t *testing.T, destination string, staged model.Artifacts) {
	t.Helper()

	_, err := compiler.PreviewArtifacts(destination, staged, false)
	if err == nil {
		t.Fatal("preview accepted replacement without clean")
	}
}

func assertPreviewMatches(t *testing.T, preview model.Artifacts, destination string, files []model.ArtifactFile) {
	t.Helper()

	if preview.Dir != destination || !reflect.DeepEqual(preview.Files, files) {
		t.Fatalf("unexpected clean preview: %+v", preview)
	}
}

func assertExistingUnchanged(t *testing.T, destination string, want model.Artifacts) {
	t.Helper()

	current, err := compiler.ReadArtifacts(destination)
	if err != nil || !reflect.DeepEqual(current, want) {
		t.Fatalf("preview mutated existing output: artifacts=%+v err=%v", current, err)
	}
}

func assertEditedOutputRejected(t *testing.T, destination string, staged model.Artifacts) {
	t.Helper()

	root, err := os.OpenRoot(destination)
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := root.ReadFile("manifest.json")
	if err != nil {
		_ = root.Close()

		t.Fatal(err)
	}

	err = root.WriteFile("manifest.json", []byte("changed"), 0o600)
	_ = root.Close()

	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PreviewArtifacts(destination, staged, true)
	if err == nil {
		t.Fatal("clean preview accepted edited output")
	}

	root, err = os.OpenRoot(destination)
	if err != nil {
		t.Fatal(err)
	}

	err = root.WriteFile("manifest.json", manifest, 0o600)
	_ = root.Close()

	if err != nil {
		t.Fatal(err)
	}
}

func assertUnrelatedOutputRejected(t *testing.T, destination string, staged model.Artifacts) {
	t.Helper()

	root, err := os.OpenRoot(destination)
	if err != nil {
		t.Fatal(err)
	}

	err = root.WriteFile("unrelated", []byte("keep"), 0o600)
	_ = root.Close()

	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PreviewArtifacts(destination, staged, true)
	if err == nil {
		t.Fatal("clean preview accepted output with unrelated file")
	}
}

func assertSymlinkDestinationRejected(t *testing.T, parent, owned string, staged model.Artifacts) {
	t.Helper()

	link := filepath.Join(parent, "link")

	err := os.Symlink(owned, link)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PreviewArtifacts(link, staged, true)
	if err == nil {
		t.Fatal("preview accepted symlink destination")
	}
}

func assertValidParentSymlink(t *testing.T, parent string, staged model.Artifacts) {
	t.Helper()

	realParent := filepath.Join(parent, "real-parent")

	err := os.Mkdir(realParent, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	parentLink := filepath.Join(parent, "parent-link")

	err = os.Symlink(realParent, parentLink)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PreviewArtifacts(filepath.Join(parentLink, "generated"), staged, false)
	if err != nil {
		t.Fatalf("preview rejected valid parent symlink: %v", err)
	}

	assertMissingPath(t, filepath.Join(realParent, "generated"), "preview created through parent symlink")
}

func assertDanglingParentRejected(t *testing.T, parent string, staged model.Artifacts) {
	t.Helper()

	dangling := filepath.Join(parent, "dangling")

	err := os.Symlink(filepath.Join(parent, "missing"), dangling)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PreviewArtifacts(filepath.Join(dangling, "generated"), staged, false)
	if err == nil {
		t.Fatal("preview accepted dangling parent symlink")
	}
}

func writePreviewFile(t *testing.T, path, contents string) {
	t.Helper()

	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}
