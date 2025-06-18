package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/efficientgo/core/runutil"
	"github.com/efficientgo/core/testutil"
	"github.com/efficientgo/e2e"
	e2edb "github.com/efficientgo/e2e/db"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

const (
	promImage              = "quay.io/prometheus/prometheus"
	repoURL                = "https://github.com/prometheus/prometheus.git"
	expectedAtLeastSamples = 500
)

type compStatus struct {
	version        *semver.Version
	compatCheckErr error
}

func TestPrometheusTSDBCompatibilityTable(t *testing.T) {
	e, err := e2e.New()
	t.Cleanup(e.Close)
	testutil.Ok(t, err)

	end := semver.MustParse("v3.4.1")
	start := semver.MustParse("v2.45.3")

	// Image built from a custom code that switches to v3 index.
	snapshotDir := obtainSnapshot(t, e, "quay.io/bwplotka/prometheus:v3-index")
	ts := time.Now()
	tsFn := func() time.Time { return ts }

	tags := tagsToCheck(t, start, end)
	compatibilityStatus := make([]compStatus, 0, len(tags))

	for _, v := range tags {
		fmt.Printf("Testing compatibility of TSDB data written by %v and read by %v\n", end.Original(), v.Original())

		currPromImage := fmt.Sprintf("%v:%v", promImage, v.Original())
		p := newReadOnlyPrometheusFromSnapshot(t, e, "prom-"+v.Original(), currPromImage, snapshotDir)
		compatCheckErr := e2e.StartAndWaitReady(p)
		if compatCheckErr == nil {
			compatCheckErr = expectAtLeastSamples(t, p, tsFn, expectedAtLeastSamples)
		}
		if compatCheckErr != nil {
			fmt.Println("NOT COMPATIBLE", compatCheckErr)
		} else {
			fmt.Println("COMPATIBLE")
		}
		testutil.Ok(t, p.Kill())

		if !v.Equal(end) {
			compatibilityStatus = append(compatibilityStatus, compStatus{
				version:        v,
				compatCheckErr: compatCheckErr,
			})
		}
	}

	fmt.Println("--- Compatibility with TSDB data written by", end.Original(), "---")
	for _, s := range compatibilityStatus {
		if s.compatCheckErr != nil {
			fmt.Println("--- Prometheus", s.version.Original(), ": NOT COMPATIBLE ", s.compatCheckErr)
			continue
		}
		fmt.Println("--- Prometheus", s.version.Original(), ": COMPATIBLE")
	}
}

func expectAtLeastSamples(t testing.TB, prom e2e.Runnable, ts func() time.Time, expected float64) error {
	t.Helper()

	// Try obtaining the current sample count as a test.
	client, err := api.NewClient(api.Config{Address: "http://" + prom.Endpoint("http")})
	testutil.Ok(t, err)
	v1api := v1.NewAPI(client)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	return runutil.Retry(1*time.Second, ctx.Done(), func() (err error) {
		defer func() {
			if err != nil {
				fmt.Println("DEBUG: Query against PromQL failed", err)
			}
		}()

		v, _, err := getScrapedSamples(ts(), v1api)
		if err != nil {
			return err
		}
		if v < expected {
			return fmt.Errorf("not enough samples, expect at least %v, got %v", expected, v)
		}
		fmt.Println("Got samples:", v)
		return nil
	})
}

func obtainSnapshot(t testing.TB, e e2e.Environment, image string) (snapshotDir string) {
	t.Helper()

	prom := e2edb.NewPrometheus(e, "prom", e2edb.WithImage(image), e2edb.WithFlagOverride(map[string]string{
		"--web.enable-admin-api": "",
	}))
	testutil.Ok(t, e2e.StartAndWaitReady(prom))

	testutil.Ok(t, expectAtLeastSamples(t, prom, time.Now, expectedAtLeastSamples))

	// Snapshot the samples from the start Prometheus version.
	resp, err := http.Post("http://"+prom.Endpoint("http")+"/api/v1/admin/tsdb/snapshot", "", nil)
	testutil.Ok(t, err)
	b, err := io.ReadAll(resp.Body)
	testutil.Ok(t, err)

	if resp.StatusCode != 200 {
		t.Fatal("expected 200, got", resp.StatusCode, string(b))
	}
	parsed := struct {
		Data struct {
			Name string
		}
	}{}
	testutil.Ok(t, json.Unmarshal(b, &parsed))
	fmt.Println("Received snapshot name", parsed.Data.Name, "response", string(b))

	snapshotDir = t.TempDir()
	testutil.Ok(t, copyDir(filepath.Join(prom.Dir(), "snapshots", parsed.Data.Name), snapshotDir))
	testutil.Ok(t, prom.Kill())

	return snapshotDir
}

func newReadOnlyPrometheusFromSnapshot(t testing.TB, env e2e.Environment, name string, image string, snapshotDir string) *e2emon.Prometheus {
	prom := e2edb.NewPrometheus(env, name, e2edb.WithImage(image))
	testutil.Ok(t, prom.SetConfigEncoded([]byte(fmt.Sprintf(`global:
  external_labels:
    prometheus: %v
`, name))))
	testutil.Ok(t, copyDir(snapshotDir, prom.Dir()))
	return prom
}

func getScrapedSamples(ts time.Time, v1api v1.API) (val float64, valTs time.Time, err error) {
	result, warnings, err := v1api.Query(context.Background(), "scrape_samples_scraped{job=\"myself\"}", ts)
	if err != nil {
		return 0, time.Time{}, err
	}
	if len(warnings) > 0 {
		return 0, time.Time{}, fmt.Errorf("got some warnings %v", warnings)
	}
	m, ok := result.(model.Vector)
	if !ok {
		return 0, time.Time{}, fmt.Errorf("expected matrix, got %v", result)
	}
	if len(m) != 1 {
		return 0, time.Time{}, fmt.Errorf("expected one series, got %v", m)
	}
	val = float64(m[0].Value)
	valTs = m[0].Timestamp.Time()
	return val, valTs, nil
}

func tagsToCheck(t testing.TB, start, end *semver.Version) []*semver.Version {
	t.Helper()

	fmt.Printf("Fetching tags from %s...\n", repoURL)
	rawTags, err := fetchTags(repoURL)
	testutil.Ok(t, err)

	latestPatches, err := filterLatestPatches(rawTags)
	testutil.Ok(t, err)

	sortedVersions := make([]*semver.Version, 0, len(latestPatches))
	for _, v := range latestPatches {
		if v.LessThan(start) || v.GreaterThan(end) {
			continue
		}
		sortedVersions = append(sortedVersions, v)
	}
	sort.Sort(sort.Reverse(semver.Collection(sortedVersions)))
	return sortedVersions
}

// --- don't judge, AI generated... ---

// fetchTags executes `git ls-remote --tags` to get a list of all tags
// from the specified remote repository URL without cloning it.
func fetchTags(url string) ([]string, error) {
	// The `git ls-remote` command is a lightweight way to get refs from a remote.
	cmd := exec.Command("git", "ls-remote", "--tags", url)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git command failed: %w. Make sure 'git' is installed and in your PATH", err)
	}

	lines := strings.Split(string(output), "\n")
	tags := make([]string, 0, len(lines))

	// The output format is "<SHA>    refs/tags/<tag_name>".
	// We need to parse this to extract just the <tag_name>.
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		ref := parts[1]
		// We only want the tag names from the 'refs/tags/' prefix.
		if strings.HasPrefix(ref, "refs/tags/") {
			// Trim the prefix to get the clean tag, e.g., "v2.30.0".
			// We also ignore the annotated tag pointers `^{}`.
			tagName := strings.TrimPrefix(ref, "refs/tags/")
			if !strings.HasSuffix(tagName, "^{}") {
				tags = append(tags, tagName)
			}
		}
	}

	return tags, nil
}

// filterLatestPatches takes a slice of tag strings, parses them as semantic
// versions, and returns a map containing only the latest patch release for
// each major.minor version stream.
func filterLatestPatches(tags []string) (map[string]*semver.Version, error) {
	// This map will store the latest version found for each minor stream.
	// The key is the major.minor version (e.g., "2.30"), and the value is the full semver object.
	latestPatches := make(map[string]*semver.Version)

	for _, tag := range tags {
		// Attempt to parse the tag string into a semantic version.
		v, err := semver.NewVersion(tag)
		if err != nil {
			// Not a valid semver tag (e.g., "v0.1.0-alpha" or malformed), so we'll ignore it.
			// You could add logging here if you want to see which tags are skipped.
			// fmt.Printf("Skipping non-semver tag: %s (%v)\n", tag, err)
			continue
		}

		// We are not interested in pre-releases (e.g., -rc.1, -beta).
		if v.Prerelease() != "" {
			continue
		}

		// Create a key for the major.minor stream, e.g., "v2.40".
		minorKey := fmt.Sprintf("v%d.%d", v.Major(), v.Minor())

		// Check if we've already seen a version for this minor stream.
		existing, ok := latestPatches[minorKey]

		// If we haven't seen this minor stream before, or if the current
		// version `v` is newer than the one we have stored, update the map.
		if !ok || v.GreaterThan(existing) {
			latestPatches[minorKey] = v
		}
	}

	return latestPatches, nil
}

func copyDir(src, dest string) error {
	err := os.MkdirAll(dest, 0755)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := copyDir(sourcePath, destPath); err != nil {
				return fmt.Errorf("failed to copy dir '%s' to '%s': %w", sourcePath, destPath, err)
			}
		} else {
			if err := copyFile(sourcePath, destPath); err != nil {
				// Return the first error encountered during the copy process.
				return fmt.Errorf("failed to copy '%s' to '%s': %w", sourcePath, destPath, err)
			}
		}
	}
	return nil
}

// copyFile handles the low-level copying of a single file's contents.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}
