// Copyright 2023 Google LLC
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

// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

// Copied from https://github.com/thanos-io/thanos/tree/19dcc7902d2431265154cefff82426fbc91448a3/pkg/logging

package logginghttp

import (
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// Decision defines rules for enabling start and end of logging.
type Decision int

const (
	// NoLogCall - Logging is disabled.
	NoLogCall Decision = iota
	// LogFinishCall - Only finish logs of request is enabled.
	LogFinishCall
	// LogStartAndFinishCall - Logging of start and end of request is enabled.
	LogStartAndFinishCall
)

var defaultOptions = &options{
	shouldLog:         DefaultDeciderMethod,
	codeFunc:          DefaultErrorToCode,
	levelFunc:         DefaultCodeToLevel,
	durationFieldFunc: DurationToTimeMillisFields,
	filterLog:         DefaultFilterLogging,
}

func evaluateOpt(opts []Option) *options {
	optCopy := &options{}
	*optCopy = *defaultOptions
	optCopy.levelFunc = DefaultCodeToLevel
	for _, o := range opts {
		o(optCopy)
	}
	return optCopy
}

// WithDecider customizes the function for deciding if the HTTP Middlewares/Tripperwares should log.
func WithDecider(f Decider) Option {
	return func(o *options) {
		o.shouldLog = f
	}
}

// WithLevels customizes the function for mapping HTTP response codes and interceptor log level statements.
func WithLevels(f CodeToLevel) Option {
	return func(o *options) {
		o.levelFunc = f
	}
}

// WithFilter customizes the function for deciding which level of logging should be allowed.
// Follows go-kit Allow<level of log> convention.
func WithFilter(f FilterLogging) Option {
	return func(o *options) {
		o.filterLog = f
	}
}

type Option func(*options)

// Fields represents logging fields. It has to have even number of elements (pairs).
type Fields []string

// ErrorToCode function determines the error code of the error
// for the http response.
type ErrorToCode func(err error) int

// DefaultErrorToCode returns an InternalServerError.
func DefaultErrorToCode(_ error) int {
	return 500
}

// Decider function defines rules for suppressing the logging.
type Decider func(methodName string, err error) Decision

// DefaultDeciderMethod is the default implementation of decider to see if you should log the call
// by default this is set to LogStartAndFinishCall.
func DefaultDeciderMethod(_ string, _ error) Decision {
	return LogStartAndFinishCall
}

// CodeToLevel function defines the mapping between HTTP Response codes to log levels for server side.
type CodeToLevel func(logger log.Logger, code int) log.Logger

// DurationToFields function defines how to produce duration fields for logging.
type DurationToFields func(duration time.Duration) Fields

// FilterLogging makes sure only the logs with level=lvl gets logged, or filtered.
type FilterLogging func(logger log.Logger) log.Logger

// DefaultFilterLogging allows logs from all levels to be logged in output.
func DefaultFilterLogging(logger log.Logger) log.Logger {
	return level.NewFilter(logger, level.AllowAll())
}

type options struct {
	levelFunc         CodeToLevel
	shouldLog         Decider
	codeFunc          ErrorToCode
	durationFieldFunc DurationToFields
	filterLog         FilterLogging
}

// DefaultCodeToLevel is the helper mapper that maps HTTP Response codes to log levels.
func DefaultCodeToLevel(logger log.Logger, code int) log.Logger {
	if code >= 200 && code < 500 {
		return level.Debug(logger)
	}
	return level.Error(logger)
}

// DurationToTimeMillisFields converts the duration to milliseconds and uses the key `http.time_ms`.
func DurationToTimeMillisFields(duration time.Duration) Fields {
	return Fields{"http.time_ms", fmt.Sprintf("%v", durationToMilliseconds(duration))}
}

func durationToMilliseconds(duration time.Duration) float32 {
	return float32(duration.Nanoseconds()/1000) / 1000
}
