// package main implements vulnupdatelist script.
//
// Run this script to list all the vulnerable Go modules to upgrade.
// Compared to govulncheck binary, it also checks severity and groups the results
// into clear table per module to upgrade.
//
// Example use:
//
//	go run ./... \
//		-go-version=1.23.0 \
//		-only-fixed \
//		-dir=../../../prometheus \
//		-nvd-api-key="$(cat ./api.text)" | tee vuln.txt
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Masterminds/semver/v3"
)

var (
	goVersion = flag.String("go-version", "", "Go version to test vulnerabilities in (stdlib). Otherwise the `go env GOVERSION` is used")
	dir       = flag.String("dir", ".", "Where to run the script from")
	nvdAPIKey = flag.String("nvd-api-key", "", "API Key for avoiding rate-limiting on severity checks; see https://nvd.nist.gov/developers/request-an-api-key")
	onlyFixed = flag.Bool("only-fixed", false, "Don't print vulnerable modules without fixed version; note: fixed version often means sometimes that a new major version contains a fix.")
)

// UpdateList presents the minimum version to upgrade to solve all CVEs with
// a fixed version. The CVE refers to the important CVE.
// For example critical CVE 1 is fixed in v0.5.1 and low is fixed in v0.10.1.
// TODO(bwplotka): Logically, there might be cases where low contains heavy breaking changes that we can't fix easily; add option to print those too.
type UpdateList struct {
	CVE            CVE // If CVE has +<number> suffix, it means the top CVE.
	AdditionalCVEs int // Lower priority CVEs included in the "fixed" version.
	Module         string
	FixedVersion   *semver.Version
	Version        string
}

func (u UpdateList) String() string {
	fixedVersion := "???"
	if u.FixedVersion != nil {
		fixedVersion = "v" + u.FixedVersion.String()
	}
	if u.AdditionalCVEs > 0 {
		return fmt.Sprintf("%s %s@%s %s(+%d more) now@%s", u.CVE.Severity, u.Module, fixedVersion, u.CVE.ID, u.AdditionalCVEs, u.Version)
	}
	return fmt.Sprintf("%s %s@%s %s now@%s", u.CVE.Severity, u.Module, fixedVersion, u.CVE.ID, u.Version)
}

func main() {
	flag.Parse()

	workDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("Failed to resolve work dir: %v", err)
	}
	slog.Info("Running vulnupdatelist", "dir", workDir)

	if err := ensureGovulncheck(workDir); err != nil {
		log.Fatalf("Failed to ensure govulncheck is installed: %v", err)
	}

	slog.Info("Running govulncheck... ")
	vulnJSON, err := runGovulncheck(workDir, *goVersion)
	if err != nil {
		log.Fatalf("Error running govulncheck: %v", err)
	}

	if len(vulnJSON) == 0 {
		slog.Info("govulncheck produced no output; no vulnerabilities found.")
		os.Exit(0)
	}

	slog.Info("Parsing vulnerabilities and finding updates...")
	updates, err := compileUpdateList(bytes.NewReader(vulnJSON), *onlyFixed)
	if err != nil {
		log.Fatalf("Error parsing govulncheck output: %v", err)
	}
	if len(updates) == 0 {
		slog.Info("No actionable vulnerabilities with fixed versions found.")
		os.Exit(0)
	}
	for _, up := range updates {
		fmt.Println(up.String())
	}
}

// ensureGovulncheck checks if govulncheck is in the PATH, and installs it if not.
func ensureGovulncheck(dir string) error {
	_, err := exec.LookPath("govulncheck")
	if err == nil {
		slog.Info("govulncheck is already installed")
		return nil
	}

	slog.Info("govulncheck not found. Installing...")
	cmd := exec.Command("go", "install", "golang.org/x/vuln/cmd/govulncheck@latest")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run 'go install': %w", err)
	}
	slog.Info("govulncheck installed successfully.")
	return nil
}

// runGovulncheck executes `govulncheck -json ./...` and returns the output.
func runGovulncheck(dir string, goVersion string) ([]byte, error) {
	cmd := exec.Command("govulncheck", "--format=json", "./...")
	if goVersion != "" {
		cmd.Env = append(os.Environ(), "GOVERSION="+goVersion)
	}

	cmd.Dir = dir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	// govulncheck exits with a non-zero status code if vulns are found.
	// We ignore the exit code and check stderr instead. If stderr is empty,
	// it's a successful run (even with vulnerabilities).
	_ = cmd.Run()

	if stderr.Len() > 0 {
		// Only return an error if stderr is not empty, as this indicates a real execution problem.
		return nil, fmt.Errorf("govulncheck execution error: %s", stderr.String())
	}
	return out.Bytes(), nil
}
