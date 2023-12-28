//go:build go1.21
// +build go1.21

/*
Copyright 2023 The logr Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package slogr enables usage of a slog.Handler with logr.Logger as front-end
// API and of a logr.LogSink through the slog.Handler and thus slog.Logger
// APIs.
//
// See the README in the top-level [./logr] package for a discussion of
// interoperability.
//
// Deprecated: use the main logr package instead.
package slogr

import (
	"log/slog"

	"github.com/go-logr/logr"
)

// NewLogr returns a logr.Logger which writes to the slog.Handler.
//
// Deprecated: use [logr.FromSlogHandler] instead.
func NewLogr(handler slog.Handler) logr.Logger {
	return logr.FromSlogHandler(handler)
