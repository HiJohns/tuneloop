package tencentcloud

import "errors"

// ErrOCRNotConfigured is returned when Tencent Cloud credentials are missing.
var ErrOCRNotConfigured = errors.New("ID card OCR not configured: TENCENTCLOUD_SECRET_ID/KEY not set")

// IDCardInfo holds the recognized fields from a Chinese ID card.
type IDCardInfo struct {
	Name      string `json:"name"`
	Sex       string `json:"sex"`
	Nation    string `json:"nation"`
	Birth     string `json:"birth"`
	Address   string `json:"address"`
	IdNum     string `json:"id_num"`
	Authority string `json:"authority,omitempty"`
	ValidDate string `json:"valid_date,omitempty"`
}

// IDCardOCRProvider abstracts ID card OCR so the handler can be tested
// without calling the real Tencent API.
type IDCardOCRProvider interface {
	// RecognizeIDCard reads a local image file and returns recognized fields.
	// cardSide: "FRONT" (portrait) or "BACK" (national emblem).
	RecognizeIDCard(imagePath, cardSide string) (*IDCardInfo, error)
}

// NullOCRProvider is returned when Tencent Cloud credentials are not configured.
type NullOCRProvider struct{}

func (NullOCRProvider) RecognizeIDCard(_, _ string) (*IDCardInfo, error) {
	return nil, ErrOCRNotConfigured
}
