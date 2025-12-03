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
	"errors"
	"flag"
	"fmt"
	"os"
)

var (
	vulnfixFlags               = flag.NewFlagSet("vulnfix", flag.ExitOnError)
	vulnfixBranch              = vulnfixFlags.String("b", "", "Release branch to work on; Project is auto-detected from this")
	vulnfixPRBranch            = vulnfixFlags.String("pr-branch", "", "(default: $USER/BRANCH-vulnfix) Upstream branch to push to (user-confirmed first).")
	vulnfixSyncDockerfilesFrom = vulnfixFlags.Bool("sync-dockerfiles-from", false, "Optional branch name to sync Dockerfiles from. Useful when things changed.")
)

// Attempt a minimal dependency upgrade to solve fixable vulnerabilities.
//
// * Docker images:
//   - Distros use latest tag so rebuilding takes latest, nothing to do.
//   - google-go.pkg.dev/golang images are updated to the latest minor version using docker-bump-images.sh
//
// * Manifests
//   - distroless bumped to latest (although our component tooling is capable of bumping this too)
//
// * Go deps: Upgrade to minimal required version per a known fixable vulnerability.
// * Npm deps: Not implemented.
//
// NOTE: The script is idempotent; to force it to recreate local artifacts (e.g. local clones, remote branches it created), remove the artifact you want to recreate.
func vulnfix() error {
	_ = vulnfixFlags.Parse(flag.Args()[1:])

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	var (
		proj     Project
		branch   = *vulnfixBranch
		prBranch = *vulnfixPRBranch
		ok       bool
	)
	if branch == "" {
		branch = selectBranch("What do you want to release?")
	}
	proj, ok = projectFromBranch(branch)
	if !ok {
		return fmt.Errorf("couldn't find project from branch %s", branch)
	}

	// TODO(bwplotka): We are force pushing prBranch. Shall we add safety check it's not a release branch?
	// Perhaps validated in mustForcePush?
	if prBranch == "" {
		prBranch = fmt.Sprintf("%v/%v-gmpctl-vulnfix", os.Getenv("USER"), branch)
	}

	logf("Assuming %q with remote %q; on %q; changes will be pushed to %q", proj.Name, proj.RemoteURL, branch, prBranch)
	dir := proj.WorkDir(cfg.Directory, branch, "vulnfix")

	// Refresh.
	mustFetchAll(dir)

	opts := []string{
		fmt.Sprintf("DIR=%v", dir),
		fmt.Sprintf("BRANCH=%v", branch),
		fmt.Sprintf("PROJECT=%v", proj.Name),
	}
	if *vulnfixSyncDockerfilesFrom {
		opts = append(opts, "SYNC_DOCKERFILES_FROM=true")
	}

	// TODO(bwplotka): Add NPM vulnfix.
	if err := runLibFunction(dir, opts, "release-lib::vulnfix"); err != nil {
		return err
	}

	// TODO: Warn of unstaged files at this point.

	// Commit if anything is staged.
	msg := fmt.Sprintf("google patch[deps]: fix %v vulnerabilities", branch)
	if proj.Name == "prometheus-engine" {
		msg = fmt.Sprintf("fix: fix %v vulnerabilities", branch)
	}
	// TODO(bwplotka): Port to Go, make it more reliable.
	// TODO(bwplotka): Quote otherwise it's split into separate args... port it so it works better (:
	if err := runLibFunction(dir, nil, "release-lib::idemp::git_commit_amend_match", "\""+msg+"\""); err != nil {
		return err
	}
	// TODO(bwplotka): Check if needs pushing?
	// TODO(bwplotka): Add option to print more debug/open terminal with the workdir?
	if confirmf("About to FORCE git push state from %q to \"origin/%v\"; are you sure?", dir, prBranch) {
		// We are in detached state, so be explicit what to push and from where.
		mustRecreateBranch(dir, prBranch)
		mustForcePush(dir, prBranch)
	} else {
		return errors.New("aborting")
	}

	if confirmf("Do you want to remove the %v worktree (recommended)?", dir) {
		proj.RemoveWorkDir(cfg.Directory, dir)
	}
	return nil
}
