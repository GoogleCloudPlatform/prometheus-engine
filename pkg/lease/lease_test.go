// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lease

import (
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func TestWrappedLock_Update(t *testing.T) {
	cases := []struct {
		desc               string
		record             resourcelock.LeaderElectionRecord
		err                error
		wantOK             bool
		wantStart, wantEnd time.Time
	}{
		{
			desc: "OK",
			record: resourcelock.LeaderElectionRecord{
				AcquireTime:          metav1.Unix(100, 0),
				RenewTime:            metav1.Unix(200, 0),
				LeaseDurationSeconds: 20,
			},
			err:       nil,
			wantOK:    true,
			wantStart: time.Unix(100, 0),
			wantEnd:   time.Unix(220, 0),
		},
		{
			desc: "has error",
			record: resourcelock.LeaderElectionRecord{
				AcquireTime:          metav1.Unix(100, 0),
				RenewTime:            metav1.Unix(200, 0),
				LeaseDurationSeconds: 20,
			},
			err:    errors.New("test"),
			wantOK: false,
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			wl := &wrappedLock{}

			wl.update(c.record, c.err)
			start, end, ok := wl.Range()
			if ok != c.wantOK {
				t.Fatalf("unexpected 'ok': want %v, got %v", c.wantOK, ok)
			}
			if !start.Equal(c.wantStart) {
				t.Fatalf("unexpected 'start: want %v, got %v", c.wantStart, start)
			}
			if !end.Equal(c.wantEnd) {
				t.Fatalf("unexpected 'end: want %v, got %v", c.wantEnd, end)
			}
		})
	}
}
