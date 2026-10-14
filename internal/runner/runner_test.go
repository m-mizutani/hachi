package runner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/errs"
	"github.com/m-mizutani/hachi/internal/runner"
)

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	res := gt.R1(runner.Run(context.Background(),
		[]string{"sh", "-c", "echo out; echo err >&2; exit 3"})).NoError(t)
	gt.V(t, string(res.Stdout)).Equal("out\n")
	gt.V(t, string(res.Stderr)).Equal("err\n")
	gt.V(t, res.ExitCode).Equal(3)
	gt.True(t, res.Duration > 0)
}

func TestRunCommandNotFound(t *testing.T) {
	_, err := runner.Run(context.Background(), []string{"hachi-no-such-command"})
	gt.Error(t, err)
	gt.True(t, goerr.HasTag(err, errs.TagCommandStart))
}

func TestRunEmptyCommand(t *testing.T) {
	_, err := runner.Run(context.Background(), nil)
	gt.Error(t, err)
	gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
}

func TestRunStdinIsEmpty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := gt.R1(runner.Run(ctx, []string{"cat"})).NoError(t)
	gt.V(t, len(res.Stdout)).Equal(0)
	gt.V(t, res.ExitCode).Equal(0)
}

func TestRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runner.Run(ctx, []string{"sleep", "10"})
	gt.Error(t, err)
	gt.True(t, errors.Is(err, context.DeadlineExceeded))
	gt.True(t, time.Since(start) < time.Second)
}

func TestRunWithDoneContextIsNotStartFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner.Run(ctx, []string{"true"})
	gt.Error(t, err)
	gt.True(t, errors.Is(err, context.Canceled))
	gt.False(t, goerr.HasTag(err, errs.TagCommandStart))
}
