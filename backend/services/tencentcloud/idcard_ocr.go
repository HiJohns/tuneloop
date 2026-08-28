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
	// Warnings (#1782 §1): CopyWarn/BorderCheckWarn/ReshootWarn 告警——辅助人工审核。
	Warnings []string `json:"warnings,omitempty"`
}

// warnCodeLabels maps Tencent IDCardOCR WarnInfos codes to human-readable labels.
// Codes reference: http://*.tencentcloudapi.com IDCardOCR AdvancedInfo → WarnInfos.
var warnCodeLabels = map[int]string{
	-9101: "身份证边框不完整",
	-9102: "身份证复印件",
	-9103: "身份证翻拍",
	-9105: "身份证框内遮挡",
	-9107: "身份证反光",
	-9108: "身份证复印件",
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
