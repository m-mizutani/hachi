package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/cli"
)

type result struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, ctx context.Context, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(ctx, append([]string{"hachi"}, args...), &stdout, &stderr, "test")
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// fast makes retries quick so that tests finish in milliseconds.
var fast = []string{"--initial-interval", "10ms", "--max-interval", "10ms"}

func withFast(args ...string) []string {
	return append(append([]string{}, fast...), args...)
}

// counterScript prints "ready" from the n-th execution on and "waiting" before that.
func counterScript(t *testing.T, n int) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "count")
	return fmt.Sprintf(`c=$(cat %[1]s 2>/dev/null || echo 0); c=$((c+1)); echo $c > %[1]s; `+
		`if [ $c -ge %[2]d ]; then echo ready; else echo waiting; fi`, file, n)
}

func TestNoConditionSucceedsOnExitZero(t *testing.T) {
	r := run(t, context.Background(), "--", "true")
	gt.V(t, r.code).Equal(cli.ExitSuccess)
}

func TestNoConditionTimesOut(t *testing.T) {
	r := run(t, context.Background(), withFast("--timeout", "200ms", "--", "sh", "-c", "exit 1")...)
	gt.V(t, r.code).Equal(cli.ExitTimeout)
	gt.S(t, r.stderr).Contains("hachi: timeout after 200ms")
}

func TestGrepWaitsUntilOutputMatches(t *testing.T) {
	r := run(t, context.Background(), withFast("--grep", "ready", "--", "sh", "-c", counterScript(t, 3))...)
	gt.V(t, r.code).Equal(cli.ExitSuccess)
	gt.V(t, r.stdout).Equal("ready\n")
	gt.S(t, r.stderr).Contains("attempt=3").Contains("matched=true")
}

func TestConditionIgnoresExitCode(t *testing.T) {
	r := run(t, context.Background(), withFast("--grep", "ready", "--", "sh", "-c", "echo ready; exit 8")...)
	gt.V(t, r.code).Equal(cli.ExitSuccess)
}

func TestExitCodeIsCombinedWithOtherConditions(t *testing.T) {
	r := run(t, context.Background(),
		withFast("--timeout", "200ms", "--grep", "ready", "--exit-code", "0", "--", "sh", "-c", "echo ready; exit 8")...)
	gt.V(t, r.code).Equal(cli.ExitTimeout)
}

func TestFailCondition(t *testing.T) {
	r := run(t, context.Background(), withFast("--grep", "ready", "--fail-grep", "failed", "--", "echo", "failed")...)
	gt.V(t, r.code).Equal(cli.ExitFailMatched)
	gt.V(t, r.stdout).Equal("failed\n")
	gt.S(t, r.stderr).Contains("hachi: fail condition matched: grep (attempts: 1)")
}

func TestFailExitCode(t *testing.T) {
	r := run(t, context.Background(), withFast("--fail-exit-code", "1", "--", "sh", "-c", "exit 1")...)
	gt.V(t, r.code).Equal(cli.ExitFailMatched)
}

func TestJQAndExprFlags(t *testing.T) {
	r := run(t, context.Background(), withFast("--jq", ".ok", "--expr", `json.ok && exit_code == 0`,
		"--", "echo", `{"ok":true}`)...)
	gt.V(t, r.code).Equal(cli.ExitSuccess)
}

func TestTargetStderr(t *testing.T) {
	r := run(t, context.Background(), withFast("--target", "stderr", "--regex", "^ready$",
		"--", "sh", "-c", "echo ready >&2")...)
	gt.V(t, r.code).Equal(cli.ExitSuccess)
	gt.S(t, r.stderr).Contains("ready\n")
}

func TestInvalidArgumentsDoNotRunCommand(t *testing.T) {
	cases := map[string][]string{
		"invalid regex":             {"--regex", "("},
		"invalid jq":                {"--jq", ".["},
		"invalid expr":              {"--expr", "1+"},
		"invalid fail regex":        {"--fail-regex", "("},
		"unknown strategy":          {"--wait-strategy", "linear"},
		"unknown target":            {"--target", "both"},
		"multiplier below one":      {"--multiplier", "0.5"},
		"exit code out of range":    {"--exit-code", "256"},
		"negative exit code":        {"--fail-exit-code", "-1"},
		"zero timeout":              {"--timeout", "0s"},
		"negative attempt":          {"--attempt-timeout", "-1s"},
		"zero fixed interval":       {"-w", "fixed", "--interval", "0s"},
		"unparsable duration":       {"--timeout", "abc"},
		"duplicated grep":           {"--grep", "ready", "--grep", "healthy"},
		"duplicated fail exit code": {"--fail-exit-code", "1", "--fail-exit-code", "2"},
		"unknown flag":              {"--no-such-flag"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			r := run(t, context.Background(), append(args, "--", "touch", marker)...)
			gt.V(t, r.code).Equal(cli.ExitInvalidUsage)
			gt.S(t, r.stderr).Contains("hachi: ").Contains("Run 'hachi --help' for usage.")
			gt.S(t, r.stderr).NotContains("Incorrect Usage")
			// stdout is reserved for the command output, so help must not be printed there.
			gt.V(t, r.stdout).Equal("")
			_, err := os.Stat(marker)
			gt.True(t, os.IsNotExist(err))
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestOutputWriteFailureKeepsExitCode(t *testing.T) {
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), []string{"hachi", "--grep", "ready", "--", "echo", "ready"},
		failingWriter{}, &stderr, "test")
	gt.V(t, code).Equal(cli.ExitSuccess)
	gt.S(t, stderr.String()).Contains("hachi: failed to write command output to stdout: broken pipe")
}

func TestMissingCommand(t *testing.T) {
	r := run(t, context.Background(), "--grep", "x")
	gt.V(t, r.code).Equal(cli.ExitInvalidUsage)
	gt.S(t, r.stderr).Contains("command is required")
}

func TestCommandNotFound(t *testing.T) {
	r := run(t, context.Background(), "--", "hachi-no-such-command")
	gt.V(t, r.code).Equal(cli.ExitCommandStart)
	gt.S(t, r.stderr).Contains("failed to start command")
}

func TestInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	r := run(t, ctx, "--", "sleep", "10")
	gt.V(t, r.code).Equal(cli.ExitInterrupted)
	gt.True(t, time.Since(start) < 5*time.Second)
	gt.S(t, r.stderr).Contains("hachi: interrupted")
}

func TestFlagsAfterCommandBelongToCommand(t *testing.T) {
	withoutSeparator := run(t, context.Background(), "--timeout", "1s", "sh", "-c", `echo "$1"`, "_", "--grep")
	gt.V(t, withoutSeparator.code).Equal(cli.ExitSuccess)
	gt.V(t, withoutSeparator.stdout).Equal("--grep\n")

	withSeparator := run(t, context.Background(), "--timeout", "1s", "--", "sh", "-c", `echo "$1"`, "_", "--grep")
	gt.V(t, withSeparator.code).Equal(cli.ExitSuccess)
	gt.V(t, withSeparator.stdout).Equal("--grep\n")
}

func TestEnvironmentVariables(t *testing.T) {
	t.Run("condition from environment", func(t *testing.T) {
		t.Setenv("HACHI_GREP", "ready")
		t.Setenv("HACHI_TIMEOUT", "200ms")
		t.Setenv("HACHI_INITIAL_INTERVAL", "10ms")
		r := run(t, context.Background(), "--", "echo", "waiting")
		gt.V(t, r.code).Equal(cli.ExitTimeout)
	})

	t.Run("empty value is unset", func(t *testing.T) {
		t.Setenv("HACHI_GREP", "")
		r := run(t, context.Background(), "--", "true")
		gt.V(t, r.code).Equal(cli.ExitSuccess)
	})

	t.Run("flag overrides environment", func(t *testing.T) {
		t.Setenv("HACHI_GREP", "never")
		r := run(t, context.Background(), "--grep", "ready", "--", "echo", "ready")
		gt.V(t, r.code).Equal(cli.ExitSuccess)
	})
}

func TestQuiet(t *testing.T) {
	r := run(t, context.Background(), withFast("-q", "--timeout", "100ms", "--", "sh", "-c", "echo oops >&2; exit 1")...)
	gt.V(t, r.code).Equal(cli.ExitTimeout)
	gt.S(t, r.stderr).NotContains("attempt finished")
	gt.S(t, r.stderr).Contains("oops\n")
	gt.S(t, r.stderr).Contains("hachi: timeout")
}

func TestIgnoredStrategyFlagIsWarned(t *testing.T) {
	r := run(t, context.Background(), "-w", "fixed", "--interval", "10ms", "--initial-interval", "1s", "--", "true")
	gt.V(t, r.code).Equal(cli.ExitSuccess)
	gt.S(t, r.stderr).Contains(`msg="flag is ignored" flag=initial-interval wait_strategy=fixed`)
}

func TestHelpAndVersion(t *testing.T) {
	version := run(t, context.Background(), "--version")
	gt.V(t, version.code).Equal(cli.ExitSuccess)
	gt.S(t, version.stdout).Contains("test")

	help := run(t, context.Background(), "--help")
	gt.V(t, help.code).Equal(cli.ExitSuccess)
	gt.S(t, help.stdout).Contains("CONDITIONS:").Contains("EXIT CODES:").Contains("EXAMPLES:")

	envVars := regexp.MustCompile(`\$HACHI_[A-Z_]+`).FindAllString(help.stdout, -1)
	unique := map[string]bool{}
	for _, v := range envVars {
		unique[v] = true
	}
	gt.V(t, len(unique)).Equal(19)
}

func TestHelpDescriptionExitCodes(t *testing.T) {
	want := fmt.Sprintf("%d success / %d timeout / %d fail condition matched / %d invalid usage /\n  %d command could not be run / %d interrupted",
		cli.ExitSuccess, cli.ExitTimeout, cli.ExitFailMatched, cli.ExitInvalidUsage, cli.ExitCommandStart, cli.ExitInterrupted)
	gt.True(t, strings.Contains(cli.HelpDescription, want))
}
