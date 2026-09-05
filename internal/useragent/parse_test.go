package useragent

import "testing"

func TestParseKnownUserAgents(t *testing.T) {
	cases := []struct {
		ua         string
		deviceType string
		browser    string
	}{
		{
			ua:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			deviceType: "desktop",
			browser:    "Chrome",
		},
		{
			ua:         "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			deviceType: "mobile",
			browser:    "Safari",
		},
		{
			ua:         "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			deviceType: "bot",
			browser:    "Googlebot",
		},
	}

	for _, tc := range cases {
		info := Parse(tc.ua)
		if info.DeviceType != tc.deviceType {
			t.Errorf("ua=%q: expected device_type %q, got %q", tc.ua, tc.deviceType, info.DeviceType)
		}
		if info.Browser != tc.browser {
			t.Errorf("ua=%q: expected browser %q, got %q", tc.ua, tc.browser, info.Browser)
		}
	}
}
