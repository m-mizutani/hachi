package runner

import "time"

// SetStopGracePeriod overrides stopGracePeriod and returns a function that restores it.
func SetStopGracePeriod(d time.Duration) func() {
	prev := stopGracePeriod
	stopGracePeriod = d
	return func() { stopGracePeriod = prev }
}
