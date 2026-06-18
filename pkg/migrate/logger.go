// Copyright 2026 Google LLC
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

package migrate

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// LevelSuccess defines a custom slog.Level for Success (standard Info is 0, Warn is 4).
const LevelSuccess slog.Level = slog.LevelInfo + 1 // Level 1

// ConsoleHandler is a thread-safe slog.Handler that formats logs for the console (Stderr)
// and tracks the highest log level seen per resource (for statistics).
type ConsoleHandler struct {
	mu             sync.Mutex
	out            io.Writer
	resourceLevels map[string]slog.Level
	attrs          []slog.Attr
}

// NewConsoleHandler creates a new ConsoleHandler.
func NewConsoleHandler(out io.Writer) *ConsoleHandler {
	return &ConsoleHandler{
		out:            out,
		resourceLevels: make(map[string]slog.Level),
	}
}

func (h *ConsoleHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true // Log everything
}

func (h *ConsoleHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var kind, namespace, name, file string
	var extraAttrs []string

	// Helper to process and categorize attributes
	processAttr := func(a slog.Attr) {
		switch a.Key {
		case "kind":
			kind = a.Value.String()
		case "namespace":
			namespace = a.Value.String()
		case "name":
			name = a.Value.String()
		case "file":
			file = a.Value.String()
		default:
			// Collect all other attributes to print at the end of the line
			extraAttrs = append(extraAttrs, fmt.Sprintf("%s=%v", a.Key, a.Value.Any()))
		}
	}

	// Extract attributes bound to the logger instance
	for _, a := range h.attrs {
		processAttr(a)
	}

	// Extract attributes passed in the individual log call
	r.Attrs(func(a slog.Attr) bool {
		processAttr(a)
		return true
	})

	// Map slog.Level to string for console output.
	var levelStr string
	switch r.Level {
	case slog.LevelDebug:
		levelStr = "DEBUG"
	case slog.LevelInfo:
		levelStr = "INFO"
	case LevelSuccess:
		levelStr = "SUCCESS"
	case slog.LevelWarn:
		levelStr = "WARNING"
	case slog.LevelError:
		levelStr = "ERROR"
	default:
		levelStr = r.Level.String()
	}

	// Format prefix cleanly
	var prefix string
	if file != "" {
		prefix = fmt.Sprintf("[%s] ", file)
	} else if kind != "" && name != "" {
		prefix = fmt.Sprintf("[%s:%s/%s] ", kind, namespace, name)
	}

	// 1. Write formatted log to Stderr (console), appending extra attributes if any.
	var suffix string
	if len(extraAttrs) > 0 {
		suffix = " " + strings.Join(extraAttrs, " ")
	}
	consoleLine := fmt.Sprintf("[%s] %s%s%s\n", levelStr, prefix, r.Message, suffix)
	if _, err := io.WriteString(h.out, consoleLine); err != nil {
		return err
	}

	// 2. Track the highest log level seen for this resource (for final report)
	if kind != "" && name != "" {
		key := fmt.Sprintf("%s/%s/%s", kind, namespace, name)
		if r.Level > h.resourceLevels[key] {
			h.resourceLevels[key] = r.Level
		}
	} else if file != "" {
		// Track file-level log severity under the file path key
		if r.Level > h.resourceLevels[file] {
			h.resourceLevels[file] = r.Level
		}
	}

	return nil
}

func (h *ConsoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := append(h.attrs, attrs...)
	return &ConsoleHandler{
		out:            h.out,
		resourceLevels: h.resourceLevels,
		attrs:          newAttrs,
	}
}

func (h *ConsoleHandler) WithGroup(_ string) slog.Handler {
	return h
}
