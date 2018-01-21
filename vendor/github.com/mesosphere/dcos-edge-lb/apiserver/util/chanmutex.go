package util

// ChanMutex is a mutex that can return a channel to wait on
type ChanMutex struct {
	c chan struct{}
}

// NewChanMutex creates a new ChanMutex struct
func NewChanMutex() ChanMutex {
	tm := ChanMutex{
		c: make(chan struct{}, 1),
	}

	// Seed the semaphore
	tm.c <- struct{}{}

	return tm
}

// LockChan returns a channel that returns when a lock is acquired
func (tm ChanMutex) LockChan() <-chan struct{} {
	return tm.c
}

// Lock returns when a lock is acquired
func (tm ChanMutex) Lock() {
	<-tm.LockChan()
}

// Unlock the ChanMutex
func (tm ChanMutex) Unlock() {
	select {
	case tm.c <- struct{}{}:
		// noop
	default:
		panic("attempt to unlock an unlocked chanMutex")
	}
}
