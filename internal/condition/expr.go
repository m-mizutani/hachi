package condition

import (
	"context"
	"encoding/json"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/errs"
)

// exprEnv defines the variables available in an --expr expression.
type exprEnv struct {
	Stdout   string `expr:"stdout"`
	Stderr   string `expr:"stderr"`
	ExitCode int    `expr:"exit_code"`
	JSON     any    `expr:"json"` // stdout parsed as JSON, or nil if stdout is not JSON
}

type exprCond struct {
	source  string
	program *vm.Program
}

// NewExpr matches when the expr-lang expression evaluates to true.
func NewExpr(source string) (Condition, error) {
	program, err := expr.Compile(source, expr.Env(exprEnv{}), expr.AsBool())
	if err != nil {
		return nil, goerr.Wrap(err, "invalid expression",
			goerr.T(errs.TagInvalidArgument), goerr.V("expr", source))
	}
	return &exprCond{source: source, program: program}, nil
}

func (c *exprCond) Name() string { return "expr" }

func (c *exprCond) Match(_ context.Context, in *Input) (bool, error) {
	env := exprEnv{
		Stdout:   string(in.Stdout),
		Stderr:   string(in.Stderr),
		ExitCode: in.ExitCode,
	}
	var v any
	if err := json.Unmarshal(in.Stdout, &v); err == nil {
		env.JSON = v
	}

	out, err := expr.Run(c.program, env)
	if err != nil {
		return false, goerr.Wrap(err, "expression evaluation failed", goerr.V("expr", c.source))
	}
	matched, ok := out.(bool)
	if !ok {
		return false, goerr.New("expression did not return a boolean",
			goerr.V("expr", c.source), goerr.V("result", out))
	}
	return matched, nil
}
