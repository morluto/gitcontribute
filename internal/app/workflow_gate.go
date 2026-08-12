package app

import (
	"context"
	"sync"
)

// workflowGate serializes workflows that share one non-concurrent writer
// boundary. The service owns its lifetime; callers own cancellation while
// waiting and must release a successful acquisition.
type workflowGate struct {
	once sync.Once
	slot chan struct{}
}

func (g *workflowGate) acquire(ctx context.Context) (func(), error) {
	g.once.Do(func() {
		g.slot = make(chan struct{}, 1)
		g.slot <- struct{}{}
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.slot:
		var releaseOnce sync.Once
		return func() { releaseOnce.Do(func() { g.slot <- struct{}{} }) }, nil
	}
}
