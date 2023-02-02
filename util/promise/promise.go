package promise

import (
	"context"
	"sync"
	"sync/atomic"
)

func All[V comparable](fnCtx context.Context, fns ...func(fnCtx context.Context) (V, error)) ([]V, []error) {
	var (
		wg            = sync.WaitGroup{}
		results       = make([]V, len(fns))
		errors        = make([]error, len(fns))
		atomicResults = make([]atomic.Value, len(fns))
		atomicErrors  = make([]atomic.Value, len(fns))
		done          = make(chan struct{}, 1)
		zeroV         V
	)
	wg.Add(len(fns))
	go func() {
		wg.Wait()
		close(done)
	}()
	for i, fn := range fns {
		go func(i int, fn func(context.Context) (V, error)) {
			defer wg.Done()
			v, e := fn(fnCtx)
			if e != nil {
				atomicErrors[i].Store(e)
				return
			}
			if v != zeroV {
				atomicResults[i].Store(v)
			}
		}(i, fn)
	}
	select {
	case <-done:
	case <-fnCtx.Done():
		// TODO: should we wait for fn() to exit with context.DeadlineExceeded?
	}
	for i := range fns {
		if atomicErrors[i].Load() != nil {
			errors[i] = atomicErrors[i].Load().(error)
			continue
		}
		if atomicResults[i].Load() != nil {
			results[i] = atomicResults[i].Load().(V)
		}
	}
	return results, errors
}
