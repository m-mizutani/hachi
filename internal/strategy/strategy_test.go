package strategy_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hachi/internal/errs"
	"github.com/m-mizutani/hachi/internal/strategy"
)

func TestBackoffDefaults(t *testing.T) {
	b := gt.R1(strategy.NewBackoff(time.Second, time.Minute, 2)).NoError(t)
	want := []time.Duration{
		1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 32 * time.Second, time.Minute, time.Minute,
	}
	for i, w := range want {
		gt.V(t, b.Next(i+1)).Equal(w)
	}
}

func TestBackoffFractionalMultiplier(t *testing.T) {
	b := gt.R1(strategy.NewBackoff(100*time.Millisecond, time.Second, 1.5)).NoError(t)
	gt.V(t, b.Next(1)).Equal(100 * time.Millisecond)
	gt.V(t, b.Next(2)).Equal(150 * time.Millisecond)
	gt.V(t, b.Next(3)).Equal(225 * time.Millisecond)
	gt.V(t, b.Next(10)).Equal(time.Second)
}

func TestBackoffOverflow(t *testing.T) {
	b := gt.R1(strategy.NewBackoff(time.Second, time.Minute, 2)).NoError(t)
	gt.V(t, b.Next(1000)).Equal(time.Minute)
	gt.V(t, b.Next(1<<30)).Equal(time.Minute)
}

func TestBackoffMultiplierOne(t *testing.T) {
	b := gt.R1(strategy.NewBackoff(time.Second, time.Minute, 1)).NoError(t)
	for i := 1; i <= 5; i++ {
		gt.V(t, b.Next(i)).Equal(time.Second)
	}
}

func TestBackoffAttemptBelowOne(t *testing.T) {
	b := gt.R1(strategy.NewBackoff(time.Second, time.Minute, 2)).NoError(t)
	gt.V(t, b.Next(0)).Equal(time.Second)
}

func TestFixed(t *testing.T) {
	f := gt.R1(strategy.NewFixed(3 * time.Second)).NoError(t)
	for i := 1; i <= 5; i++ {
		gt.V(t, f.Next(i)).Equal(3 * time.Second)
	}
}

func TestInvalidArguments(t *testing.T) {
	cases := map[string]error{
		"zero initial":            second(strategy.NewBackoff(0, time.Minute, 2)),
		"negative initial":        second(strategy.NewBackoff(-time.Second, time.Minute, 2)),
		"max below initial":       second(strategy.NewBackoff(time.Minute, time.Second, 2)),
		"multiplier below one":    second(strategy.NewBackoff(time.Second, time.Minute, 0.5)),
		"zero fixed interval":     second(strategy.NewFixed(0)),
		"negative fixed interval": second(strategy.NewFixed(-time.Second)),
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, err)
			gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
		})
	}
}

func TestParseName(t *testing.T) {
	gt.V(t, gt.R1(strategy.ParseName("backoff")).NoError(t)).Equal(strategy.NameBackoff)
	gt.V(t, gt.R1(strategy.ParseName("fixed")).NoError(t)).Equal(strategy.NameFixed)

	_, err := strategy.ParseName("linear")
	gt.Error(t, err)
	gt.True(t, goerr.HasTag(err, errs.TagInvalidArgument))
}

func second[T any](_ T, err error) error { return err }
