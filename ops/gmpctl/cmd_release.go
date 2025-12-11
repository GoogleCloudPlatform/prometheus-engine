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
)

var (
	releaseFlags  = flag.NewFlagSet("release", flag.ExitOnError)
	releaseBranch = releaseFlags.String("b", "", "Release branch to work on; Project is auto-detected from this")
	releaseTag    = releaseFlags.String("t", "", "Tag to release. If empty, next TAG version will be auto-detected (double check this!)")
	releasePatch  = releaseFlags.Bool("patch", false, "If true, and --tag is empty, forces a new patch version as a new TAG.")
)

func release() error {
	_ = releaseFlags.Parse(flag.Args()[1:])

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	var (
		proj   Project
		branch = *releaseBranch
		tag    = *releaseTag
		ok     bool
	)
	if branch == "" {
		branch = selectBranch("What do you want to release?")
	}
	proj, ok = projectFromBranch(branch)
	if !ok {
		return fmt.Errorf("couldn't find project from branch %s", branch)
	}

	logf("Assuming %q with remote %q; branch to release: %q", proj.Name, proj.RemoteURL(), branch)
	dir := proj.WorkDir(cfg.Directory, branch, "release")

	mustFetchAll(dir)

	if tag == "" {
		var envs []string
		if *releasePatch {
			envs = append(envs, "FORCE_NEW_PATCH_VERSION=true")
		}
		tag, err = getFromLibFunction(dir, envs, "release-lib::next_release_tag", dir)
		if err != nil {
			return err
		}
	}
	logf("Selected %v tag", tag)

	if err := runLocalBash(dir, []string{
		fmt.Sprintf("DIR=%v", dir),
		fmt.Sprintf("BRANCH=%v", branch),
		fmt.Sprintf("PROJECT=%v", proj.Name),
		fmt.Sprintf("TAG=%v", tag),
	}, "prep-rc.sh"); err != nil {
		return err
	}

	msg := fmt.Sprintf("chore: prepare for %v release", tag)
	// TODO(bwplotka): Port to Go, make it more reliable.
	// TODO(bwplotka): Quote otherwise it's split into separate args... port it so it works better (:
	// TODO(bwplotka): Add message about a script command.
	if err := runLibFunction(dir, nil, "release-lib::idemp::git_commit_amend_match", "\""+msg+"\""); err != nil {
		return err
	}

	if !mustIsRemoteUpToDate(dir, branch) {
		if confirmf("About to git push state from %q to \"origin/%v\" for %q tag; are you sure?", dir, branch, tag) {
			// We are in detached state, so use the HEAD reference.
			mustPush(dir, fmt.Sprintf("HEAD:%v", branch))
		} else {
			return errors.New("aborting")
		}
	}

	// TODO(bwplotka): Check if tag exists.
	mustCreateSignedTag(dir, tag)
	if confirmf("About to git push %q tag from %q to \"origin/%v\"; are you sure?", tag, dir, branch) {
		mustPush(dir, tag)
	} else {
		return errors.New("aborting")
	}
	if confirmf("Do you want to remove the %v worktree (recommended)?", dir) {
		proj.RemoveWorkDir(cfg.Directory, dir)
	}
	return nil
}
