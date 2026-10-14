package condition

import "context"

type exitCode struct {
	code int
}

// NewExitCode matches when the command exits with code.
func NewExitCode(code int) Condition {
	return &exitCode{code: code}
}

func (c *exitCode) Name() string { return "exit-code" }

func (c *exitCode) Match(_ context.Context, in *Input) (bool, error) {
	return in.ExitCode == c.code, nil
}
