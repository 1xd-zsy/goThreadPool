package pool

import (
	"context"
	"time"
)

type poolWorker struct {
	pool        *Pool
	task        chan func()
	lastUseTime time.Time
}

func (w *poolWorker) execute(ctx context.Context) {
	w.pool.addRunning(1)
	go func() {
		defer func() {
			w.pool.addRunning(-1)
			if p := recover(); p != nil {
				if h := w.pool.options.PanicHandler; h != nil {
					h(p)
				}
			}
			w.pool.cond.Signal()
			close(w.task)
		}()
		for {
			select {
			case f := <-w.task:
				if f == nil {
					return
				}
				f()
				if cloudRecycle := w.pool.putWorker(w); !cloudRecycle {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

}
