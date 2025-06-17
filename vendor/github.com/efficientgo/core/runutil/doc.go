// Copyright (c) The EfficientGo Authors.
// Licensed under the Apache License 2.0.

// Package runutil implements helpers for advanced function scheduling control like repeat or retry.
//
// It's very often the case when you need to excutes some code every fixed intervals or have it retried automatically.
// To make it reliably with proper timeout, you need to carefully arrange some boilerplate for this.
// Below function does it for you.
//
// For repeat executes, use Repeat:
//
//	err := runutil.Repeat(10*time.Second, stopc, func() error {
//		// ...
//	})
//
// Retry starts executing closure function f until no error is returned from f:
//
//	err := runutil.Retry(10*time.Second, stopc, func() error {
//		// ...
//	})
//
// For logging an error on each f error, use RetryWithLog:
//
//	err := runutil.RetryWithLog(logger, 10*time.Second, stopc, func() error {
//		// ...
//	})
package runutil
