package condition

import (
	"context"
	"encoding/json"

	"github.com/itchyny/gojq"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/errs"
)

type jq struct {
	query  string
	code   *gojq.Code
	target Target
}

// NewJQ matches when the last value produced by query is neither false nor null,
// which is the same rule as the exit status of `jq -e`.
func NewJQ(query string, target Target) (Condition, error) {
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, goerr.Wrap(err, "invalid jq query",
			goerr.T(errs.TagInvalidArgument), goerr.V("query", query))
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return nil, goerr.Wrap(err, "invalid jq query",
			goerr.T(errs.TagInvalidArgument), goerr.V("query", query))
	}
	return &jq{query: query, code: code, target: target}, nil
}

func (c *jq) Name() string { return "jq" }

func (c *jq) Match(ctx context.Context, in *Input) (bool, error) {
	data := c.target.bytes(in)
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return false, goerr.Wrap(err, "output is not JSON",
			goerr.V("target", c.target), goerr.V("size", len(data)))
	}

	var last any
	produced := false
	iter := c.code.RunWithContext(ctx, v)
	for {
		out, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := out.(error); isErr {
			return false, goerr.Wrap(err, "jq query failed", goerr.V("query", c.query))
		}
		last = out
		produced = true
	}

	if !produced {
		return false, nil
	}
	return last != nil && last != false, nil
}
