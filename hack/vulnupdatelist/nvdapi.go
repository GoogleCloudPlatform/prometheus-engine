package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NVDResponse is the top-level object for the NVD CVE API.
type NVDResponse struct {
	Vulnerabilities []struct {
		CVE struct {
			ID      string `json:"id"`
			Metrics struct {
				CVSSMetricV31 []struct {
					CVSSData struct {
						BaseSeverity string `json:"baseSeverity"`
					} `json:"cvssData"`
				} `json:"cvssMetricV31"`
			} `json:"metrics"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

// getCVSSSeverity fetches vulnerability details from the NVD API and returns the CVSS V3 severity.
func getCVSSSeverity(apiKey string, cveID string) (string, error) {
	// https://nvd.nist.gov/developers/vulnerabilities
	apiURL := fmt.Sprintf("https://services.nvd.nist.gov/rest/json/cves/2.0?cveId=%s", cveID)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("apiKey", apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request to NVD API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("NVD API returned non-200 status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var nvdResponse NVDResponse
	if err := json.Unmarshal(body, &nvdResponse); err != nil {
		return "", fmt.Errorf("failed to parse JSON from NVD API: %w", err)
	}

	if len(nvdResponse.Vulnerabilities) > 0 {
		metrics := nvdResponse.Vulnerabilities[0].CVE.Metrics
		if len(metrics.CVSSMetricV31) > 0 {
			return metrics.CVSSMetricV31[0].CVSSData.BaseSeverity, nil
		}
	}

	return "UNKNOWN", nil
}

type CVE struct {
	ID       string
	Severity string
}

func (a CVE) LessThan(b CVE) bool {
	order := map[string]int{
		"CRITICAL": 0,
		"HIGH":     1,
		"MEDIUM":   2,
		"UNKNOWN":  3,
		"":         3,
	}
	return order[a.Severity] < order[b.Severity]
}

func getCVEDetails(apiKey string, osv OSV) CVE {
	cveID := CVE{Severity: "UNKNOWN"}
	// Assume ID is GO-... ID and use it as a fallback.
	for _, a := range osv.Aliases {
		if strings.HasPrefix(a, "CVE") {
			cveID.ID = a
			break
		}
	}
	if cveID.ID == "" {
		return CVE{ID: osv.ID, Severity: "UNKNOWN"} // Fallback to GO ID.
	}
	sev, err := getCVSSSeverity(apiKey, cveID.ID)
	if err != nil {
		slog.Error("failed to find severity", "cve", cveID, "err", err)
	} else {
		cveID.Severity = sev
	}
	return cveID
}
