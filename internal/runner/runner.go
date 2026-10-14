// Package runner executes the target command once and captures its result.
package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/errs"
)

// Result is the outcome of one command execution.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int // ProcessState.ExitCode(); -1 when the process was killed by a signal
	Duration time.Duration
}

// RunFunc executes argv once. A non-zero exit status is not an error.
type RunFunc func(ctx context.Context, argv []string) (*Result, error)

// stopGracePeriod is how long a command may keep running after SIGTERM before
// it is killed. It is a variable so that tests can shorten it.
var stopGracePeriod = 5 * time.Second

// Run starts argv[0] with argv[1:], waits for it and captures its output.
// When ctx is done, the command and the processes it started receive SIGTERM
// and then SIGKILL after stopGracePeriod, and Run returns an error wrapping ctx.Err().
func Run(ctx context.Context, argv []string) (*Result, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, goerr.New("command is empty", goerr.T(errs.TagInvalidArgument))
	}

	var stdout, stderr bytes.Buffer
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Stdin = nil
	c.Stdout = &stdout
	c.Stderr = &stderr
	finishStop := prepareStop(c, stopGracePeriod)

	start := time.Now()
	if err := c.Start(); err != nil {
		// Start fails with ctx.Err() when ctx is already done; that is a stop, not a start failure.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, goerr.Wrap(ctxErr, "command was stopped", goerr.V("command", argv[0]))
		}
		return nil, goerr.Wrap(err, "failed to start command",
			goerr.T(errs.TagCommandStart), goerr.V("command", argv[0]))
	}

	waitErr := c.Wait()
	finishStop()
	duration := time.Since(start)

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, goerr.Wrap(ctxErr, "command was stopped",
			goerr.V("command", argv[0]), goerr.V("duration", duration))
	}

	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		// Not an exit status but a failure of the wait itself (e.g. copying output);
		// retrying does not help, so it is reported like a start failure.
		return nil, goerr.Wrap(waitErr, "failed to wait for command",
			goerr.T(errs.TagCommandStart), goerr.V("command", argv[0]))
	}

	return &Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: c.ProcessState.ExitCode(),
		Duration: duration,
	}, nil
}
