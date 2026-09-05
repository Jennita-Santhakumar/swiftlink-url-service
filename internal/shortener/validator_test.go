package shortener

import "testing"

func TestIsValidLongURL(t *testing.T) {
	valid := []string{
		"http://example.com",
		"https://example.com/path?query=1",
		"https://sub.example.com:8080/a/b/c",
	}
	invalid := []string{
		"",
		"not a url",
		"ftp://example.com",
		"example.com",
		"javascript:alert(1)",
	}
	for _, u := range valid {
		if !IsValidLongURL(u) {
			t.Errorf("expected %q to be valid", u)
		}
	}
	for _, u := range invalid {
		if IsValidLongURL(u) {
			t.Errorf("expected %q to be invalid", u)
		}
	}
}

func TestIsValidCustomSlug(t *testing.T) {
	valid := []string{"mylink", "spring-sale-2026", "a_b-C9", "x"}
	invalid := []string{"", "has space", "has/slash", "has?query", string(make([]byte, 65))}
	for _, s := range valid {
		if !IsValidCustomSlug(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range invalid {
		if IsValidCustomSlug(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}
