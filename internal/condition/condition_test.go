package condition_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/condition"
	"github.com/m-mizutani/hachi/internal/errs"
)

func TestParseTarget(t *testing.T) {
	for _, s := range []string{"stdout", "stderr", "combined"} {
		gt.V(t, gt.R1(condition.ParseTarget(s)).NoError(t)).Equal(condition.Target(s))
	}

	_, err := condition.ParseTarget("both")
	gt.Error(t, err)
	gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
}

// match evaluates c and fails the test if evaluation returns an error.
func match(t *testing.T, c condition.Condition, in *condition.Input) bool {
	t.Helper()
	return gt.R1(c.Match(context.Background(), in)).NoError(t)
}
