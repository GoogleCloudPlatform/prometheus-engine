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

package main

import (
	"encoding/json"
	"strings"
)

type prInfo struct {
	URL string `json:"url"`
}

func mustEnsurePullRequest(dir, baseBranch, headBranch, title, body string) {
	logf("Checking for existing pull request for %v...", headBranch)

	// gh pr list --head <head> --base <base> --state open --json url
	out, err := runCommand(
		&cmdOpts{Dir: dir, HideOutputs: true},
		"gh", "pr", "list",
		"--head", headBranch,
		"--base", baseBranch,
		"--state", "open",
		"--json", "url",
	)
	if err != nil {
		panicf("failed to check existing PR: %v", err)
	}

	var prs []prInfo
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		panicf("failed to parse gh output: %v, output: %q", err, out)
	}

	if len(prs) > 0 {
		logf("Pull request already exists: %v", prs[0].URL)
		return
	}

	logf("Creating pull request from %v to %v...", headBranch, baseBranch)

	// gh pr create --title <title> --body <body> --base <base> --head <head>
	prURL, err := runCommand(
		&cmdOpts{Dir: dir},
		"gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--base", baseBranch,
		"--head", headBranch,
	)
	if err != nil {
		panicf("failed to create pull request: %v", err)
	}
	logf("Pull request created: %v", strings.TrimSpace(prURL))
}
