//go:build !testclock

package hosts

import "time"

const sweepEvery = 10 * time.Minute
