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
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	vulnfixFlags               = flag.NewFlagSet("vulnfix", flag.ExitOnError)
	vulnfixBranch              = vulnfixFlags.String("b", "", "Release branch to work on; Project is auto-detected from this")
	vulnfixPRBranch            = vulnfixFlags.String("pr-branch", "", "(default: $USER/BRANCH-vulnfix) Upstream branch to push to (user-confirmed first).")
	vulnfixSyncDockerfilesFrom = vulnfixFlags.Bool("sync-dockerfiles-from", false, "Optional branch name to sync Dockerfiles from. Useful when things changed.")
	vulnfixGoVersion           = vulnfixFlags.String("go-version", "", "Go minor version to use for docker images.")
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

	logf("Assuming %q with remote %q; on %q; changes will be pushed to %q", proj.Name, proj.RemoteURL(), branch, prBranch)
	dir := proj.WorkDir(cfg.Directory, branch, "vulnfix")

	// Refresh.
	mustFetchAll(dir)

	goVersion := *vulnfixGoVersion
	if goVersion == "" {
		goVersion, err = detectGoMinorVersion(dir)
		if err != nil {
			return fmt.Errorf("could not detect Go version from Dockerfile: %v", err)
		}
	}
	logf("Using Go version: %s", goVersion)

	opts := []string{
		fmt.Sprintf("DIR=%v", dir),
		fmt.Sprintf("BRANCH=%v", branch),
		fmt.Sprintf("PROJECT=%v", proj.Name),
		fmt.Sprintf("GO_VERSION=%v", goVersion),
		// We are hardcoding toolchain everywhere for now, until we have deps that require higher version.
		// This makes it simpler to maintain dependencies across old versions, forks and tools (e.g. code gen).
		// This follows what e.g. Prometheus is doing https://github.com/prometheus/prometheus/pull/18938#issue-4676291443
		fmt.Sprintf("GOTOOLCHAIN=go1.25.0"),
	}
	if *vulnfixSyncDockerfilesFrom {
		opts = append(opts, "SYNC_DOCKERFILES_FROM=true")
	}
	// Update go version in go.mod to what toolchain is set to if it was updated by accident
	// otherwise it won't work with our toolchain.
	if _, err := runCommand(&cmdOpts{Dir: dir, Envs: opts}, "go", "mod", "edit", "-go=1.25.0"); err != nil {
		return fmt.Errorf("failed to update go version in go.mod: %v", err)
	}

	// TODO(bwplotka): Add NPM vulnfix.
	if err := runLocalBash(dir, opts, "vulnfix.sh"); err != nil {
		return err
	}

	if proj.Name != "prometheus-engine" {
		if err := fixOtelSchemaConflict(dir); err != nil {
			return err
		}
	}

	// TODO: Warn of any unstaged files at this point.

	// Commit if anything is staged.
	msg := fmt.Sprintf("google patch[deps]: fix %v vulnerabilities", branch)
	if proj.Name == "prometheus-engine" {
		msg = fmt.Sprintf("fix: fix %v vulnerabilities", branch)
	}
	// TODO(bwplotka): Port to Go, make it more reliable.
	// TODO(bwplotka): Quote otherwise it's split into separate args... port it so it works better (:
	if err := runLibFunction(dir, nil, "release-lib::idemp::git_commit_amend_match", "\""+msg+"\"", "\""+branch+"\""); err != nil {
		return err
	}

	if mustIsRemoteUpToDate(dir, branch) {
		return fmt.Errorf("nothing to push from %q to \"origin/%v\"; aborting", dir, prBranch)
	}

	// TODO(bwplotka): Add option to print more debug/open terminal with the workdir?
	if confirmf("About to FORCE git push state from %q to \"origin/%v\"; are you sure?", dir, prBranch) {
		// We are in detached state, so be explicit what to push and from where, by recreating the local prBranch.
		mustRecreateBranch(dir, prBranch)
		mustForcePush(dir, prBranch)
		mustEnsurePullRequest(dir, branch, prBranch, msg, "Updating Go and image vulnerabilities using"+wrapCode("./gmpctl.sh vulnfix"))
	} else {
		return errors.New("aborting")
	}

	if confirmf("Do you want to remove the %v worktree?", dir) {
		proj.RemoveWorkDir(cfg.Directory, dir)
	}
	return nil
}

func detectGoMinorVersion(dir string) (string, error) {
	var dockerfiles []string
	err := filepath.WalkDir(dir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "third_party" || name == "ui" || name == "vendor" || name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), "Dockerfile") {
			dockerfiles = append(dockerfiles, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(dockerfiles) == 0 {
		return "", fmt.Errorf("no Dockerfile found in %s", dir)
	}

	re := regexp.MustCompile(`(?:google-go\.pkg\.dev/golang|golang):([0-9]+\.[0-9]+)`)

	for _, df := range dockerfiles {
		content, err := os.ReadFile(df)
		if err != nil {
			continue
		}
		matches := re.FindSubmatch(content)
		if len(matches) > 1 {
			return string(matches[1]), nil
		}
	}
	return "", fmt.Errorf("could not find golang image in any Dockerfile under %s", dir)
}

func wrapCode(s string) string {
	return "\n```\n" + s + "\n```\n"
}

// It's a common occurrence that schema import goes off-sync with the go module, fix it.
func fixOtelSchemaConflict(dir string) error {
	targetVersion, err := detectSchemaVersion(dir)
	if err != nil {
		return err
	}
	if targetVersion == "" {
		return nil
	}
	return replaceOtelImports(dir, targetVersion)
}

// TODO(bwplotka): AI figured some way, but there's likely a better way to tell?
func detectSchemaVersion(dir string) (string, error) {
	tmpFile := filepath.Join(dir, "gmpctl_tmp_schema.go")
	tmpCode := `package main

import (
	"fmt"
	"go.opentelemetry.io/otel/sdk/resource"
)

func main() {
	r := resource.Default()
	fmt.Print(r.SchemaURL())
}
`
	if err := os.WriteFile(tmpFile, []byte(tmpCode), 0o644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	defer os.Remove(tmpFile)

	cmd := exec.Command("go", "run", "gmpctl_tmp_schema.go")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		// If it fails to run, it might be because otel/sdk is not in dependencies,
		// or some other issue. We log and ignore to not block the whole pipeline if it's not relevant.
		logf("Warning: failed to run temp schema detector: %v", err)
		return "", nil
	}

	schemaURL := string(out)
	if schemaURL == "" {
		logf("No schema URL detected from SDK resource")
		return "", nil
	}

	reVersion := regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+)$`)
	matches := reVersion.FindStringSubmatch(schemaURL)
	if len(matches) < 2 {
		logf("Could not parse version from schema URL: %s", schemaURL)
		return "", nil
	}
	return "v" + matches[1], nil
}

func replaceOtelImports(dir string, targetVersion string) error {
	logf("Detected target OpenTelemetry schema version: %s", targetVersion)

	reImport := regexp.MustCompile(`"go\.opentelemetry\.io/otel/semconv/(v1\.[0-9]+\.[0-9]+)"`)
	reSchemaURLUse := regexp.MustCompile(`\.SchemaURL\b`)

	if err := filepath.WalkDir(dir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "third_party" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if !reImport.Match(content) || !reSchemaURLUse.Match(content) {
			return nil
		}

		newContent := reImport.ReplaceAllFunc(content, func(match []byte) []byte {
			return []byte(fmt.Sprintf(`"go.opentelemetry.io/otel/semconv/%s"`, targetVersion))
		})

		if string(newContent) != string(content) {
			logf("Updating OTEL semconv imports to %s in %s", targetVersion, path)
			if err := os.WriteFile(path, newContent, 0o644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", path, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	mustAddAll(dir)
	return nil
}
