package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNextRC(t *testing.T) {
	tag, err := NewTag("v0.15.3-rc.5")
	require.NoError(t, err)
	require.Equal(t, "v0.15.3-rc.6", tag.NextRC().String())

	tag, err = NewTag("v0.15.3")
	require.NoError(t, err)
	require.Equal(t, "v0.15.4-rc.0", tag.NextRC().String())

	_, err = NewTag("v0.15.3-rc.5-magicsuffix")
	require.Error(t, err)
}
