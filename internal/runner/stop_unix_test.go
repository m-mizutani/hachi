//go:build unix

package runner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/runner"
)

// startWithChild runs a shell that starts `sleep 30` in the background, records
// its PID and waits for it. trapTerm makes both ignore SIGTERM.
func startWithChild(t *testing.T, ctx context.Context, trapTerm bool) (pid int, elapsed time.Duration) {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "pid")
	script := fmt.Sprintf(`sleep 30 & echo $! > %s; wait`, pidFile)
	if trapTerm {
		script = `trap "" TERM; ` + script
	}

	start := time.Now()
	_, err := runner.Run(ctx, []string{"sh", "-c", script})
	elapsed = time.Since(start)
	gt.Error(t, err)

	data := gt.R1(os.ReadFile(pidFile)).NoError(t)
	return gt.R1(strconv.Atoi(strings.TrimSpace(string(data)))).NoError(t), elapsed
}

// waitGone reports whether the process exits within one second. An orphaned
// process is reaped by init asynchronously, so it is polled.
func waitGone(pid int) bool {
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
	}
	return false
}

func TestStopTerminatesChildProcesses(t *testing.T) {
	defer runner.SetStopGracePeriod(2 * time.Second)()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	pid, elapsed := startWithChild(t, ctx, false)
	gt.True(t, waitGone(pid))
	// The group exited on SIGTERM, so Run does not wait for the grace period.
	gt.True(t, elapsed < 2*time.Second)
}

func TestStopKillsChildProcessesIgnoringSIGTERM(t *testing.T) {
	defer runner.SetStopGracePeriod(100 * time.Millisecond)()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	pid, elapsed := startWithChild(t, ctx, true)
	gt.True(t, waitGone(pid))
	gt.True(t, elapsed < 5*time.Second)
}
