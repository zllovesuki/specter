package promise

import (
	"context"
	"sync"
)

// All provides similar concurrency to JavaScript's Promise.All, however with 1 distinction. []func that
// are passed in must respect fnCtx cancellation and returns, otherwise All will block even if fnCtx is cancelled.
func All[V comparable](fnCtx context.Context, fns ...func(fnCtx context.Context) (V, error)) ([]V, []error) {
	var (
		wg      = sync.WaitGroup{}
		results = make([]V, len(fns))
		errors  = make([]error, len(fns))
		done    = make(chan struct{}, 1)
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
				errors[i] = e
				return
			}
			results[i] = v
		}(i, fn)
	}
	select {
	case <-done:
	case <-fnCtx.Done():
		// assert that the fn returns first
		<-done
	}
	return results, errors
}
