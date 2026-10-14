package waiter_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/condition"
	"github.com/m-mizutani/hachi/internal/errs"
	"github.com/m-mizutani/hachi/internal/runner"
	"github.com/m-mizutani/hachi/internal/waiter"
)

// fixedWait waits the same duration and records the attempt numbers it was asked about.
type fixedWait struct {
	d        time.Duration
	attempts []int
}

func (f *fixedWait) Next(attempt int) time.Duration {
	f.attempts = append(f.attempts, attempt)
	return f.d
}

// funcCond adapts a function to condition.Condition.
type funcCond struct {
	name string
	fn   func(in *condition.Input) (bool, error)
}

func (c *funcCond) Name() string { return c.name }
func (c *funcCond) Match(_ context.Context, in *condition.Input) (bool, error) {
	return c.fn(in)
}

// stdoutIs matches when stdout equals s.
func stdoutIs(name, s string) condition.Condition {
	return &funcCond{name: name, fn: func(in *condition.Input) (bool, error) {
		return string(in.Stdout) == s, nil
	}}
}

// sequence returns a RunFunc whose n-th call (1-origin) prints outputs[n-1],
// repeating the last element afterwards.
func sequence(outputs ...string) (runner.RunFunc, *int) {
	calls := 0
	return func(context.Context, []string) (*runner.Result, error) {
		calls++
		i := min(calls, len(outputs)) - 1
		return &runner.Result{Stdout: []byte(outputs[i])}, nil
	}, &calls
}

// blockUntilDone returns a RunFunc that never finishes on its own.
func blockUntilDone(ctx context.Context, _ []string) (*runner.Result, error) {
	<-ctx.Done()
	return nil, goerr.Wrap(ctx.Err(), "command was stopped")
}

func baseConfig(run runner.RunFunc, success ...condition.Condition) waiter.Config {
	return waiter.Config{
		Command:  []string{"cmd"},
		Timeout:  5 * time.Second,
		Strategy: &fixedWait{d: time.Millisecond},
		Success:  success,
		Run:      run,
	}
}

func TestWaitSucceedsOnThirdAttempt(t *testing.T) {
	run, _ := sequence("a", "b", "ready")
	st := &fixedWait{d: time.Millisecond}
	cfg := baseConfig(run, stdoutIs("ready", "ready"))
	cfg.Strategy = st

	out, err := waiter.Wait(context.Background(), cfg)
	gt.NoError(t, err)
	gt.V(t, out.Attempts).Equal(3)
	gt.V(t, string(out.Last.Stdout)).Equal("ready")
	gt.A(t, st.attempts).Equal([]int{1, 2})
}

func TestWaitFailConditionMatched(t *testing.T) {
	run, _ := sequence("failed")
	cfg := baseConfig(run, stdoutIs("ready", "ready"))
	cfg.Fail = []condition.Condition{stdoutIs("failed", "failed")}

	out, err := waiter.Wait(context.Background(), cfg)
	gt.Error(t, err)
	gt.True(t, goerr.HasTag(err, errs.TagFailConditionMatched))
	gt.V(t, goerr.Values(err)["condition"]).Equal(any("failed"))
	gt.V(t, out.Attempts).Equal(1)
	gt.V(t, string(out.Last.Stdout)).Equal("failed")
}

func TestWaitFailTakesPrecedenceOverSuccess(t *testing.T) {
	run, _ := sequence("both")
	cfg := baseConfig(run, stdoutIs("success", "both"))
	cfg.Fail = []condition.Condition{stdoutIs("fail", "both")}

	_, err := waiter.Wait(context.Background(), cfg)
	gt.True(t, goerr.HasTag(err, errs.TagFailConditionMatched))
}

func TestWaitSuccessConditionsAreAND(t *testing.T) {
	run, calls := sequence("a", "ab", "abc")
	hasA := &funcCond{name: "a", fn: func(in *condition.Input) (bool, error) {
		return strings.Contains(string(in.Stdout), "a"), nil
	}}
	hasC := &funcCond{name: "c", fn: func(in *condition.Input) (bool, error) {
		return strings.Contains(string(in.Stdout), "c"), nil
	}}

	out, err := waiter.Wait(context.Background(), baseConfig(run, hasA, hasC))
	gt.NoError(t, err)
	gt.V(t, out.Attempts).Equal(3)
	gt.V(t, *calls).Equal(3)
}

func TestWaitFailConditionsAreOR(t *testing.T) {
	run, _ := sequence("x")
	cfg := baseConfig(run, stdoutIs("never", "never"))
	cfg.Fail = []condition.Condition{stdoutIs("no", "no"), stdoutIs("x", "x")}

	_, err := waiter.Wait(context.Background(), cfg)
	gt.True(t, goerr.HasTag(err, errs.TagFailConditionMatched))
	gt.V(t, goerr.Values(err)["condition"]).Equal(any("x"))
}

func TestWaitTimeout(t *testing.T) {
	run, calls := sequence("pending")
	cfg := baseConfig(run, stdoutIs("ready", "ready"))
	cfg.Timeout = 50 * time.Millisecond
	cfg.Strategy = &fixedWait{d: 10 * time.Millisecond}

	start := time.Now()
	out, err := waiter.Wait(context.Background(), cfg)
	elapsed := time.Since(start)

	gt.True(t, goerr.HasTag(err, errs.TagTimeout))
	gt.True(t, elapsed >= 50*time.Millisecond)
	gt.True(t, elapsed < time.Second)
	gt.V(t, string(out.Last.Stdout)).Equal("pending")
	gt.True(t, *calls >= 2)
}

func TestWaitTimeoutShorterThanInterval(t *testing.T) {
	run, calls := sequence("pending")
	cfg := baseConfig(run, stdoutIs("ready", "ready"))
	cfg.Timeout = 50 * time.Millisecond
	cfg.Strategy = &fixedWait{d: time.Hour}

	start := time.Now()
	_, err := waiter.Wait(context.Background(), cfg)
	gt.True(t, goerr.HasTag(err, errs.TagTimeout))
	gt.True(t, time.Since(start) < time.Second)
	gt.V(t, *calls).Equal(1)
}

func TestWaitTimeoutDuringAttempt(t *testing.T) {
	cfg := baseConfig(blockUntilDone, stdoutIs("ready", "ready"))
	cfg.Timeout = 50 * time.Millisecond

	out, err := waiter.Wait(context.Background(), cfg)
	gt.True(t, goerr.HasTag(err, errs.TagTimeout))
	gt.Nil(t, out.Last)
	gt.V(t, out.Attempts).Equal(1)
}

func TestWaitTimeoutDuringConditionEvaluation(t *testing.T) {
	run, _ := sequence(`{}`)
	endless := gt.R1(condition.NewJQ(`last(repeat(.))`, condition.TargetStdout)).NoError(t)
	var logs bytes.Buffer
	cfg := baseConfig(run, endless)
	cfg.Timeout = 100 * time.Millisecond
	cfg.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	start := time.Now()
	out, err := waiter.Wait(context.Background(), cfg)
	gt.True(t, goerr.HasTag(err, errs.TagTimeout))
	gt.True(t, time.Since(start) < 2*time.Second)
	gt.V(t, out.Attempts).Equal(1)
	gt.S(t, logs.String()).NotContains("condition evaluation failed")
}

func TestWaitAttemptTimeout(t *testing.T) {
	calls := 0
	run := func(ctx context.Context, argv []string) (*runner.Result, error) {
		calls++
		if calls == 1 {
			return blockUntilDone(ctx, argv)
		}
		return &runner.Result{Stdout: []byte("ready")}, nil
	}
	var logs bytes.Buffer
	cfg := baseConfig(run, stdoutIs("ready", "ready"))
	cfg.AttemptTimeout = 20 * time.Millisecond
	cfg.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	out, err := waiter.Wait(context.Background(), cfg)
	gt.NoError(t, err)
	gt.V(t, out.Attempts).Equal(2)
	gt.V(t, strings.Count(logs.String(), `msg="attempt timed out"`)).Equal(1)
}

func TestWaitInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := func(ctx context.Context, argv []string) (*runner.Result, error) {
		cancel()
		return blockUntilDone(ctx, argv)
	}

	_, err := waiter.Wait(ctx, baseConfig(run, stdoutIs("ready", "ready")))
	gt.True(t, goerr.HasTag(err, errs.TagInterrupted))
}

func TestWaitInterruptedWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := func(context.Context, []string) (*runner.Result, error) {
		cancel()
		return &runner.Result{Stdout: []byte("pending")}, nil
	}
	cfg := baseConfig(run, stdoutIs("ready", "ready"))
	cfg.Strategy = &fixedWait{d: time.Hour}

	out, err := waiter.Wait(ctx, cfg)
	gt.True(t, goerr.HasTag(err, errs.TagInterrupted))
	gt.V(t, string(out.Last.Stdout)).Equal("pending")
}

func TestWaitCommandStartFailureIsNotRetried(t *testing.T) {
	calls := 0
	run := func(context.Context, []string) (*runner.Result, error) {
		calls++
		return nil, goerr.New("not found", goerr.T(errs.TagCommandStart))
	}

	_, err := waiter.Wait(context.Background(), baseConfig(run, stdoutIs("ready", "ready")))
	gt.True(t, goerr.HasTag(err, errs.TagCommandStart))
	gt.V(t, calls).Equal(1)
}

func TestWaitConditionErrorIsTreatedAsNoMatch(t *testing.T) {
	run, _ := sequence("not json", "ready")
	failing := &funcCond{name: "jq", fn: func(in *condition.Input) (bool, error) {
		if string(in.Stdout) == "not json" {
			return false, goerr.New("output is not JSON")
		}
		return true, nil
	}}
	var logs bytes.Buffer
	cfg := baseConfig(run, failing)
	cfg.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	out, err := waiter.Wait(context.Background(), cfg)
	gt.NoError(t, err)
	gt.V(t, out.Attempts).Equal(2)
	gt.S(t, logs.String()).Contains(`msg="condition evaluation failed"`)
	gt.S(t, logs.String()).Contains(`condition=jq`)
	gt.S(t, logs.String()).Contains(`kind=success`)
}

func TestWaitLogsAttempts(t *testing.T) {
	run, _ := sequence("a", "ready")
	var logs bytes.Buffer
	cfg := baseConfig(run, stdoutIs("ready", "ready"))
	cfg.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	gt.R1(waiter.Wait(context.Background(), cfg)).NoError(t)
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	gt.A(t, lines).Length(2)
	gt.S(t, lines[0]).Contains("attempt=1").Contains("matched=false").Contains("next_in=1ms")
	gt.S(t, lines[1]).Contains("attempt=2").Contains("matched=true").NotContains("next_in")
}

func TestWaitInvalidConfig(t *testing.T) {
	run, _ := sequence("x")
	valid := func() waiter.Config { return baseConfig(run, stdoutIs("x", "x")) }

	cases := map[string]func(*waiter.Config){
		"empty command":            func(c *waiter.Config) { c.Command = nil },
		"empty command name":       func(c *waiter.Config) { c.Command = []string{""} },
		"zero timeout":             func(c *waiter.Config) { c.Timeout = 0 },
		"negative attempt timeout": func(c *waiter.Config) { c.AttemptTimeout = -time.Second },
		"no strategy":              func(c *waiter.Config) { c.Strategy = nil },
		"no success condition":     func(c *waiter.Config) { c.Success = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := valid()
			mutate(&cfg)
			out, err := waiter.Wait(context.Background(), cfg)
			gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
			gt.V(t, out.Attempts).Equal(0)
		})
	}
}
