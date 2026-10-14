package condition

import (
	"bytes"
	"context"
	"regexp"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/errs"
)

type grep struct {
	substr []byte
	target Target
}

// NewGrep matches when the target output contains substr.
func NewGrep(substr string, target Target) Condition {
	return &grep{substr: []byte(substr), target: target}
}

func (c *grep) Name() string { return "grep" }

func (c *grep) Match(_ context.Context, in *Input) (bool, error) {
	return bytes.Contains(c.target.bytes(in), c.substr), nil
}

type regex struct {
	re     *regexp.Regexp
	target Target
}

// NewRegex matches when the target output matches pattern.
// Multi-line mode is enabled so that ^ and $ match at line boundaries of command output.
func NewRegex(pattern string, target Target) (Condition, error) {
	// Validate the pattern as given so that the error message does not show the (?m) prefix.
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, goerr.Wrap(err, "invalid regular expression",
			goerr.T(errs.TagInvalidArgument), goerr.V("pattern", pattern))
	}
	re, err := regexp.Compile("(?m)" + pattern)
	if err != nil {
		return nil, goerr.Wrap(err, "invalid regular expression",
			goerr.T(errs.TagInvalidArgument), goerr.V("pattern", pattern))
	}
	return &regex{re: re, target: target}, nil
}

func (c *regex) Name() string { return "regex" }

func (c *regex) Match(_ context.Context, in *Input) (bool, error) {
	return c.re.Match(c.target.bytes(in)), nil
}
