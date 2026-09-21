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

package gokitlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/stretchr/testify/require"
)

func TestNewAdapter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	slogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	adapter := log.With(NewAdapter(slogger), "component", "test")

	// Debug should be filtered out by the slog handler configured at Info level.
	require.NoError(t, level.Debug(adapter).Log("msg", "debug message", "k", "v"))
	require.Empty(t, buf.String())

	// Info should be logged with component and key-values.
	require.NoError(t, level.Info(adapter).Log("msg", "info message", "k", "v"))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, "INFO", entry["level"])
	require.Equal(t, "info message", entry["msg"])
	require.Equal(t, "test", entry["component"])
	require.Equal(t, "v", entry["k"])
}
