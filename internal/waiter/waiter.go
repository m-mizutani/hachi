// Package waiter runs a command repeatedly until its result satisfies the
// success conditions, a fail condition matches, or the timeout is reached.
package waiter

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/condition"
	"github.com/m-mizutani/hachi/internal/errs"
	"github.com/m-mizutani/hachi/internal/runner"
	"github.com/m-mizutani/hachi/internal/strategy"
)

// Config describes one wait.
type Config struct {
	Command        []string
	Timeout        time.Duration // > 0
	AttemptTimeout time.Duration // 0 means no per-attempt limit
	Strategy       strategy.Strategy
	Success        []condition.Condition // at least one; all must match
	Fail           []condition.Condition // any match aborts
	Run            runner.RunFunc        // nil means runner.Run
	Logger         *slog.Logger          // nil means slog.New(slog.DiscardHandler)
}

// Outcome summarizes a finished wait.
type Outcome struct {
	Attempts int
	Last     *runner.Result // last attempt that finished on its own; nil if none
	Elapsed  time.Duration
}

func (c *Config) validate() error {
	switch {
	case len(c.Command) == 0 || c.Command[0] == "":
		return goerr.New("command is required", goerr.T(errs.TagInvalidArgument))
	case c.Timeout <= 0:
		return goerr.New("timeout must be positive",
			goerr.T(errs.TagInvalidArgument), goerr.V("timeout", c.Timeout))
	case c.AttemptTimeout < 0:
		return goerr.New("attempt timeout must not be negative",
			goerr.T(errs.TagInvalidArgument), goerr.V("attempt_timeout", c.AttemptTimeout))
	case c.Strategy == nil:
		return goerr.New("wait strategy is required", goerr.T(errs.TagInvalidArgument))
	case len(c.Success) == 0:
		return goerr.New("at least one success condition is required", goerr.T(errs.TagInvalidArgument))
	}
	return nil
}

type verdict int

const (
	verdictContinue verdict = iota
	verdictSuccess
	verdictFail
)

// Wait returns a non-nil Outcome even when it returns an error.
func Wait(ctx context.Context, cfg Config) (*Outcome, error) {
	start := time.Now()
	outcome := &Outcome{}
	finish := func(err error) (*Outcome, error) {
		outcome.Elapsed = time.Since(start)
		return outcome, err
	}

	if err := cfg.validate(); err != nil {
		return finish(err)
	}
	run := cfg.Run
	if run == nil {
		run = runner.Run
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	waitCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	for attempt := 1; ; attempt++ {
		outcome.Attempts = attempt

		res, err := runAttempt(waitCtx, run, cfg.Command, cfg.AttemptTimeout)
		switch {
		case err == nil:
			outcome.Last = res
			// waitCtx bounds condition evaluation too, e.g. a jq query that never ends.
			v, matched := evaluate(waitCtx, logger, attempt, res, cfg.Success, cfg.Fail)
			if waitCtx.Err() != nil {
				return finish(stopReason(ctx, cfg.Timeout, attempt))
			}
			switch v {
			case verdictSuccess:
				logAttempt(logger, attempt, res, true, nil)
				return finish(nil)
			case verdictFail:
				logAttempt(logger, attempt, res, false, nil)
				return finish(goerr.New("fail condition matched",
					goerr.T(errs.TagFailConditionMatched),
					goerr.V("condition", matched), goerr.V("attempts", attempt)))
			}
		case goerr.HasTag(err, errs.TagCommandStart):
			return finish(err)
		case waitCtx.Err() != nil:
			return finish(stopReason(ctx, cfg.Timeout, attempt))
		case errors.Is(err, context.DeadlineExceeded):
			logger.Warn("attempt timed out",
				slog.Int("attempt", attempt), slog.Duration("attempt_timeout", cfg.AttemptTimeout))
		default:
			return finish(err)
		}

		next := cfg.Strategy.Next(attempt)
		if err == nil {
			logAttempt(logger, attempt, res, false, &next)
		}

		timer := time.NewTimer(next)
		select {
		case <-timer.C:
		case <-waitCtx.Done():
			timer.Stop()
			return finish(stopReason(ctx, cfg.Timeout, attempt))
		}
	}
}

func runAttempt(ctx context.Context, run runner.RunFunc, argv []string, timeout time.Duration) (*runner.Result, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return run(ctx, argv)
}

// evaluate checks fail conditions first because missing a failure and moving on
// to later steps costs more than reporting a failure for a result that also succeeded.
// The returned string is the name of the fail condition that matched.
func evaluate(ctx context.Context, logger *slog.Logger, attempt int, res *runner.Result, success, fail []condition.Condition) (verdict, string) {
	in := &condition.Input{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: res.ExitCode}

	for _, c := range fail {
		if matchOrWarn(ctx, logger, attempt, "fail", c, in) {
			return verdictFail, c.Name()
		}
	}
	for _, c := range success {
		if !matchOrWarn(ctx, logger, attempt, "success", c, in) {
			return verdictContinue, ""
		}
	}
	return verdictSuccess, ""
}

func matchOrWarn(ctx context.Context, logger *slog.Logger, attempt int, kind string, c condition.Condition, in *condition.Input) bool {
	matched, err := c.Match(ctx, in)
	if err != nil {
		// An evaluation stopped by the timeout or an interruption is reported by the caller.
		if ctx.Err() != nil {
			return false
		}
		logger.Warn("condition evaluation failed",
			slog.Int("attempt", attempt),
			slog.String("condition", c.Name()),
			slog.String("kind", kind),
			slog.String("error", err.Error()))
		return false
	}
	return matched
}

func logAttempt(logger *slog.Logger, attempt int, res *runner.Result, matched bool, next *time.Duration) {
	attrs := []any{
		slog.Int("attempt", attempt),
		slog.Int("exit_code", res.ExitCode),
		slog.Duration("duration", res.Duration),
		slog.Bool("matched", matched),
	}
	if next != nil {
		attrs = append(attrs, slog.Duration("next_in", *next))
	}
	logger.Info("attempt finished", attrs...)
}

// stopReason tells an interruption of the parent context apart from reaching the timeout.
func stopReason(parent context.Context, timeout time.Duration, attempts int) error {
	if parent.Err() != nil {
		return goerr.Wrap(parent.Err(), "interrupted",
			goerr.T(errs.TagInterrupted), goerr.V("attempts", attempts))
	}
	return goerr.New("timeout",
		goerr.T(errs.TagTimeout), goerr.V("timeout", timeout), goerr.V("attempts", attempts))
}
