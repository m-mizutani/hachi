// Package strategy computes how long to wait between attempts.
package strategy

import (
	"math"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/hachi/internal/errs"
)

// Strategy decides the wait duration between attempts.
type Strategy interface {
	// Next returns how long to wait after the attempt-th attempt (1-origin).
	Next(attempt int) time.Duration
}

// Name identifies a wait strategy on the command line.
type Name string

const (
	NameBackoff Name = "backoff"
	NameFixed   Name = "fixed"
)

// ParseName validates a wait strategy name.
func ParseName(s string) (Name, error) {
	switch Name(s) {
	case NameBackoff, NameFixed:
		return Name(s), nil
	default:
		return "", goerr.New("unknown wait strategy",
			goerr.T(errs.TagInvalidArgument),
			goerr.V("wait_strategy", s),
			goerr.V("allowed", []Name{NameBackoff, NameFixed}))
	}
}

// Backoff grows the wait duration exponentially up to a maximum.
type Backoff struct {
	initial    time.Duration
	max        time.Duration
	multiplier float64
}

// NewBackoff returns a Backoff that waits initial*multiplier^(attempt-1), capped at max.
func NewBackoff(initial, max time.Duration, multiplier float64) (*Backoff, error) {
	if initial <= 0 {
		return nil, goerr.New("initial interval must be positive",
			goerr.T(errs.TagInvalidArgument), goerr.V("initial_interval", initial))
	}
	if max < initial {
		return nil, goerr.New("max interval must be greater than or equal to initial interval",
			goerr.T(errs.TagInvalidArgument), goerr.V("initial_interval", initial), goerr.V("max_interval", max))
	}
	if math.IsNaN(multiplier) || multiplier < 1.0 {
		return nil, goerr.New("multiplier must be 1.0 or greater",
			goerr.T(errs.TagInvalidArgument), goerr.V("multiplier", multiplier))
	}
	return &Backoff{initial: initial, max: max, multiplier: multiplier}, nil
}

// Next implements Strategy.
func (b *Backoff) Next(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	// Computed in float64 because the power overflows int64 nanoseconds quickly.
	d := float64(b.initial) * math.Pow(b.multiplier, float64(attempt-1))
	if math.IsInf(d, 0) || d >= float64(b.max) {
		return b.max
	}
	return time.Duration(d)
}

// Fixed waits the same duration after every attempt.
type Fixed struct {
	interval time.Duration
}

// NewFixed returns a Fixed strategy.
func NewFixed(interval time.Duration) (*Fixed, error) {
	if interval <= 0 {
		return nil, goerr.New("interval must be positive",
			goerr.T(errs.TagInvalidArgument), goerr.V("interval", interval))
	}
	return &Fixed{interval: interval}, nil
}

// Next implements Strategy.
func (f *Fixed) Next(int) time.Duration {
	return f.interval
}
