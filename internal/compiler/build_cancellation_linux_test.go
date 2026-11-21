package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestPreparedBuildCancellationStopsDescendants(t *testing.T) {
	t.Parallel()

	request := preparedBuildFixture(t)
	marker := filepath.Join(t.TempDir(), "child")
	request.Env = append(request.Env, "OTELPLAN_TEST_MODE=child", "OTELPLAN_TEST_CHILD="+marker,
		"OTELPLAN_TEST_READY="+filepath.Join(t.TempDir(), "ready"))

	ctx, cancel := context.WithCancel(t.Context())
	completed := make(chan error, 1)
	consumed := false
	pid := 0

	t.Cleanup(func() {
		cancel()

		if !consumed {
			<-completed
		}

		cleanupBackendChild(t, pid, request.Executable)
	})

	go func() { completed <- compiler.BuildPrepared(ctx, request) }()

	waitForPreparedBuildStart(t, marker, completed, &consumed)

	pid = readBackendChildPID(t, marker)
	if !backendChildRunning(t, pid, request.Executable) {
		t.Fatal("backend child did not become runnable")
	}

	cancel()

	select {
	case err := <-completed:
		consumed = true

		if !errors.Is(err, context.Canceled) {
			t.Fatal("backend cancellation lost context identity", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("backend cancellation did not return")
	}

	deadline := time.Now().Add(5 * time.Second)
	for backendChildRunning(t, pid, request.Executable) {
		if time.Now().After(deadline) {
			t.Fatal("cancelled build left its backend child running")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func backendChildRunning(t *testing.T, pid int, executable string) bool {
	t.Helper()

	directory, err := os.OpenRoot(fmt.Sprintf("/proc/%d", pid))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	if err != nil {
		t.Fatal("open owned backend child state", err)
	}

	defer closeBackendChildState(t, directory)

	actual, err := directory.Readlink("exe")
	if backendChildExited(err) {
		return false
	}

	if err != nil {
		t.Fatal("inspect owned backend child", err)
	}

	if actual != executable {
		return false
	}

	data, err := directory.ReadFile("stat")
	if backendChildExited(err) {
		return false
	}

	if err != nil {
		t.Fatal("inspect backend child state", err)
	}

	closing := bytes.LastIndexByte(data, ')')
	state := strings.Fields(string(data[closing+1:]))

	return len(state) > 0 && state[0] != "Z" && state[0] != "X"
}

func backendChildExited(err error) bool {
	// The process can exit while its /proc directory is open.
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH)
}

func readBackendChildPID(t *testing.T, marker string) int {
	t.Helper()

	directory, err := os.OpenRoot(filepath.Dir(marker))
	if err != nil {
		t.Fatal(err)
	}

	defer closeBackendChildState(t, directory)

	data, err := directory.ReadFile(filepath.Base(marker))
	if err != nil {
		t.Fatal(err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		t.Fatal("invalid backend child identity", err)
	}

	return pid
}

func cleanupBackendChild(t *testing.T, pid int, executable string) {
	t.Helper()

	if pid <= 0 || !backendChildRunning(t, pid, executable) {
		return
	}

	child, err := os.FindProcess(pid)
	if err != nil {
		t.Errorf("find owned backend child: %v", err)

		return
	}

	err = child.Kill()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("stop owned backend child: %v", err)
	}

	err = child.Release()
	if err != nil {
		t.Errorf("release backend child handle: %v", err)
	}
}

func closeBackendChildState(t *testing.T, directory *os.Root) {
	t.Helper()

	err := directory.Close()
	if err != nil {
		t.Error("close backend process state", err)
	}
}
