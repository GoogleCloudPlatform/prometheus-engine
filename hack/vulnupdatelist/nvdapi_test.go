package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCVEDetails(t *testing.T) {
	t.Skip("depends on NVDE API")

	c := getCVEDetails("", OSV{
		ID:      "GO-2021-0065",
		Aliases: []string{"GHSA-jmrx-5g74-6v2f", "CVE-2019-11250"},
	})
	require.Equal(t, "CVE-2019-11250", c.ID)
	require.Equal(t, "MEDIUM", c.Severity)
}
