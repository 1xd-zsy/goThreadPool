package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"sync/atomic"
)

const closeState int32 = int32(1)

type Pool struct {
	running int32
	lock    sync.Locker
	workers *workArray
	state   int32
	cond    *sync.Cond
	waiting int32

	stopHeartbeat context.CancelFunc
	ctx           context.Context
	options       *Options
}

func BuildPool(options ...Option) (*Pool, error) {
	opts := loadOptions(options...)
	if opts.Size <= 0 {
		return nil, fmt.Errorf("size less than 0")
	}

	pool := &Pool{
		lock:    &sync.Mutex{},
		options: opts,
	}

	pool.workers = newWorkArray(int(opts.Size))
	pool.cond = sync.NewCond(pool.lock)

	//var ctx context.Context
	pool.ctx, pool.stopHeartbeat = context.WithCancel(context.Background())
	if pool.options.ExpireWorkerCleanInterval != 0 {
		go pool.delExpiredWorker(pool.ctx)
	}
	return pool, nil
}

func (p *Pool) Exit() {
	if !atomic.CompareAndSwapInt32(&p.state, 0, closeState) {
		return
	}
	fmt.Println("exiting")
	p.lock.Lock()
	for {
		if p.Waiting() == 0 {
			break
		}
		p.cond.Signal()
		p.cond.Wait()
	}
	p.stopHeartbeat()
	p.workers.exit()
	p.lock.Unlock()
	fmt.Println("exited")
}

func (p *Pool) isExit() bool {
	return atomic.LoadInt32(&p.state) == closeState
}

func (p *Pool) Submit(task func()) error {
	var w *poolWorker
	if p.isExit() {
		return errors.New("pool exiting")
	}
	if w = p.getWorker(); w == nil {
		return errors.New("pool full")
	}
	w.task <- task
	return nil
}

func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.options.Size))
}

func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *Pool) Waiting() int {
	return int(atomic.LoadInt32(&p.waiting))
}

func (p *Pool) delExpiredWorker(ctx context.Context) {
	hb := time.NewTicker(p.options.ExpireWorkerCleanInterval)

	defer func() {
		hb.Stop()
	}()

	for {
		select {
		case <-hb.C:
		case <-ctx.Done():
			return
		}
		p.lock.Lock()
		expairedWorkers := p.workers.getExpiredWorker(p.options.ExpireWorkerCleanInterval)
		p.lock.Unlock()

		for i := range expairedWorkers {
			expairedWorkers[i].task <- nil
			expairedWorkers[i] = nil
		}
		// 有可能所有都过期了
		if p.Running() > 0 || p.Waiting() > 0 {
			p.cond.Broadcast()
		}
	}
}

func (p *Pool) putWorker(worker *poolWorker) bool {
	if c := p.Cap(); p.Running() > c {
		return false
	}
	worker.lastUseTime = time.Now()
	p.lock.Lock()
	p.workers.putWorker(worker)
	// 这里只需要通知一个
	p.cond.Signal()
	p.lock.Unlock()
	return true
}

func (p *Pool) getWorker() (w *poolWorker) {
	newWorkerAndRun := func() {
		w = &poolWorker{
			pool: p,
			task: make(chan func()),
		}
		w.execute(p.ctx)
	}
	p.lock.Lock()
	// 有情况不能defer
	// defer p.lock.Unlock()
	w = p.workers.getWorker()
	if w != nil {
		p.lock.Unlock()
		return
	} else if c := p.Cap(); c > p.Running() {
		p.lock.Unlock()
		newWorkerAndRun()
	} else {
		// 这里需要无限循环
		for {
			if p.Waiting() >= int(p.options.MaxWaitTaskNum) {
				p.lock.Unlock()
				return
			}
			p.addWaiting(1)
			p.cond.Wait()
			p.addWaiting(-1)

			var newWorkerNum int
			if newWorkerNum = p.Running(); newWorkerNum == 0 {
				p.lock.Unlock()
				newWorkerAndRun()
				return
			}
			if w = p.workers.getWorker(); w == nil {
				if newWorkerNum < p.Cap() {
					p.lock.Unlock()
					newWorkerAndRun()
					return
				} else {
					continue
				}
			} else {
				break
			}
		}
		p.lock.Unlock()
	}
	return
}

func (p *Pool) addRunning(t int) {
	atomic.AddInt32(&p.running, int32(t))
}

func (p *Pool) addWaiting(t int) {
	atomic.AddInt32(&p.waiting, int32(t))
}
