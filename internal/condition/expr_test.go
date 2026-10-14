package condition_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/condition"
	"github.com/m-mizutani/hachi/internal/errs"
)

func TestExpr(t *testing.T) {
	c := gt.R1(condition.NewExpr(`exit_code == 8 && stdout contains "pending"`)).NoError(t)
	gt.V(t, c.Name()).Equal("expr")
	gt.True(t, match(t, c, &condition.Input{Stdout: []byte("2 pending"), ExitCode: 8}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte("2 pending"), ExitCode: 0}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte("done"), ExitCode: 8}))
}

func TestExprStderr(t *testing.T) {
	c := gt.R1(condition.NewExpr(`stderr contains "warn"`)).NoError(t)
	gt.True(t, match(t, c, &condition.Input{Stderr: []byte("warn: x")}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte("warn: x")}))
}

func TestExprJSON(t *testing.T) {
	c := gt.R1(condition.NewExpr(`json != nil && json.status == "ok"`)).NoError(t)
	gt.True(t, match(t, c, &condition.Input{Stdout: []byte(`{"status":"ok"}`)}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte(`{"status":"ng"}`)}))

	isNil := gt.R1(condition.NewExpr(`json == nil`)).NoError(t)
	gt.True(t, match(t, isNil, &condition.Input{Stdout: []byte(`not json`)}))
	gt.False(t, match(t, isNil, &condition.Input{Stdout: []byte(`[]`)}))
}

func TestExprEvaluationError(t *testing.T) {
	c := gt.R1(condition.NewExpr(`json.status == "ok"`)).NoError(t)
	matched, err := c.Match(context.Background(), &condition.Input{Stdout: []byte("not json")})
	gt.Error(t, err)
	gt.False(t, matched)
}

func TestExprInvalid(t *testing.T) {
	for name, src := range map[string]string{
		"syntax error":       `1 +`,
		"non-boolean result": `1 + 1`,
		"undefined variable": `foo == 1`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := condition.NewExpr(src)
			gt.Error(t, err)
			gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
		})
	}
}
