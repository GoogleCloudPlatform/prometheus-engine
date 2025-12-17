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
	"strings"
)

func mustCloneRepo(repoURL, destinationDir string) {
	if _, err := runCommand(nil, "git", "clone", repoURL, destinationDir); err != nil {
		panicf(err.Error())
	}
}

func mustAddWorktree(dir, newWorktreeDir, branchName string) {
	// TODO(bwplotka): Ideally we want fresh changes.
	mustFetchAll(dir)
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "worktree", "add", newWorktreeDir, "origin/"+branchName); err != nil {
		panicf(err.Error())
	}
}

func mustRemoveWorktree(dir, worktreeDir string) {
	// Without force and some local modifications, worktree fails with "contains
	// modified or untracked files, use --force to delete it".
	// TODO(bwplotka): Perhaps a good safety guard?
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "worktree", "remove", "-f", worktreeDir); err != nil {
		panicf(err.Error())
	}
}

func mustFetchAll(dir string) {
	logf("Fetching origin...")
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "fetch", "--tags"); err != nil {
		panicf("failed to fetch: %v", err)
	}
}

func mustCreateSignedTag(dir, tag string) {
	logf("Creating a signed tag %v...", tag)

	// explicit TTY is often needed on Macs.
	// TODO(bwplotka): Consider adding v0.x second tag for Prometheus fork (similar to how v0.300 Prometheus releases are structured).
	// This is to have a little bit cleaner prometheus-engine go.mod version against the fork.
	if _, err := runCommand(
		&cmdOpts{Dir: dir},
		"bash", "-c",
		fmt.Sprintf("GPG_TTY=$(tty) git tag -s %v -m %v", tag, tag),
	); err != nil {
		panicf(err.Error())
	}
}

// mustIsRemoteUpToDate returns true if HEAD points to the same commit as
// the origin branch
func mustIsRemoteUpToDate(dir, branch string) bool {
	// Fetch to ensure we have the latest remote state.
	mustFetchAll(dir)

	// Get the commit hash of the local HEAD.
	localHead, err := runCommand(&cmdOpts{Dir: dir, HideOutputs: true}, "git", "rev-parse", "HEAD")
	if err != nil {
		panicf(err.Error())
	}

	// Get the commit hash of the remote branch.
	remoteHead, err := runCommand(&cmdOpts{Dir: dir, HideOutputs: true}, "git", "rev-parse", "origin/"+branch)
	if err != nil {
		panicf(err.Error())
	}
	return strings.TrimSpace(localHead) == strings.TrimSpace(remoteHead)
}

func mustPush(dir, what string) {
	logf("Pushing %v...", what)
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "push", "origin", what); err != nil {
		panicf("failed to push: %v", err)
	}
}

func mustForcePush(dir, what string) {
	logf("FORCE Pushing %v...", what)
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "push", "--force", "origin", what); err != nil {
		panicf("failed to force push: %v", err)
	}
}

func mustRecreateBranch(dir, branch string) {
	// TODO(bwplotka): Yolo, check error etc.
	_, _ = runCommand(&cmdOpts{Dir: dir}, "git", "branch", "-D", branch)
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "branch", branch); err != nil {
		panicf("failed to  fopush: %v", err)
	}
}

func checkoutBranch(dir, branchName string) {
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "checkout", branchName); err != nil {
		panicf(err.Error())
	}
}

func resetBranch(dir, target string) {
	if _, err := runCommand(&cmdOpts{Dir: dir}, "git", "reset", "--hard", target); err != nil {
		panicf(err.Error())
	}
}

func getLatestTagForBranch(dir, branchName string) (string, error) {
	// --abbrev=0 suppresses the suffix (e.g., returns "v1.0" instead of "v1.0-5-g3a1b2")
	// --tags allows lightweight tags, not just annotated ones
	tag, err := runCommand(&cmdOpts{Dir: dir}, "git", "describe", "--tags", "--abbrev=0", branchName)
	if err != nil {
		return "", fmt.Errorf("no tags found reachable from %s or error: %w", branchName, err)
	}
	return tag, nil
}
