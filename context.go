package lazyapp

import (
	"time"
)

func (a *lazyApp) Deadline() (deadline time.Time, ok bool) {
	return a.ctx.Deadline()
}

func (a *lazyApp) Done() <-chan struct{} {
	return a.ctx.Done()
}

func (a *lazyApp) Err() error {
	return a.ctx.Err()
}

func (a *lazyApp) Value(key any) any {
	return a.ctx.Value(key)
}
