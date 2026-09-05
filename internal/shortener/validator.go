package shortener

import (
	"net/url"
	"regexp"
)

var customSlugPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// IsValidLongURL checks the long URL is a well-formed http(s) URL, matching
// the PRD's `valid_url` CHECK constraint (`^https?://`) plus basic parse
// validation so obviously malformed URLs are rejected before ever reaching
// Postgres.
func IsValidLongURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

// IsValidCustomSlug checks a user-supplied slug is URL-safe and fits the
// widened short_code column (see migration 000002's Design Decision note).
func IsValidCustomSlug(slug string) bool {
	return customSlugPattern.MatchString(slug)
}
