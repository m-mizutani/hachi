// Package cli implements the hachi command line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/errs"
	"github.com/m-mizutani/hachi/internal/waiter"
	"github.com/urfave/cli/v3"
)

// Process exit codes.
const (
	ExitSuccess      = 0
	ExitTimeout      = 1
	ExitFailMatched  = 2
	ExitInvalidUsage = 3
	ExitCommandStart = 4
	ExitInterrupted  = 130
)

// Run parses args, runs the wait loop and returns the process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version string) int {
	stopAt := 1
	cmd := &cli.Command{
		Name:            "hachi",
		Usage:           "Run a command repeatedly until its result satisfies the given conditions",
		UsageText:       "hachi [options] [--] <command> [args...]",
		Description:     helpDescription,
		Version:         version,
		Writer:          stdout,
		ErrWriter:       stderr,
		HideHelpCommand: true,
		// Flags after the command name belong to the command, so `--` is optional.
		StopOnNthArg: &stopAt,
		// Exit codes are decided by Run; urfave/cli must not call os.Exit.
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		// Without this, urfave/cli prints the whole help to stdout on a usage
		// error, mixing it into the stream reserved for the command output.
		OnUsageError: func(_ context.Context, _ *cli.Command, err error, _ bool) error {
			return goerr.Wrap(err, "", goerr.T(errs.TagInvalidArgument))
		},
		Flags: flags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return action(ctx, cmd, stdout, stderr)
		},
	}

	err := cmd.Run(ctx, args)
	if err == nil {
		return ExitSuccess
	}
	fmt.Fprintf(stderr, "hachi: %s\n", errorMessage(err))
	return exitCode(err)
}

func action(ctx context.Context, cmd *cli.Command, stdout, stderr io.Writer) error {
	handler := slog.DiscardHandler
	if !cmd.Bool(flagQuiet) {
		handler = slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	logger := slog.New(handler)

	cfg, err := buildConfig(cmd, logger)
	if err != nil {
		return err
	}

	outcome, waitErr := waiter.Wait(ctx, *cfg)
	if outcome != nil && outcome.Last != nil {
		// A write failure is reported but does not replace the result of the wait:
		// the exit code still tells whether the conditions were satisfied.
		if _, err := stdout.Write(outcome.Last.Stdout); err != nil {
			fmt.Fprintf(stderr, "hachi: failed to write command output to stdout: %s\n", err)
		}
		if _, err := stderr.Write(outcome.Last.Stderr); err != nil {
			fmt.Fprintf(stderr, "hachi: failed to write command output to stderr: %s\n", err)
		}
	}
	return waitErr
}

func exitCode(err error) int {
	switch {
	case goerr.HasTag(err, errs.TagTimeout):
		return ExitTimeout
	case goerr.HasTag(err, errs.TagFailConditionMatched):
		return ExitFailMatched
	case goerr.HasTag(err, errs.TagCommandStart):
		return ExitCommandStart
	case goerr.HasTag(err, errs.TagInterrupted):
		return ExitInterrupted
	default:
		// TagInvalidArgument, and untagged errors from urfave/cli flag parsing.
		return ExitInvalidUsage
	}
}

func errorMessage(err error) string {
	values := goerr.Values(err)
	switch {
	case goerr.HasTag(err, errs.TagTimeout):
		return fmt.Sprintf("timeout after %v (attempts: %v)", values["timeout"], values["attempts"])
	case goerr.HasTag(err, errs.TagFailConditionMatched):
		return fmt.Sprintf("fail condition matched: %v (attempts: %v)", values["condition"], values["attempts"])
	case goerr.HasTag(err, errs.TagInterrupted):
		return fmt.Sprintf("interrupted (attempts: %v)", values["attempts"])
	}

	msg := err.Error()
	// Values already shown in the message (e.g. the expression in a parse error) are not repeated.
	var pairs []string
	for _, k := range slices.Sorted(maps.Keys(values)) {
		v := fmt.Sprint(values[k])
		if !strings.Contains(msg, v) {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if len(pairs) > 0 {
		msg = fmt.Sprintf("%s (%s)", msg, strings.Join(pairs, ", "))
	}
	if exitCode(err) == ExitInvalidUsage {
		msg += "\nRun 'hachi --help' for usage."
	}
	return msg
}
