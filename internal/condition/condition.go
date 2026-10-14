// Package condition evaluates the result of one command execution.
package condition

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/errs"
)

// Input is the result of one command execution to be inspected.
type Input struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Condition reports whether a command result satisfies it.
// A non-nil error means the condition could not be evaluated for this input.
type Condition interface {
	Name() string
	Match(ctx context.Context, in *Input) (bool, error)
}

// Target selects which output grep, regex and jq conditions inspect.
type Target string

const (
	TargetStdout   Target = "stdout"
	TargetStderr   Target = "stderr"
	TargetCombined Target = "combined"
)

// ParseTarget validates a target name.
func ParseTarget(s string) (Target, error) {
	switch Target(s) {
	case TargetStdout, TargetStderr, TargetCombined:
		return Target(s), nil
	default:
		return "", goerr.New("unknown target",
			goerr.T(errs.TagInvalidArgument),
			goerr.V("target", s),
			goerr.V("allowed", []Target{TargetStdout, TargetStderr, TargetCombined}))
	}
}

// bytes returns the inspected output. Combined is stdout followed by stderr
// because the two streams are captured separately and their interleaving is lost.
func (t Target) bytes(in *Input) []byte {
	switch t {
	case TargetStderr:
		return in.Stderr
	case TargetCombined:
		out := make([]byte, 0, len(in.Stdout)+len(in.Stderr))
		out = append(out, in.Stdout...)
		return append(out, in.Stderr...)
	default:
		return in.Stdout
	}
}
