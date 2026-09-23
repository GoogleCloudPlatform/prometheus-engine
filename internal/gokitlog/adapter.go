// Copyright 2026 Google LLC
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

// Package gokitlog provides a github.com/go-kit/log.Logger adapter backed by log/slog.
package gokitlog

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/go-kit/log"
)

type slogAdapter struct {
	logger *slog.Logger
}

// NewAdapter wraps a *slog.Logger to implement the github.com/go-kit/log.Logger interface.
// If logger is nil, a no-op go-kit logger is returned.
func NewAdapter(logger *slog.Logger) log.Logger {
	if logger == nil {
		return log.NewNopLogger()
	}
	return slogAdapter{logger: logger}
}

func (a slogAdapter) Log(keyvals ...any) error {
	var (
		msg  string
		lvl  = slog.LevelInfo
		args = make([]any, 0, len(keyvals))
	)
	for i := 0; i < len(keyvals); i += 2 {
		key := fmt.Sprint(keyvals[i])
		var val any
		if i+1 < len(keyvals) {
			val = keyvals[i+1]
		}
		switch key {
		case "msg":
			msg = fmt.Sprint(val)
		case "level":
			switch strings.ToLower(fmt.Sprint(val)) {
			case "debug":
				lvl = slog.LevelDebug
			case "info":
				lvl = slog.LevelInfo
			case "warn":
				lvl = slog.LevelWarn
			case "error":
				lvl = slog.LevelError
			}
		default:
			args = append(args, key, val)
		}
	}

	if !a.logger.Enabled(context.Background(), lvl) {
		return nil
	}

	var pcs [16]uintptr
	n := runtime.Callers(2, pcs[:])
	var pc uintptr
	if n > 0 {
		pc = pcs[0]
		frames := runtime.CallersFrames(pcs[:n])
		for {
			frame, more := frames.Next()
			if !strings.Contains(frame.Function, "github.com/go-kit/log") && !strings.HasSuffix(frame.Function, "slogAdapter.Log") {
				pc = frame.PC
				break
			}
			if !more {
				break
			}
		}
	}

	r := slog.NewRecord(time.Now(), lvl, msg, pc)
	r.Add(args...)
	_ = a.logger.Handler().Handle(context.Background(), r)
	return nil
}
