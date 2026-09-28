// Ported from the limiter and lock cases in chat/tests/unit/test_concurrency.py.
package httpapi_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/httpapi"
)

func TestLimiterRefusesAfterTheWaitThenAdmits(t *testing.T) {
	l := httpapi.NewTurnLimiter(1, 50*time.Millisecond)
	release, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = l.Acquire(context.Background())
	waited := time.Since(start)
	var busy httpapi.ErrTurnsBusy
	if !errors.As(err, &busy) || busy.Limit != 1 {
		t.Fatalf("err %v, want ErrTurnsBusy{1}", err)
	}
	if waited < 50*time.Millisecond || waited > time.Second {
		t.Errorf("refused after %v, want about 50ms", waited)
	}

	release()
	release() // a second call frees nothing more
	again, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	defer again()
	if _, err := l.Acquire(context.Background()); !errors.As(err, &busy) {
		t.Errorf("a double release freed a second slot: %v", err)
	}
}

func TestLimiterTinyWaitStillAdmitsAFreeSlot(t *testing.T) {
	release, err := httpapi.NewTurnLimiter(1, time.Nanosecond).Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestLimiterWaiterLeavesWithItsContext(t *testing.T) {
	l := httpapi.NewTurnLimiter(1, time.Minute)
	release, _ := l.Acquire(context.Background())
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := l.Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err %v, want the context's", err)
	}
}

func TestLocksLeaveNoEntriesBehind(t *testing.T) {
	c := httpapi.NewConversationLocks()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := c.Lock(context.Background(), fmt.Sprintf("c%d", i%10))
			if err != nil {
				t.Error(err)
				return
			}
			unlock()
		}()
	}
	wg.Wait()
	if n := c.Tracked(); n != 0 {
		t.Errorf("%d entries left", n)
	}
}

func TestLocksSerialiseOneConversation(t *testing.T) {
	c := httpapi.NewConversationLocks()
	var mu sync.Mutex
	var order []string
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, _ := c.Lock(context.Background(), "c1")
			record("enter")
			time.Sleep(20 * time.Millisecond)
			record("exit")
			unlock()
		}()
	}
	wg.Wait()
	if want := []string{"enter", "exit", "enter", "exit"}; !reflect.DeepEqual(order, want) {
		t.Errorf("order %v", order)
	}
}

func TestLocksDifferentConversationsOverlap(t *testing.T) {
	c := httpapi.NewConversationLocks()
	a, _ := c.Lock(context.Background(), "a")
	defer a()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := c.Lock(ctx, "b")
	if err != nil {
		t.Fatalf("b waited on a: %v", err)
	}
	b()
}

func TestLocksCancelledWaiterLeaves(t *testing.T) {
	c := httpapi.NewConversationLocks()
	unlock, _ := c.Lock(context.Background(), "c1")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := c.Lock(ctx, "c1")
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("err %v", err)
	}
	if n := c.Tracked(); n != 1 {
		t.Errorf("tracked %d while held", n)
	}
	unlock()
	if n := c.Tracked(); n != 0 {
		t.Errorf("tracked %d: the waiter was never counted out", n)
	}
}

func TestLocksSurviveWhileASecondTurnWaits(t *testing.T) {
	c := httpapi.NewConversationLocks()
	first, _ := c.Lock(context.Background(), "c1")

	got := make(chan func())
	go func() {
		unlock, _ := c.Lock(context.Background(), "c1")
		got <- unlock
	}()
	time.Sleep(50 * time.Millisecond) // let the waiter register
	first()
	if n := c.Tracked(); n != 1 {
		t.Errorf("tracked %d with a waiter", n)
	}
	(<-got)()
	if n := c.Tracked(); n != 0 {
		t.Errorf("tracked %d", n)
	}
}
