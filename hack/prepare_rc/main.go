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

// package main implements prepare_rc script.
//
// Run this script to prepare the prometheus-engine files for the next release;
// without regenerating them (!).
//
// Example use:
//
//	go run ./... \
//		-tag=v0.15.4-rc.0 \
//		-dir=../../../prometheus-engine
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	versionFile = "pkg/export/export.go"
	valuesFile  = "charts/values.global.yaml"
)

var (
	dir    = flag.String("dir", ".", "The path with prometheus-engine repo/")
	tag    = flag.String("tag", "", "Tag to release. Empty means the tag will be a next RC number from the last git tag in a given -branch.")
	branch = flag.String("branch", "", "Branch to use for auto-detecting tag to release. Must be matching ^release/(\\d+)\\.(\\d+)$. Can't be used with -tag.")
)

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

func (b *Branch) currentTag() (Tag, error) {
	tagRegex := fmt.Sprintf("v%d.%d.*", b.major, b.minor)
	tags, err := Cmd("git", "tag", "--list", tagRegex, "--sort=-v:refname").Run()
	if err != nil {
		return Tag{}, err
	}
	return NewTag(strings.SplitN(tags, "\n", 2)[0])
}

func updateVersionFile(repoPath string, tag Tag) error {
	path := filepath.Join(repoPath, versionFile)
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}
	content := string(contentBytes)
	re := regexp.MustCompile(`(mainModuleVersion\s*=\s*)".*"`)
	replacement := fmt.Sprintf(`${1}"%s"`, tag.String())
	newContent := re.ReplaceAllString(content, replacement)

	err = os.WriteFile(path, []byte(newContent), 0664)
	if err != nil {
		return fmt.Errorf("failed to write updated content to %s: %w", path, err)
	}
	return nil
}

func updateValuesFile(repoPath string, tag Tag) error {
	path := filepath.Join(repoPath, valuesFile)
	if _, err := Cmd("yq", "e",
		fmt.Sprintf(`.version = "%d.%d.%d"`, tag.Major(), tag.Minor(), tag.Patch()),
		"-i", path,
	).Run(); err != nil {
		return err
	}
	for _, image := range []string{"configReloader", "operator", "ruleEvaluator", "datasourceSyncer"} {
		_, err := Cmd("yq", "e",
			// This assumes RC number is always same as the GKE number -- this is not always
			// correct e.g. when we retry releases on Louhi side. It's the best guess though
			// and we can update manifests later on.
			fmt.Sprintf(`.images.%s.tag = "v%d.%d.%d-gke.%d"`, image, tag.Major(), tag.Minor(), tag.Patch(), tag.RC()),
			"-i", path,
		).Run()
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	flag.Parse()

	if *dir == "" {
		fmt.Println("::error::Error: -dir has to be specified.")
		flag.Usage()
		os.Exit(1)
	}

	if *tag != "" && *branch != "" {
		fmt.Println("::error::Error: -tag and -branch can't be specified in the same time.")
		flag.Usage()
		os.Exit(1)
	}

	var releaseTag Tag
	if *tag != "" {
		var err error
		releaseTag, err = NewTag(*tag)
		if err != nil {
			fmt.Printf("::error::Failed to parse the -tag: %v\n", err)
			os.Exit(1)
		}
	} else if *branch != "" {
		// TODO(bwplotka): Why we ask for repo directory if we this command require this
		// script to be in the current directory? Fix it.
		if _, err := Cmd("git", "fetch", "--tags", "-f").Run(); err != nil {
			fmt.Printf("::error::Failed to fetch tags: %v\n", err)
		}
		branch, err := getBranch(os.Args[1])
		if err != nil {
			fmt.Printf("::error::Failed to parse branch: %v\n", err)
		}
		ct, err := branch.currentTag()
		if err != nil {
			fmt.Printf("::error::Failed to get next release candidate: %v\n", err)
		}
		releaseTag = ct.NextRC()
	} else {
		fmt.Println("::error::Error: Either -tag or -branch has to be specified.")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println("Updating files to", tag)
	if err := updateVersionFile(*dir, releaseTag); err != nil {
		fmt.Printf("::error::Failed to update version file: %v\n", err)
		os.Exit(1)
	}
	if err := updateValuesFile(*dir, releaseTag); err != nil {
		fmt.Printf("::error::Failed to update value file: %v\n", err)
		os.Exit(1)
	}

	// For GH actions, optionally.
	outputPath := os.Getenv("GITHUB_OUTPUT")
	if outputPath != "" {
		outputFile, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("::error::Failed to open GITHUB_OUTPUT: %v\n", err)
			return
		}
		defer outputFile.Close()

		s := fmt.Sprintf("full_rc_version=%v\n", releaseTag.String())
		_, err = outputFile.WriteString(s)
		if err != nil {
			fmt.Printf("::error::Failed to write to GITHUB_OUTPUT: %v\n", err)
		}
	}
}
