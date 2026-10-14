//go:build !unix

package runner

import (
	"os/exec"
	"time"
)

// prepareStop falls back to the os/exec default on platforms without process
// groups: the direct child is killed and child processes it started may remain.
func prepareStop(c *exec.Cmd, grace time.Duration) func() {
	c.WaitDelay = grace
	return func() {}
}
