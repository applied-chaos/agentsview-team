//go:build testclock

package hosts

import "time"

// The e2e suite moves the clock and waits for the sweep.
const sweepEvery = 500 * time.Millisecond
