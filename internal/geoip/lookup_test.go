package geoip

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupKnownRanges(t *testing.T) {
	l, err := New()
	require.NoError(t, err)
	require.NotEmpty(t, l.ranges, "embedded dataset must parse into at least one range")

	// From the dataset's own first lines: 1.0.0.0-1.0.0.255 = AU, 1.0.1.0-1.0.3.255 = CN.
	require.Equal(t, "AU", l.Country("1.0.0.1"))
	require.Equal(t, "CN", l.Country("1.0.1.1"))
}

func TestLookupInvalidInput(t *testing.T) {
	l, err := New()
	require.NoError(t, err)

	require.Equal(t, "", l.Country("not-an-ip"))
	require.Equal(t, "", l.Country("::1")) // IPv6 out of scope for this dataset
}

func TestLookupRangesAreSorted(t *testing.T) {
	l, err := New()
	require.NoError(t, err)
	for i := 1; i < len(l.ranges); i++ {
		require.LessOrEqual(t, l.ranges[i-1].start, l.ranges[i].start)
	}
}
