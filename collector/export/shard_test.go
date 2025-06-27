// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
