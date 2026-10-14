package condition_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/condition"
)

func TestExitCode(t *testing.T) {
	c := condition.NewExitCode(0)
	gt.V(t, c.Name()).Equal("exit-code")
	gt.True(t, match(t, c, &condition.Input{ExitCode: 0}))
	gt.False(t, match(t, c, &condition.Input{ExitCode: 1}))
}
