package condition_test

import (
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/condition"
	"github.com/m-mizutani/hachi/internal/errs"
)

func TestGrep(t *testing.T) {
	c := condition.NewGrep("ready", condition.TargetStdout)
	gt.V(t, c.Name()).Equal("grep")
	gt.True(t, match(t, c, &condition.Input{Stdout: []byte("status: ready\n")}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte("status: pending\n")}))
}

func TestGrepTarget(t *testing.T) {
	onlyStdout := &condition.Input{Stdout: []byte("ready"), Stderr: []byte("waiting")}
	onlyStderr := &condition.Input{Stdout: []byte("waiting"), Stderr: []byte("ready")}

	gt.False(t, match(t, condition.NewGrep("ready", condition.TargetStderr), onlyStdout))
	gt.True(t, match(t, condition.NewGrep("ready", condition.TargetStderr), onlyStderr))
	gt.True(t, match(t, condition.NewGrep("ready", condition.TargetCombined), onlyStderr))
	gt.True(t, match(t, condition.NewGrep("ready", condition.TargetCombined), onlyStdout))
	gt.False(t, match(t, condition.NewGrep("ready", condition.TargetStdout), onlyStderr))
}

func TestRegex(t *testing.T) {
	c := gt.R1(condition.NewRegex(`^state=(done|ok)$`, condition.TargetStdout)).NoError(t)
	gt.V(t, c.Name()).Equal("regex")
	gt.True(t, match(t, c, &condition.Input{Stdout: []byte("id=1\nstate=done\n")}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte("id=1\nstate=running\n")}))
	gt.False(t, match(t, c, &condition.Input{Stdout: []byte("prev_state=done\n")}))
}

func TestRegexInvalid(t *testing.T) {
	_, err := condition.NewRegex("(", condition.TargetStdout)
	gt.Error(t, err)
	gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
}
