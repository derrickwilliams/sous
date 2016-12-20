package sous

import (
	"fmt"
	"sync"
)

// ResolveStatus represents the status of a resolve run.
type ResolveStatus struct {
	// Errors is a channel of resolve errors.
	Errors chan error
	// phase is used to tell the user which phase the resolution is in.
	phase string
	Log   chan string
	// finished may be closed with no error, or closed after a single
	// error is emitted to the channel.
	finished chan error
	// err is the final error returned from a phase that ends the resolution.
	err error
	sync.RWMutex
}

// NewResolveStatus creates a new ResolveStatus and calls f with it as its
// argument. It then returns that ResolveStatus immediately.
func NewResolveStatus(f func(*ResolveStatus)) *ResolveStatus {
	rs := &ResolveStatus{
		Log:      make(chan string, 10e6),
		Errors:   make(chan error, 10e6),
		finished: make(chan error),
	}
	go func() {
		f(rs)
		close(rs.Errors)
		close(rs.Log)
		rs.write(func() {
			select {
			default:
				close(rs.finished)
			case err, open := <-rs.finished:
				if open {
					close(rs.finished)
				}
				rs.err = err
			}
		})
	}()
	return rs
}

func (rs *ResolveStatus) log(format string, a ...interface{}) {
	rs.Log <- fmt.Sprintf(format, a...)
}

// Done returns true if the resolution has finished. Otherwise it returns false.
func (rs *ResolveStatus) Done() bool {
	select {
	case err := <-rs.finished:
		if err != nil {
			rs.write(func() {
				rs.err = err
			})
		}
		return true
	default:
		return false
	}
}

// Wait blocks until the resolution is finished.
func (rs *ResolveStatus) Wait() error {
	return <-rs.finished
}

// performPhase performs the requested phase, only if nothing has cancelled the
// resolve.
func (rs *ResolveStatus) performPhase(name string, f func() error) {
	if rs.Done() {
		return
	}
	rs.setPhase(name)
	if err := f(); err != nil {
		rs.doneWithError(err)
	}
}

func (rs *ResolveStatus) performGuaranteedPhase(name string, f func()) {
	rs.performPhase(name, func() error { f(); return nil })
}

// setPhase sets the phase of this resolve status.
func (rs *ResolveStatus) setPhase(phase string) {
	rs.log("Entering %s phase.", phase)
	rs.write(func() {
		rs.phase = phase
	})
}

// write encapsulates locking this ResolveStatus for writing using f.
func (rs *ResolveStatus) write(f func()) {
	rs.Lock()
	defer rs.Unlock()
	f()
}

// read encapsulates locking this ResolveStatus for reading using f.
func (rs *ResolveStatus) read(f func()) {
	rs.RLock()
	defer rs.RUnlock()
	f()
}

// doneWithError marks the resolution as finished with an error.
func (rs *ResolveStatus) doneWithError(err error) {
	rs.write(func() {
		rs.finished <- err
		close(rs.finished)
	})
}

// done marks the resolution as done.
func (rs *ResolveStatus) done() {
	rs.write(func() {
		close(rs.finished)
	})
}
