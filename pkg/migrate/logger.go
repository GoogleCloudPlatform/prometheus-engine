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
	"maps"
	"os"
	"strings"
	"sync"
)

// LevelSuccess defines a custom slog.Level for Success (standard Info is 0, Warn is 4).
const LevelSuccess slog.Level = slog.LevelInfo + 1 // Level 1

// loggerState encapsulates the shared, thread-safe state across all handler clones.
type loggerState struct {
	mu             sync.Mutex
	resourceLevels map[string]slog.Level
}

// ConsoleHandler is a thread-safe slog.Handler that formats logs for the console (Stderr)
// and tracks the highest log level seen per resource (for statistics).
type ConsoleHandler struct {
	out   io.Writer
	state *loggerState
	attrs []slog.Attr
}

// NewConsoleHandler creates a new ConsoleHandler.
func NewConsoleHandler(out io.Writer) *ConsoleHandler {
	if out == nil {
		out = os.Stderr
	}
	return &ConsoleHandler{
		out: out,
		state: &loggerState{
			resourceLevels: make(map[string]slog.Level),
		},
	}
}

func (h *ConsoleHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true // Log everything
}

func (h *ConsoleHandler) Handle(_ context.Context, r slog.Record) error {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	var kind, namespace, name, file string
	var extraAttrs []string

	// Helper to process and categorize attributes
	processAttr := func(a slog.Attr) {
		val := a.Value.Resolve()
		switch a.Key {
		case "kind":
			kind = val.String()
		case "namespace":
			namespace = val.String()
		case "name":
			name = val.String()
		case "file":
			file = val.String()
		default:
			// Collect all other attributes to print at the end of the line
			extraAttrs = append(extraAttrs, fmt.Sprintf("%s=%v", a.Key, val.Any()))
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
		if namespace == "" {
			prefix = fmt.Sprintf("[%s:%s] ", kind, name)
		} else {
			prefix = fmt.Sprintf("[%s:%s/%s] ", kind, namespace, name)
		}
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
		var key string
		if namespace == "" {
			key = fmt.Sprintf("%s/%s", kind, name)
		} else {
			key = fmt.Sprintf("%s/%s/%s", kind, namespace, name)
		}
		if val, exists := h.state.resourceLevels[key]; !exists || r.Level > val {
			h.state.resourceLevels[key] = r.Level
		}
	} else if file != "" {
		// Track file-level log severity under the file path key
		if val, exists := h.state.resourceLevels[file]; !exists || r.Level > val {
			h.state.resourceLevels[file] = r.Level
		}
	}

	return nil
}

func (h *ConsoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// 1. Allocate a brand-new, independent underlying array of exact size
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))

	// 2. Copy the parent's attributes to the beginning of the new array
	copy(newAttrs, h.attrs)

	// 3. Copy the new attributes to the remaining space, starting at the offset
	copy(newAttrs[len(h.attrs):], attrs)
	return &ConsoleHandler{
		out:   h.out,
		state: h.state,
		attrs: newAttrs,
	}
}

func (h *ConsoleHandler) WithGroup(_ string) slog.Handler {
	return h
}

// ResourceLevels returns a thread-safe copy of the tracked resource log levels.
func (h *ConsoleHandler) ResourceLevels() map[string]slog.Level {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	return maps.Clone(h.state.resourceLevels)
}
