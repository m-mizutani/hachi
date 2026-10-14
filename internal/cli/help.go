package cli

// helpDescription is shown in the DESCRIPTION section of --help.
// Flag descriptions and environment variable names are listed by urfave/cli
// in the OPTIONS section, so they are not repeated here.
const helpDescription = `Run a command repeatedly until its result satisfies the conditions.
Fails when the timeout is reached first.

CONDITIONS:
  success (all must match): --exit-code --grep --regex --jq --expr
  fail    (any match aborts): --fail-exit-code --fail-grep --fail-regex --fail-jq --fail-expr
  Without success conditions, exit code 0 means success.
  With success conditions, the exit code is ignored unless --exit-code is given.
  --grep, --regex and --jq inspect stdout (change with --target).
  --grep and --fail-grep match a fixed string. Regular expressions are not
  supported, so 'a|b' matches only the text "a|b"; use --regex for patterns.
  --jq matches when its last output is neither false nor null.
  --expr variables: stdout, stderr, exit_code, json (parsed stdout or nil).

EXIT CODES:
  0 success / 1 timeout / 2 fail condition matched / 3 invalid usage /
  4 command could not be run / 130 interrupted

EXAMPLES:
  # Retry until the command succeeds
  hachi -- curl -fsS https://example.com/healthz

  # Wait until the output contains a string
  hachi --grep Running -- kubectl get pod my-pod

  # Check every 30s for up to 2h with a regular expression
  hachi -w fixed --interval 30s -t 2h --regex '^status: (done|ok)$' -- ./job-status.sh 42

  # Wait for GitHub PR checks and stop as soon as one fails
  hachi --jq 'length > 0 and all(.[]; .bucket == "pass" or .bucket == "skipping")' \
        --fail-jq 'any(.[]; .bucket == "fail" or .bucket == "cancel")' \
        -- gh pr checks 123 --json bucket

  # Combine output and exit code in one expression
  hachi --expr 'exit_code == 0 && !(stdout contains "pending")' -- ./check.sh

  # Use a shell for pipes
  hachi --grep ready -- sh -c 'docker logs my-app 2>&1 | tail -n 20'

  # The last output is written to stdout, so it can be piped
  hachi --jq '.status == "completed"' -- gh api repos/OWNER/REPO/actions/runs/1 | jq .conclusion`
