# hachi

`hachi` runs a command repeatedly until its result satisfies the conditions you give, and exits with code 0. If the timeout (30 minutes by default) is reached first, it exits with a non-zero code. Between attempts it waits with exponential backoff (1s, 2s, 4s, ... up to 1m by default).

Typical uses are waiting in shell scripts and CI jobs: for CI checks to finish, for a deployment to become healthy, or for an asynchronous job to complete.

```sh
hachi --grep Running -- kubectl get pod my-pod
```

## Install

```sh
go install github.com/m-mizutani/hachi@latest
```

Prebuilt binaries for Linux and macOS (amd64 and arm64) are attached to [GitHub Releases](https://github.com/m-mizutani/hachi/releases).

hachi supports Linux and macOS.

## Usage

```
hachi [options] [--] <command> [args...]
```

The command is executed directly, not through a shell. Options must come before the command; everything from the command name on is passed to the command, so `--` is optional (`hachi gh pr checks 123 --json bucket` passes `--json` to `gh`).

Without any condition, hachi waits until the command exits with code 0:

```sh
hachi -- curl -fsS https://example.com/healthz
```

Run `hachi --help` to see the options, the condition rules and examples.

## Wait strategies

| Strategy | Wait after the n-th attempt | Options |
| --- | --- | --- |
| `backoff` (default) | `min(initial-interval × multiplier^(n-1), max-interval)` | `--initial-interval` (1s), `--multiplier` (2), `--max-interval` (1m) |
| `fixed` | `interval` | `--interval` (10s) |

With the defaults, `backoff` waits 1s, 2s, 4s, 8s, 16s, 32s, 1m, 1m, ... and runs about 35 attempts in 30 minutes. The first attempt runs immediately. If the next wait would go past the timeout, hachi waits only until the timeout and then exits with code 1.

Options of the strategy that is not selected are ignored with a warning.

## Conditions

Each attempt is checked against two kinds of conditions.

| Kind | Options | Rule |
| --- | --- | --- |
| Success | `--exit-code` `--grep` `--regex` `--jq` `--expr` | hachi exits with 0 when **all** of them match |
| Fail | `--fail-exit-code` `--fail-grep` `--fail-regex` `--fail-jq` `--fail-expr` | hachi exits with 2 as soon as **any** of them matches |

- Fail conditions are checked first. If a result matches both, hachi treats it as a failure.
- Without success conditions, exit code 0 is the success condition.
- With success conditions, the exit code is ignored unless `--exit-code` is given. This lets you inspect the output of commands that exit with non-zero codes while waiting (for example, `gh pr checks` exits with 8 while checks are pending).
- Each condition option can be given only once; giving it twice is an error. To combine several patterns, use one `--regex`, `--jq` or `--expr`. An empty value (for example `HACHI_GREP=`) is treated as not given.
- If a condition cannot be evaluated for an attempt (for example, `--jq` on output that is not JSON), it is treated as not matching, a warning is logged, and hachi keeps waiting. A syntax error in a regular expression, jq query or expression is reported before the command runs.

| Option | Matches when |
| --- | --- |
| `--exit-code N` | the command exits with code `N` (0-255) |
| `--grep STRING` | the output contains `STRING` |
| `--regex PATTERN` | the output matches the [RE2](https://github.com/google/re2/wiki/Syntax) pattern; `^` and `$` match at the start and end of each line |
| `--jq QUERY` | the output is JSON and the last value produced by the [jq](https://jqlang.org/manual/) query is neither `false` nor `null` (the same rule as `jq -e`); a query that produces no value does not match |
| `--expr EXPR` | the [expr-lang](https://expr-lang.org/docs/language-definition) expression is true |

`--grep`, `--regex` and `--jq` inspect stdout. Use `--target stderr` to inspect stderr, or `--target combined` to inspect stdout followed by stderr.

`--expr` can use these variables:

| Variable | Type | Value |
| --- | --- | --- |
| `stdout` | string | standard output |
| `stderr` | string | standard error |
| `exit_code` | int | exit code (-1 if the command was killed by a signal) |
| `json` | any | standard output parsed as JSON, or `nil` if it is not JSON |

```sh
hachi --expr 'exit_code == 0 && !(stdout contains "pending")' -- ./check.sh
hachi --expr 'json != nil && json.replicas == json.readyReplicas' -- ./status.sh
```

## Options

Every option can also be set with an environment variable. A command line option takes precedence over its environment variable.

| Option | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `--timeout`, `-t` | `HACHI_TIMEOUT` | `30m` | Give up and exit with code 1 after this duration |
| `--wait-strategy`, `-w` | `HACHI_WAIT_STRATEGY` | `backoff` | `backoff` or `fixed` |
| `--initial-interval` | `HACHI_INITIAL_INTERVAL` | `1s` | backoff: wait after the first attempt |
| `--max-interval` | `HACHI_MAX_INTERVAL` | `1m` | backoff: upper limit of the wait |
| `--multiplier` | `HACHI_MULTIPLIER` | `2` | backoff: factor applied to the wait after each attempt (1.0 or greater) |
| `--interval` | `HACHI_INTERVAL` | `10s` | fixed: wait between attempts |
| `--attempt-timeout` | `HACHI_ATTEMPT_TIMEOUT` | `0` | Stop a single attempt after this duration and retry; `0` means no limit other than `--timeout` |
| `--target` | `HACHI_TARGET` | `stdout` | Output inspected by `--grep`, `--regex`, `--jq` and their fail variants: `stdout`, `stderr` or `combined` |
| `--exit-code` | `HACHI_EXIT_CODE` | | Success condition: exit code |
| `--grep` | `HACHI_GREP` | | Success condition: substring |
| `--regex` | `HACHI_REGEX` | | Success condition: regular expression |
| `--jq` | `HACHI_JQ` | | Success condition: jq query |
| `--expr` | `HACHI_EXPR` | | Success condition: expr-lang expression |
| `--fail-exit-code` | `HACHI_FAIL_EXIT_CODE` | | Fail condition: exit code |
| `--fail-grep` | `HACHI_FAIL_GREP` | | Fail condition: substring |
| `--fail-regex` | `HACHI_FAIL_REGEX` | | Fail condition: regular expression |
| `--fail-jq` | `HACHI_FAIL_JQ` | | Fail condition: jq query |
| `--fail-expr` | `HACHI_FAIL_EXPR` | | Fail condition: expr-lang expression |
| `--quiet`, `-q` | `HACHI_QUIET` | `false` | Do not log each attempt |

Durations use Go syntax such as `500ms`, `30s`, `5m` and `1h30m`.

## Output and exit codes

hachi writes its own logs only to stderr: one line per attempt, plus warnings. When it exits, it writes the stdout and stderr of the last completed attempt to its own stdout and stderr, so the result can be piped to the next command. An attempt stopped by a timeout or an interruption is not completed and its output is discarded.

| Exit code | Meaning |
| --- | --- |
| 0 | All success conditions matched |
| 1 | The timeout was reached |
| 2 | A fail condition matched |
| 3 | Invalid options or arguments; the command was not run |
| 4 | The command could not be run (for example, it was not found) |
| 130 | Interrupted by SIGINT or SIGTERM |

When the timeout is reached or hachi is interrupted while the command is running, the command and the processes it started receive SIGTERM, and SIGKILL 5 seconds later if any of them is still running. The command runs in its own process group for this purpose.

If hachi cannot write the final output, it reports the error on stderr and keeps the exit code of the wait.

## Examples

### Wait for the CI checks of a GitHub pull request

```sh
hachi \
  --jq      'length > 0 and all(.[]; .bucket == "pass" or .bucket == "skipping")' \
  --fail-jq 'any(.[]; .bucket == "fail" or .bucket == "cancel")' \
  -- gh pr checks 123 --json bucket
```

This exits with 0 when every check has passed or was skipped, and with 2 as soon as a check fails or is cancelled.

With `--json`, `gh pr checks` exits with 0 whenever it can fetch the checks, and reports the state of each check in the `bucket` field (`pass`, `fail`, `pending`, `skipping` or `cancel`). When it cannot fetch them, for example right after a push when no check has been registered yet or on a network error, it prints nothing to stdout, so the jq conditions do not match and hachi keeps waiting.

Judging by exit code alone does not work as well. Without `--json`, `gh pr checks` exits with 0 when the checks pass, 8 while they are pending, and 1 both when a check fails and when the checks cannot be fetched. `hachi -- gh pr checks 123` therefore keeps waiting until the timeout after a check fails, and adding `--fail-exit-code 1` makes hachi stop right after a push, before any check has been registered.

A check appears only after its workflow starts. If a workflow starts later than the others (for example, one triggered by the completion of another workflow), hachi may report success before that check appears. The `--required` option of `gh pr checks` does not prevent this: it only limits the reported checks to required ones and does not wait for checks that have not appeared yet. To avoid reporting success too early, require the expected check names in the jq query:

```sh
hachi --jq 'any(.[]; .name == "deploy" and .bucket == "pass")' \
      --fail-jq 'any(.[]; .bucket == "fail" or .bucket == "cancel")' \
      -- gh pr checks 123 --json name,bucket
```

### Wait for the workflow runs of a commit

```sh
hachi \
  --jq      'length > 0 and all(.[]; .status == "completed")' \
  --fail-jq 'any(.[]; .status == "completed" and .conclusion != "success" and .conclusion != "skipped")' \
  -- gh run list --commit "$(git rev-parse HEAD)" --json status,conclusion
```

### Wait until a service becomes healthy

```sh
hachi -t 5m -- curl -fsS https://example.com/healthz
```

### Wait for a job with a fixed interval and a longer timeout

```sh
hachi -w fixed --interval 30s -t 2h --regex '^status: (done|ok)$' --fail-regex '^status: error$' -- ./job-status.sh 42
```

### Use a shell for pipes and redirects

hachi does not use a shell, so wrap pipelines in `sh -c`:

```sh
hachi --grep ready -- sh -c 'docker logs my-app 2>&1 | tail -n 20'
```

### Pass the final output to the next command

```sh
hachi --jq '.status == "completed"' -- gh api repos/OWNER/REPO/actions/runs/1 | jq -r .conclusion
```

### Stop a hanging attempt

```sh
hachi --attempt-timeout 10s -- ./flaky-check.sh
```

### Change defaults with environment variables

```sh
export HACHI_TIMEOUT=10m
export HACHI_MAX_INTERVAL=15s
hachi --grep Running -- kubectl get pod my-pod
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
