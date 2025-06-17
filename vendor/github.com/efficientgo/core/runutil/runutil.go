// Copyright (c) The EfficientGo Authors.
// Licensed under the Apache License 2.0.

// Initially copied from Thanos
//
// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package runutil

import (
	"time"
)

// Repeat executes f every interval seconds until stopc is closed or f returns an error.
// It executes f once right after being called.
func Repeat(interval time.Duration, stopc <-chan struct{}, f func() error) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		if err := f(); err != nil {
			return err
		}
		select {
		case <-stopc:
			return nil
		case <-tick.C:
		}
	}
}

// Logger interface compatible with go-kit/logger.
type Logger interface {
	Log(keyvals ...interface{}) error
}

// Retry executes f every interval seconds until timeout or no error is returned from f.
func Retry(interval time.Duration, stopc <-chan struct{}, f func() error) error {
	return RetryWithLog(nil, interval, stopc, f)
}

// RetryWithLog executes f every interval seconds until timeout or no error is returned from f. It logs an error on each f error.
func RetryWithLog(logger Logger, interval time.Duration, stopc <-chan struct{}, f func() error) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	var err error
	for {
		if err = f(); err == nil {
			return nil
		}
		if logger != nil {
			_ = logger.Log("msg", "function failed. Retrying in next tick", "err", err)
		}
		select {
		case <-stopc:
			return err
		case <-tick.C:
		}
	}
}
