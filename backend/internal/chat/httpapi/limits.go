package httpapi

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RetryAfterSeconds is what a refused caller is told to wait: roughly one turn.
const RetryAfterSeconds = 5

// ErrTurnsBusy is every turn slot taken.
type ErrTurnsBusy struct {
	Limit int
}

func (e ErrTurnsBusy) Error() string {
	return fmt.Sprintf("all %d turn slots are busy; retry in %ds", e.Limit, RetryAfterSeconds)
}

// TurnLimiter caps how many turns run at once, across every conversation.
type TurnLimiter struct {
	slots chan struct{}
	limit int
	wait  time.Duration
}

func NewTurnLimiter(limit int, wait time.Duration) *TurnLimiter {
	return &TurnLimiter{slots: make(chan struct{}, limit), limit: limit, wait: wait}
}

// Acquire takes a slot, waiting at most l.wait. A free slot is taken even when the wait is tiny.
func (l *TurnLimiter) Acquire(ctx context.Context) (release func(), err error) {
	select {
	case l.slots <- struct{}{}:
		return l.releaser(), nil
	default:
	}
	timer := time.NewTimer(l.wait)
	defer timer.Stop()
	select {
	case l.slots <- struct{}{}:
		return l.releaser(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrTurnsBusy{Limit: l.limit}
	}
}

func (l *TurnLimiter) releaser() func() {
	var once sync.Once
	return func() { once.Do(func() { <-l.slots }) }
}

// ConversationLocks serialises turns within one conversation.
type ConversationLocks struct {
	mu    sync.Mutex
	locks map[string]*convLock
}

type convLock struct {
	ch      chan struct{} // 1-buffered, so a waiter can give up on its context
	holders int           // holders and waiters; the entry goes at zero
}

func NewConversationLocks() *ConversationLocks {
	return &ConversationLocks{locks: map[string]*convLock{}}
}

func (c *ConversationLocks) Lock(ctx context.Context, id string) (unlock func(), err error) {
	c.mu.Lock()
	l := c.locks[id]
	if l == nil {
		l = &convLock{ch: make(chan struct{}, 1)}
		c.locks[id] = l
	}
	l.holders++
	c.mu.Unlock()

	select {
	case l.ch <- struct{}{}:
	case <-ctx.Done():
		c.drop(id, l)
		return nil, ctx.Err()
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			<-l.ch
			c.drop(id, l)
		})
	}, nil
}

func (c *ConversationLocks) drop(id string, l *convLock) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if l.holders--; l.holders == 0 {
		delete(c.locks, id)
	}
}

// Tracked is how many conversations have a holder or a waiter.
func (c *ConversationLocks) Tracked() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.locks)
}
