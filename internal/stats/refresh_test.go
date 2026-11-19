package stats

import (
	"context"
	"testing"
	"time"
)

func TestLoopRetriesSoonAfterAFailedRefresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	calls := 0
	done := make(chan struct{})
	go func() {
		loop(ctx, func(context.Context) bool {
			calls++
			if calls == 2 {
				close(done)
			}
			return calls > 1
		}, time.Hour, 10*time.Millisecond)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("a failed refresh was not retried before the full interval")
	}
}
