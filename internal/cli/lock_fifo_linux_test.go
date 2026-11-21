package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/cli"
)

const lockFIFOTimeout = 5 * time.Second

type lockFIFOReply struct {
	exit           int
	stdout, stderr string
}

func TestLockCommandsRejectFIFOWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"lock"}, {"lock", "--check"}, {"lock", "--dry-run"}, {"diff"}, {"validate"},
	} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			t.Parallel()

			root, files := lockContractFixture(t)
			directory := openLockTestRoot(t, root)
			path := filepath.Join(root, "otelplan.lock")

			err := syscall.Mkfifo(path, 0o600)
			requireTestSuccess(t, err, "create lockfile FIFO")

			result := make(chan lockFIFOReply, 1)

			arguments := append([]string{"--root", root, "--offline", "--format=json"}, args...)

			go func() {
				var stdout, stderr bytes.Buffer

				exit := cli.Run(t.Context(), arguments, &stdout, &stderr)
				result <- lockFIFOReply{exit: exit, stdout: stdout.String(), stderr: stderr.String()}
			}()

			reply := awaitLockFIFOReply(t, directory, result)
			if reply.exit != 6 || reply.stderr != "" {
				t.Errorf("FIFO lock command exit=%d stdout=%s stderr=%s", reply.exit, reply.stdout, reply.stderr)
			}

			var decoded globalArgumentReply

			err = json.Unmarshal([]byte(reply.stdout), &decoded)
			requireTestSuccess(t, err, "decode FIFO lockfile diagnostic")
			assertDiagnostic(t, decoded, "cannot read lockfile")

			info, err := directory.Lstat("otelplan.lock")
			if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("lockfile FIFO was replaced: info=%v error=%v", info, err)
			}

			for name, contents := range files {
				assertEqualFile(t, directory, name, contents)
			}

			entries, err := fs.ReadDir(directory.FS(), ".")
			if err != nil || len(entries) != len(files)+1 {
				t.Fatalf("FIFO failure changed project file set: entries=%v error=%v", entries, err)
			}
		})
	}
}

func awaitLockFIFOReply(t *testing.T, directory *os.Root, result <-chan lockFIFOReply) lockFIFOReply {
	t.Helper()

	timer := time.NewTimer(lockFIFOTimeout)
	defer timer.Stop()

	select {
	case reply := <-result:
		return reply
	case <-timer.C:
		t.Errorf("lock command did not return within %s", lockFIFOTimeout)
	}

	return finishTimedOutLockFIFO(t, directory, result)
}

func finishTimedOutLockFIFO(t *testing.T, directory *os.Root, result <-chan lockFIFOReply) lockFIFOReply {
	t.Helper()

	timer := time.NewTimer(lockFIFOTimeout)
	defer timer.Stop()

	probe := time.NewTicker(10 * time.Millisecond)
	defer probe.Stop()

	for {
		select {
		case reply := <-result:
			return reply
		case <-timer.C:
			t.Fatal("lock command did not return during FIFO cleanup")

			return lockFIFOReply{exit: 0, stdout: "", stderr: ""}
		case <-probe.C:
			writer, err := directory.OpenFile("otelplan.lock", os.O_WRONLY|syscall.O_NONBLOCK, 0)
			if errors.Is(err, syscall.ENXIO) {
				continue
			}

			requireTestSuccess(t, err, "open lockfile FIFO for cleanup")
			requireTestSuccess(t, writer.Close(), "release lockfile FIFO reader")
			t.Log("released a lockfile FIFO reader after the deadline")
		}
	}
}
