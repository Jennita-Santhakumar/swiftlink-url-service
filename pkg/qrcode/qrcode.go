// Package qrcode generates a QR code PNG for a short URL and returns it as
// a base64 data URI, embedded directly in the create-URL response.
package qrcode

import (
	"encoding/base64"
	"fmt"

	goqr "github.com/skip2/go-qrcode"
)

// GenerateDataURI renders content as a 256x256 PNG QR code and returns it
// as a `data:image/png;base64,...` URI.
func GenerateDataURI(content string) (string, error) {
	png, err := goqr.Encode(content, goqr.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("failed to generate QR code: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
