// Package errs defines goerr tags that classify why hachi stopped.
// The CLI layer maps each tag to a process exit code.
package errs

import "github.com/m-mizutani/goerr/v2"

var (
	TagInvalidArgument      = goerr.NewTag("invalid_argument")
	TagCommandStart         = goerr.NewTag("command_start")
	TagTimeout              = goerr.NewTag("timeout")
	TagFailConditionMatched = goerr.NewTag("fail_condition_matched")
	TagInterrupted          = goerr.NewTag("interrupted")
)
