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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	Prometheus = Project{
		Name:      "prometheus",
		remoteURL: "git@github.com:GoogleCloudPlatform/prometheus.git",
		BranchRE:  regexp.MustCompile(`^release-[23]\.[0-9]+\.[0-9]+-gmp$`),
	}
	Alertmanager = Project{
		Name:      "alertmanager",
		remoteURL: "git@github.com:GoogleCloudPlatform/alertmanager.git",
		BranchRE:  regexp.MustCompile(`^release-0\.[0-9]+\.[0-9]+-gmp$`),
	}
	PrometheusEngine = Project{
		Name:      "prometheus-engine",
		remoteURL: "git@github.com:GoogleCloudPlatform/prometheus-engine.git",
		BranchRE:  regexp.MustCompile(`^release/0\.[0-9]+$`),
	}

	// ReleaseBranches contains hardcoded list of active branches. We could pull it out from somewhere.
	ReleaseBranches = []string{
		"release/0.18",
		"release/0.17",
		"release/0.15",
		"release/0.14",
		"release/0.12",
		"release-2.45.3-gmp",
		"release-2.53.5-gmp",
		"release-0.27.0-gmp",
	}
)

func projectFromBranch(branch string) (Project, bool) {
	switch {
	case Prometheus.BranchRE.MatchString(branch):
		return Prometheus, true
	case Alertmanager.BranchRE.MatchString(branch):
		return Alertmanager, true
	case PrometheusEngine.BranchRE.MatchString(branch):
		return PrometheusEngine, true
	}
	return Project{}, false
}

type Project struct {
	Name      string
	remoteURL string
	BranchRE  *regexp.Regexp
}

func (p Project) cloneDir(dir string) (cloneDir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panicf(err.Error())
		}
	}

	cloneDir = filepath.Join(dir, p.Name)
	_, err := os.Stat(cloneDir)
	if err == nil {
		return cloneDir
	}
	if !errors.Is(err, os.ErrNotExist) {
		panicf("failed to stat %s: %v", cloneDir, err)
	}
	logf("Cloning %q into %q", p.RemoteURL(), cloneDir)
	mustCloneRepo(p.RemoteURL(), cloneDir)
	return cloneDir
}

func (p Project) workDir(dir, branch, suffix string) string {
	subDir := strings.ToLower(fmt.Sprintf("%v_%v", branch, suffix))
	subDir = strings.ReplaceAll(subDir, "/", "_")
	return filepath.Join(dir, p.Name, subDir)
}

func (p Project) RemoteURL() string {
	if *gitPreferHTTPS {
		return "https://" +
			strings.TrimSuffix(
				strings.TrimPrefix(strings.ReplaceAll(p.remoteURL, ":", "/"), "git@"),
				".git")
	}
	return p.remoteURL
}

// WorkDir returns a new working directory.
func (p Project) WorkDir(dir, branch, suffix string) (workDir string) {
	cloneDir := p.cloneDir(dir)

	workDir = p.workDir(dir, branch, suffix)
	if _, err := os.Stat(workDir); err == nil {
		if confirmf("Found worktree %q; do you want to reuse it (without reset)?", workDir) {
			logf("Reusing %q worktree", workDir)
			return workDir
		}
		logf("Removing %q worktree", workDir)
		mustRemoveWorktree(cloneDir, workDir)
	}

	logf("Creating new worktree %q from %q", workDir, cloneDir)
	mustAddWorktree(cloneDir, workDir, branch)
	// Whenever we start work tree, we
	return workDir
}

func (p Project) RemoveWorkDir(dir, workDir string) {
	_, err := os.Stat(workDir)
	if err == nil {
		mustRemoveWorktree(filepath.Join(dir, p.Name), workDir)
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		panicf("failed to stat %s: %v", workDir, err)
	}
}
