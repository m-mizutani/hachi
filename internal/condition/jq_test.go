package condition_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/condition"
	"github.com/m-mizutani/hachi/internal/errs"
)

func TestJQ(t *testing.T) {
	c := gt.R1(condition.NewJQ(`.status == "ok"`, condition.TargetStdout)).NoError(t)
	gt.V(t, c.Name()).Equal("jq")
	gt.True(t, match(t, c, &condition.Input{Stdout: []byte(`{"status":"ok"}`)}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte(`{"status":"ng"}`)}))
}

func TestJQTruthiness(t *testing.T) {
	cases := []struct {
		name  string
		query string
		input string
		want  bool
	}{
		{"no output", `empty`, `{}`, false},
		{"last value null", `.missing`, `{}`, false},
		{"last value false", `false`, `{}`, false},
		{"non-boolean value", `.name`, `{"name":"x"}`, true},
		{"last of many values decides", `.[] | .ok`, `[{"ok":false},{"ok":true}]`, true},
		{"last of many values is false", `.[] | .ok`, `[{"ok":true},{"ok":false}]`, false},
		{"github checks all passed", `length > 0 and all(.[]; .bucket == "pass" or .bucket == "skipping")`,
			`[{"bucket":"pass"},{"bucket":"skipping"}]`, true},
		{"github checks pending", `length > 0 and all(.[]; .bucket == "pass" or .bucket == "skipping")`,
			`[{"bucket":"pass"},{"bucket":"pending"}]`, false},
		{"github checks empty", `length > 0 and all(.[]; .bucket == "pass" or .bucket == "skipping")`,
			`[]`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := gt.R1(condition.NewJQ(tc.query, condition.TargetStdout)).NoError(t)
			gt.V(t, match(t, c, &condition.Input{Stdout: []byte(tc.input)})).Equal(tc.want)
		})
	}
}

func TestJQTarget(t *testing.T) {
	c := gt.R1(condition.NewJQ(`.ok`, condition.TargetStderr)).NoError(t)
	gt.True(t, match(t, c, &condition.Input{Stdout: []byte(`not json`), Stderr: []byte(`{"ok":true}`)}))
}

func TestJQEvaluationErrors(t *testing.T) {
	cases := map[string]struct {
		query string
		input string
	}{
		"not JSON":      {`.status`, `not json`},
		"empty output":  {`.status`, ``},
		"runtime error": {`error("x")`, `{}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := gt.R1(condition.NewJQ(tc.query, condition.TargetStdout)).NoError(t)
			matched, err := c.Match(context.Background(), &condition.Input{Stdout: []byte(tc.input)})
			gt.Error(t, err)
			gt.False(t, matched)
		})
	}
}

func TestJQInvalid(t *testing.T) {
	_, err := condition.NewJQ(`.[`, condition.TargetStdout)
	gt.Error(t, err)
	gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
}
