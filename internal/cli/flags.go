package cli

import (
	"log/slog"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/condition"
	"github.com/m-mizutani/hachi/internal/errs"
	"github.com/m-mizutani/hachi/internal/strategy"
	"github.com/m-mizutani/hachi/internal/waiter"
	"github.com/urfave/cli/v3"
)

const (
	flagTimeout         = "timeout"
	flagWaitStrategy    = "wait-strategy"
	flagInitialInterval = "initial-interval"
	flagMaxInterval     = "max-interval"
	flagMultiplier      = "multiplier"
	flagInterval        = "interval"
	flagAttemptTimeout  = "attempt-timeout"
	flagTarget          = "target"
	flagExitCode        = "exit-code"
	flagGrep            = "grep"
	flagRegex           = "regex"
	flagJQ              = "jq"
	flagExpr            = "expr"
	flagFailExitCode    = "fail-exit-code"
	flagFailGrep        = "fail-grep"
	flagFailRegex       = "fail-regex"
	flagFailJQ          = "fail-jq"
	flagFailExpr        = "fail-expr"
	flagQuiet           = "quiet"
)

func flags() []cli.Flag {
	return []cli.Flag{
		&cli.DurationFlag{
			Name: flagTimeout, Aliases: []string{"t"}, Value: 30 * time.Minute,
			Usage:   "give up and exit with code 1 after this duration",
			Sources: cli.EnvVars("HACHI_TIMEOUT"),
		},
		&cli.StringFlag{
			Name: flagWaitStrategy, Aliases: []string{"w"}, Value: string(strategy.NameBackoff),
			Usage:   "how to wait between attempts: backoff or fixed",
			Sources: cli.EnvVars("HACHI_WAIT_STRATEGY"),
		},
		&cli.DurationFlag{
			Name: flagInitialInterval, Value: time.Second,
			Usage:   "backoff: wait after the first attempt",
			Sources: cli.EnvVars("HACHI_INITIAL_INTERVAL"),
		},
		&cli.DurationFlag{
			Name: flagMaxInterval, Value: time.Minute,
			Usage:   "backoff: upper limit of the wait",
			Sources: cli.EnvVars("HACHI_MAX_INTERVAL"),
		},
		&cli.FloatFlag{
			Name: flagMultiplier, Value: 2,
			Usage:   "backoff: factor applied to the wait after each attempt",
			Sources: cli.EnvVars("HACHI_MULTIPLIER"),
		},
		&cli.DurationFlag{
			Name: flagInterval, Value: 10 * time.Second,
			Usage:   "fixed: wait between attempts",
			Sources: cli.EnvVars("HACHI_INTERVAL"),
		},
		&cli.DurationFlag{
			Name: flagAttemptTimeout, Value: 0,
			Usage:   "stop a single attempt after this duration and retry (0 means no limit)",
			Sources: cli.EnvVars("HACHI_ATTEMPT_TIMEOUT"),
		},
		&cli.StringFlag{
			Name: flagTarget, Value: string(condition.TargetStdout),
			Usage:   "output inspected by grep, regex and jq conditions: stdout, stderr or combined",
			Sources: cli.EnvVars("HACHI_TARGET"),
		},
		// Condition flags are OnlyOnce: urfave/cli otherwise keeps only the last
		// value, so `--grep a --grep b` would silently drop the first condition.
		&cli.IntFlag{
			Name:        flagExitCode,
			HideDefault: true, // unset means "no exit code condition", not 0
			OnlyOnce:    true,
			Usage:       "success condition: the exit code equals this value",
			Sources:     cli.EnvVars("HACHI_EXIT_CODE"),
		},
		&cli.StringFlag{
			Name:     flagGrep,
			OnlyOnce: true,
			Usage:    "success condition: the output contains this string as is (not a regular expression; use --regex for patterns)",
			Sources:  cli.EnvVars("HACHI_GREP"),
		},
		&cli.StringFlag{
			Name:     flagRegex,
			OnlyOnce: true,
			Usage:    "success condition: the output matches this regular expression (RE2, ^ and $ match at line boundaries)",
			Sources:  cli.EnvVars("HACHI_REGEX"),
		},
		&cli.StringFlag{
			Name:     flagJQ,
			OnlyOnce: true,
			Usage:    "success condition: the last result of this jq query on the JSON output is neither false nor null",
			Sources:  cli.EnvVars("HACHI_JQ"),
		},
		&cli.StringFlag{
			Name:     flagExpr,
			OnlyOnce: true,
			Usage:    "success condition: this expr-lang expression is true",
			Sources:  cli.EnvVars("HACHI_EXPR"),
		},
		&cli.IntFlag{
			Name:        flagFailExitCode,
			HideDefault: true,
			OnlyOnce:    true,
			Usage:       "fail condition: the exit code equals this value",
			Sources:     cli.EnvVars("HACHI_FAIL_EXIT_CODE"),
		},
		&cli.StringFlag{
			Name:     flagFailGrep,
			OnlyOnce: true,
			Usage:    "fail condition: the output contains this string as is (not a regular expression; use --fail-regex for patterns)",
			Sources:  cli.EnvVars("HACHI_FAIL_GREP"),
		},
		&cli.StringFlag{
			Name:     flagFailRegex,
			OnlyOnce: true,
			Usage:    "fail condition: the output matches this regular expression",
			Sources:  cli.EnvVars("HACHI_FAIL_REGEX"),
		},
		&cli.StringFlag{
			Name:     flagFailJQ,
			OnlyOnce: true,
			Usage:    "fail condition: the last result of this jq query is neither false nor null",
			Sources:  cli.EnvVars("HACHI_FAIL_JQ"),
		},
		&cli.StringFlag{
			Name:     flagFailExpr,
			OnlyOnce: true,
			Usage:    "fail condition: this expr-lang expression is true",
			Sources:  cli.EnvVars("HACHI_FAIL_EXPR"),
		},
		&cli.BoolFlag{
			Name: flagQuiet, Aliases: []string{"q"},
			Usage:   "do not log each attempt",
			Sources: cli.EnvVars("HACHI_QUIET"),
		},
	}
}

// conditionFlags lists the flag names of one condition kind in evaluation order.
type conditionFlags struct {
	exitCode, grep, regex, jq, expr string
}

var (
	successFlags = conditionFlags{flagExitCode, flagGrep, flagRegex, flagJQ, flagExpr}
	failFlags    = conditionFlags{flagFailExitCode, flagFailGrep, flagFailRegex, flagFailJQ, flagFailExpr}
)

func buildConfig(cmd *cli.Command, logger *slog.Logger) (*waiter.Config, error) {
	argv := cmd.Args().Slice()
	if len(argv) == 0 || argv[0] == "" {
		return nil, goerr.New("command is required: hachi [options] [--] <command> [args...]",
			goerr.T(errs.TagInvalidArgument))
	}

	st, err := buildStrategy(cmd, logger)
	if err != nil {
		return nil, err
	}

	target, err := condition.ParseTarget(cmd.String(flagTarget))
	if err != nil {
		return nil, goerr.Wrap(err, "--"+flagTarget)
	}

	success, err := buildConditions(cmd, successFlags, target)
	if err != nil {
		return nil, err
	}
	if len(success) == 0 {
		success = []condition.Condition{condition.NewExitCode(0)}
	}
	fail, err := buildConditions(cmd, failFlags, target)
	if err != nil {
		return nil, err
	}

	timeout := cmd.Duration(flagTimeout)
	if timeout <= 0 {
		return nil, goerr.New("--"+flagTimeout+" must be positive",
			goerr.T(errs.TagInvalidArgument), goerr.V("timeout", timeout))
	}
	attemptTimeout := cmd.Duration(flagAttemptTimeout)
	if attemptTimeout < 0 {
		return nil, goerr.New("--"+flagAttemptTimeout+" must not be negative",
			goerr.T(errs.TagInvalidArgument), goerr.V("attempt_timeout", attemptTimeout))
	}

	return &waiter.Config{
		Command:        argv,
		Timeout:        timeout,
		AttemptTimeout: attemptTimeout,
		Strategy:       st,
		Success:        success,
		Fail:           fail,
		Logger:         logger,
	}, nil
}

func buildStrategy(cmd *cli.Command, logger *slog.Logger) (strategy.Strategy, error) {
	name, err := strategy.ParseName(cmd.String(flagWaitStrategy))
	if err != nil {
		return nil, goerr.Wrap(err, "--"+flagWaitStrategy)
	}

	// The flags of the other strategy are ignored rather than rejected so that
	// defaults set through HACHI_* variables do not block switching strategies.
	warnIgnored := func(names ...string) {
		for _, n := range names {
			if cmd.IsSet(n) {
				logger.Warn("flag is ignored", slog.String("flag", n), slog.String("wait_strategy", string(name)))
			}
		}
	}

	switch name {
	case strategy.NameFixed:
		warnIgnored(flagInitialInterval, flagMaxInterval, flagMultiplier)
		return strategy.NewFixed(cmd.Duration(flagInterval))
	default:
		warnIgnored(flagInterval)
		return strategy.NewBackoff(cmd.Duration(flagInitialInterval), cmd.Duration(flagMaxInterval), cmd.Float(flagMultiplier))
	}
}

func buildConditions(cmd *cli.Command, f conditionFlags, target condition.Target) ([]condition.Condition, error) {
	var conds []condition.Condition

	if cmd.IsSet(f.exitCode) {
		code := cmd.Int(f.exitCode)
		if code < 0 || code > 255 {
			return nil, goerr.New("--"+f.exitCode+" must be between 0 and 255",
				goerr.T(errs.TagInvalidArgument), goerr.V("exit_code", code))
		}
		conds = append(conds, condition.NewExitCode(code))
	}

	// An empty string would match every output, so it is treated as unset.
	// urfave/cli marks a string flag as set even when its environment variable is empty.
	if s := cmd.String(f.grep); s != "" {
		conds = append(conds, condition.NewGrep(s, target))
	}

	builders := []struct {
		flag  string
		build func(string) (condition.Condition, error)
	}{
		{f.regex, func(s string) (condition.Condition, error) { return condition.NewRegex(s, target) }},
		{f.jq, func(s string) (condition.Condition, error) { return condition.NewJQ(s, target) }},
		{f.expr, condition.NewExpr},
	}
	for _, b := range builders {
		s := cmd.String(b.flag)
		if s == "" {
			continue
		}
		c, err := b.build(s)
		if err != nil {
			return nil, goerr.Wrap(err, "--"+b.flag)
		}
		conds = append(conds, c)
	}

	return conds, nil
}
