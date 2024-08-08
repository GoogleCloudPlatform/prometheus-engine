// Copyright 2024 Google LLC
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

// Package prompb and its io/prometheus/write/v2 subpackage was copied from
// https://github.com/prometheus/prometheus/tree/e044028cbdd707ca6bce9f38df2ccf0e2937a6b8/prompb
// to avoid using deprecated gogoproto (instead we use vtproto for efficiency).
package prompb
