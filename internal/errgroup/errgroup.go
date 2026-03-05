package errgroup

import (
	"context"
	"sync"
)

type ErrGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	errCh  chan error
	wg     sync.WaitGroup
}

func NewErrGroup(ctx context.Context) (*ErrGroup, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	return &ErrGroup{
		ctx:    ctx,
		cancel: cancel,
		errCh:  make(chan error, 1),
	}, ctx
}

func (g *ErrGroup) Go(f func(ctx context.Context) error) {
	g.wg.Add(1)

	go func() {
		defer g.wg.Done()

		if err := f(g.ctx); err != nil {
			g.cancel()
			select {
			case g.errCh <- err:
			default:
			}
		}
	}()
}

func (g *ErrGroup) Cancel() {
	g.cancel()
}

func (g *ErrGroup) Wait() error {
	go func() {
		g.wg.Wait()
		close(g.errCh)
	}()

	return <-g.errCh
}
