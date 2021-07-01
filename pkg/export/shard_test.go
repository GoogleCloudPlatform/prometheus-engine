package export

import (
	"testing"
	"time"
)

func TestEnqueue(t *testing.T) {
	s := newShard(4)
	s.mtx.Lock()
	locked := true

	ch := make(chan bool)
	go func() {
		s.enqueue(1, nil)
		ch <- true
	}()

	// Assume a non-locking enqueue would return
	// before timer fires.
	timer := time.NewTimer(100 * time.Millisecond)
	for {
		select {
		case <-ch:
			if locked {
				t.Error("enqueue mutated a locked shard")
			}
			return
		case <-timer.C:
			s.mtx.Unlock()
			locked = false
		}
	}
}
