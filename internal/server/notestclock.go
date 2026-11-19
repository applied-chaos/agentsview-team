//go:build !testclock

package server

import (
	"net/http"

	"github.com/applied-chaos/agentsview-team/internal/clock"
)

func registerTestClock(*http.ServeMux, *clock.Clock) {}
