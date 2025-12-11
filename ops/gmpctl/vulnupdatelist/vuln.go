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
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Vuln represents the top-level structure of a single vulnerability report item.
type Vuln struct {
	OSV     *OSV     `json:"osv"`
	Finding *Finding `json:"finding"`
}

// OSV contains the details of the Open Source Vulnerability.
type OSV struct {
	ID       string     `json:"id"`
	Aliases  []string   `json:"aliases"`
	Summary  string     `json:"summary"`
	Details  string     `json:"details"`
	Affected []Affected `json:"affected"`
}

func (o OSV) CVEs() string {
	return strings.Join(append([]string{o.ID}, o.Aliases...), ", ")
}

// Affected describes a package that is affected by the vulnerability.
type Affected struct {
	Package Package `json:"package"`
	Ranges  []Range `json:"ranges"`
}

// Package holds the name and ecosystem of the vulnerable package.
type Package struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// Range specifies the version ranges affected by the vulnerability.
type Range struct {
	Type   string  `json:"type"`
	Events []Event `json:"events"`
}

// Event marks the introduction or fixing of a vulnerability in the version history.
type Event struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

type Finding struct {
	OSVID        string `json:"osv"`
	FixedVersion string `json:"fixed_version"`
	Trace        []FindingTrace
}

type FindingTrace struct {
	Module  string `json:"module"`
	Version string `json:"version"`
}

// compileUpdateList decodes the JSON stream from govulncheck and extracts
// a list of modules that need to be updated to a fixed version.
func compileUpdateList(jsonData io.Reader, onlyFixed bool) ([]UpdateList, error) {
	updates := make(map[string]UpdateList)
	osvs := make(map[string]*OSV)
	decoder := json.NewDecoder(jsonData)

	for {
		var v Vuln
		if err := decoder.Decode(&v); err == io.EOF {
			break // End of JSON stream
		} else if err != nil {
			// It might not be a JSON error, could be other text in the stream.
			// We continue to try and find valid JSON objects.
			continue
		}

		if v.OSV == nil && v.Finding == nil {
			continue
		}
		if v.OSV != nil {
			osvs[v.OSV.ID] = v.OSV
			continue
		}

		// Parse finding.
		// We assume OSVs are printed first.
		osv := osvs[v.Finding.OSVID]
		cve := CVE{}
		allCVEs := v.Finding.OSVID
		if osv != nil {
			cve = getCVEDetails(*nvdAPIKey, *osv)
			allCVEs = osv.CVEs()
		} else {
			slog.Error("Malformed govulncheck input; a finding without a OSV entry; assuming unkown severity.", "finding.osv", v.Finding.OSVID)
			cve.ID = v.Finding.OSVID // Fallback to GO ID
			cve.Severity = "UNKNOWN"
		}
		if len(v.Finding.Trace) == 0 {
			slog.Error("Malformed govulncheck input; a finding with empty trace; ignoring.", "finding.osv", v.Finding.OSVID)
			continue
		}

		module := v.Finding.Trace[0].Module

		var fixVersion *semver.Version
		if v.Finding.FixedVersion != "" {
			var err error
			fixVersion, err = semver.NewVersion(v.Finding.FixedVersion)
			if err != nil {
				slog.Warn("Found Go vulnerability with a fix that is not a correct semver version; assuming no fix version", "mod", module, "osv", cve, "fixedVersion", v.Finding.FixedVersion, "err", err)
			}
		}

		if onlyFixed && fixVersion == nil {
			slog.Warn("IMPORTANT: Found Go vulnerability without a fixed version. Ignoring this module, given the -only-fixed flag...", "mod", module, "osv", cve)
			continue
		}

		up, ok := updates[module]
		if !ok {
			slog.Info("Found Go vulnerability with a fix; queuing...", "mod", module, "CVEs", allCVEs)
			updates[module] = UpdateList{
				CVE:          cve,
				Module:       module,
				FixedVersion: fixVersion,
				Version:      v.Finding.Trace[0].Version,
			}
			continue
		}

		// Check if there are more CVE IDs corresponding to the vulnerability, which can give more context.
		slog.Debug("Found Go vulnerability with a fix, the module was already queued; resolving version...", "mod", module, "CVEs", allCVEs)
		up.AdditionalCVEs++
		if fixVersion != nil {
			if up.FixedVersion == nil || fixVersion.GreaterThan(up.FixedVersion) {
				up.FixedVersion = fixVersion
			}
		}
		if !cve.LessThan(up.CVE) {
			up.CVE = cve
		}
		updates[module] = up
	}

	updateList := slices.Collect(maps.Values(updates))
	sort.Slice(updateList, func(i, j int) bool {
		return updateList[i].CVE.LessThan(updateList[j].CVE)
	})
	return updateList, nil
}
