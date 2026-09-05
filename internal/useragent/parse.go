// Package useragent extracts device type, browser, and OS from a raw
// User-Agent header using github.com/mileusna/useragent (pure Go, no
// external service).
package useragent

import "github.com/mileusna/useragent"

type Info struct {
	DeviceType string // mobile, tablet, desktop, bot
	Browser    string
	OS         string
}

func Parse(rawUA string) Info {
	ua := useragent.Parse(rawUA)

	deviceType := "desktop"
	switch {
	case ua.Bot:
		deviceType = "bot"
	case ua.Mobile:
		deviceType = "mobile"
	case ua.Tablet:
		deviceType = "tablet"
	}

	return Info{
		DeviceType: deviceType,
		Browser:    ua.Name,
		OS:         ua.OS,
	}
}
