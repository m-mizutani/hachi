//go:build unix

package runner

import (
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// prepareStop runs the command in its own process group so that processes it
// starts (e.g. the commands of a `sh -c` pipeline) are stopped together with it.
// os/exec only signals the direct child, which would leave those processes running.
//
// The returned function must be called after Wait returns. When the command
// was stopped by ctx, it waits until the process group exits or the grace
// period ends, and then kills what is left. It runs synchronously because hachi
// may exit right after Run returns, which would also discard a pending timer.
func prepareStop(c *exec.Cmd, grace time.Duration) func() {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.WaitDelay = grace

	var deadline atomic.Int64 // UnixNano; 0 until Cancel is called
	c.Cancel = func() error {
		deadline.Store(time.Now().Add(grace).UnixNano())
		return syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
	}

	return func() {
		d := deadline.Load()
		if d == 0 || c.Process == nil {
			return
		}
		pgid := c.Process.Pid
		for time.Now().UnixNano() < d {
			// Signal 0 only checks whether any process of the group still exists.
			if err := syscall.Kill(-pgid, 0); err != nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}
