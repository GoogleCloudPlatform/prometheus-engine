// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
	"strconv"
	"strings"
)

const ValuesFile = "charts/values.global.yaml"

type Branch struct {
	major int
	minor int
}

func getBranch(branch string) (*Branch, error) {
	re := regexp.MustCompile(`^release/(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(branch)
	if matches == nil {
		return nil, errors.New("branch name must match regex release/([0-9]+).([0-9]+)")
	}
	majorRaw, minorRaw := matches[1], matches[2]
	major, err := strconv.Atoi(majorRaw)
	if err != nil {
		return nil, fmt.Errorf("malformatted branch %q", branch)
	}
	minor, err := strconv.Atoi(minorRaw)
	if err != nil {
		return nil, fmt.Errorf("malformatted branch %q", branch)
	}

	return &Branch{
		major: major,
		minor: minor,
	}, nil
}

type ReleaseCandidate struct {
	branch *Branch
	patch  int
	rc     int
}

func (b *Branch) lastTag() (string, error) {
	tagRegex := fmt.Sprintf("v%d.%d.*", b.major, b.minor)
	tags, err := Cmd("git", "tag", "--list", tagRegex, "--sort=-v:refname").Run()
	return strings.SplitN(tags, "\n", 2)[0], err
}

func (b *Branch) lastReleaseCandidate() (*ReleaseCandidate, error) {
	tag, err := b.lastTag()
	if err != nil {
		return nil, err
	}
	if tag == "" {
		return nil, nil
	}
	re := regexp.MustCompile(`^v\d+.\d+.(\d+)-rc.(\d+)$`)
	matches := re.FindStringSubmatch(tag)
	if matches == nil {
		return nil, fmt.Errorf("malformatted tag name %q", tag)
	}
	patchRaw, rcRaw := matches[1], matches[2]
	patch, err := strconv.Atoi(patchRaw)
	if err != nil {
		return nil, fmt.Errorf("malformatted tag name %q", tag)
	}
	rc, err := strconv.Atoi(rcRaw)
	if err != nil {
		return nil, fmt.Errorf("malformatted tag name %q", tag)
	}
	return &ReleaseCandidate{
		branch: b,
		patch:  patch,
		rc:     rc,
	}, nil
}

func (b *Branch) nextReleaseCandidate() (*ReleaseCandidate, error) {
	last, err := b.lastReleaseCandidate()
	if err != nil {
		return nil, err
	}
	if last == nil {
		return &ReleaseCandidate{
			branch: b,
			patch:  0,
			rc:     0,
		}, nil
	}
	hasRelease, err := last.hasRelease()
	if err != nil {
		return nil, err
	}
	if hasRelease {
		return &ReleaseCandidate{
			branch: last.branch,
			patch:  last.patch + 1,
			rc:     0,
		}, nil
	}
	return &ReleaseCandidate{
		branch: last.branch,
		patch:  last.patch,
		rc:     last.rc + 1,
	}, nil
}

func (rc *ReleaseCandidate) version() string {
	return fmt.Sprintf("v%d.%d.%d", rc.branch.major, rc.branch.minor, rc.patch)
}

func (rc *ReleaseCandidate) hasRelease() (bool, error) {
	out, err := Cmd("git", "tag", "--list", rc.version()).Run()
	return out != "", err
}

func (rc *ReleaseCandidate) updateValuesFile(repoPath string) error {
	path := filepath.Join(repoPath, ValuesFile)
	_, err := Cmd("go", "tool", "yq", "e",
		fmt.Sprintf(`.version = "%d.%d.%d"`, rc.branch.major, rc.branch.minor, rc.patch),
		"-i", path,
	).Run()
	if err != nil {
		return err
	}
	for _, image := range []string{"configReloader", "operator", "ruleEvaluator", "datasourceSyncer"} {
		_, err := Cmd("go", "tool", "yq", "e",
			fmt.Sprintf(`.images.%s.tag = "%s-gke.%d"`, image, rc.version(), rc.rc),
			"-i", path,
		).Run()
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("::error::Usage: %s <branch-name> <repo-path>\n", os.Args[0])
		fmt.Println("::error::Error: Missing arguments")
		os.Exit(1)
	}
	if _, err := Cmd("git", "fetch", "--tags", "-f").Run(); err != nil {
		fmt.Printf("::error::Failed to fetch tags: %v\n", err)
	}
	branch, err := getBranch(os.Args[1])
	if err != nil {
		fmt.Printf("::error::Failed to parse branch: %v\n", err)
	}
	nextRc, err := branch.nextReleaseCandidate()
	if err != nil {
		fmt.Printf("::error::Failed to get next release candidate: %v\n", err)
	}

	repoPath := os.Args[2]
	if err := nextRc.updateValuesFile(repoPath); err != nil {
		fmt.Printf("::error::Failed to update value file: %v\n", err)
	}
	// For GH actions
	outputPath := os.Getenv("GITHUB_OUTPUT")
	if outputPath == "" {
		fmt.Println("::error::GITHUB_OUTPUT environment variable not set.")
		os.Exit(1)
	}
	outputFile, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("::error::Failed to open GITHUB_OUTPUT: %v\n", err)
		return
	}
	defer outputFile.Close()

	s := fmt.Sprintf("full_rc_version=%s-rc.%d\n", nextRc.version(), nextRc.rc)
	_, err = outputFile.WriteString(s)
	if err != nil {
		fmt.Printf("::error::Failed to write to GITHUB_OUTPUT: %v\n", err)
	}
}
