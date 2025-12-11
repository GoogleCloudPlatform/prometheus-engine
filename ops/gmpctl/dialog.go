// Copyright 2025 Google LLC
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

package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
)

func selectBranch(question string) (branch string) {
	var opts []huh.Option[string]
	for _, b := range ReleaseBranches {
		opts = append(opts, huh.NewOption[string](b, b))
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(question).Options(opts...).Value(&branch),
	)).Run(); err != nil {
		os.Exit(1) // Abort.
	}
	return branch
}

func confirmf(question string, args ...any) (confirmed bool) {
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(fmt.Sprintf(question, args...)).Inline(true).Value(&confirmed),
	)).Run(); err != nil {
		os.Exit(1) // Abort.
	}
	return confirmed
}
