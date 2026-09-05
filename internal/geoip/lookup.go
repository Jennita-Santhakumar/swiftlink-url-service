// Package geoip resolves an IPv4 address to a country code using a fully
// offline, embedded dataset — no paid API, no account/license key required
// (unlike MaxMind GeoLite2, which needs a free account + license key to
// download).
//
// Scope: country-level only. The PRD's click_events schema also has
// city/latitude/longitude columns, which stay unpopulated — precise
// city-level geolocation needs a paid or account-gated database. This is a
// documented trade-off, not a silent gap; see README's Design Decisions.
//
// Data: IP to Country Lite by DB-IP (https://db-ip.com), redistributed via
// github.com/sapics/ip-location-db under CC BY 4.0 — attribution required
// and given in the README and the analytics dashboard footer.
package geoip

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strings"
)

//go:embed data/dbip-country-ipv4.csv
var rawData []byte

type ipRange struct {
	start   uint32
	end     uint32
	country string
}

type Lookup struct {
	ranges []ipRange
}

// New parses the embedded CSV dataset into an in-memory sorted range table.
// Parsing ~357K rows happens once at process startup (a few hundred
// milliseconds) and uses ~6MB of memory.
func New() (*Lookup, error) {
	ranges := make([]ipRange, 0, 360_000)

	scanner := bufio.NewScanner(bytes.NewReader(rawData))
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 3 {
			continue
		}
		start, ok1 := ipToUint32(parts[0])
		end, ok2 := ipToUint32(parts[1])
		if !ok1 || !ok2 {
			continue
		}
		ranges = append(ranges, ipRange{start: start, end: end, country: parts[2]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse embedded geoip dataset: %w", err)
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	return &Lookup{ranges: ranges}, nil
}

func ipToUint32(s string) (uint32, bool) {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0, false
	}
	return binary.BigEndian.Uint32(ip), true
}

// Country returns the ISO 3166-1 alpha-2 country code for an IPv4 address,
// or "" if it's not IPv4 or falls outside every known range.
func (l *Lookup) Country(ipStr string) string {
	target, ok := ipToUint32(ipStr)
	if !ok {
		return ""
	}

	// Binary search for the last range whose start <= target.
	i := sort.Search(len(l.ranges), func(i int) bool { return l.ranges[i].start > target })
	if i == 0 {
		return ""
	}
	candidate := l.ranges[i-1]
	if target >= candidate.start && target <= candidate.end {
		return candidate.country
	}
	return ""
}
