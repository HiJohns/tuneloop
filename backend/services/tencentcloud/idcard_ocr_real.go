package tencentcloud

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ocr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ocr/v20181119"
)

// RealOCRProvider implements IDCardOCRProvider using Tencent Cloud OCR SDK.
type RealOCRProvider struct {
	secretID  string
	secretKey string
	region    string
}

// NewRealOCRProvider creates a real OCR provider from the shared config.
func NewRealOCRProvider(cfg Config) *RealOCRProvider {
	return &RealOCRProvider{
		secretID:  cfg.SecretID,
		secretKey: cfg.SecretKey,
		region:    cfg.Region,
	}
}

// RecognizeIDCard reads a local image file → base64 → calls Tencent Cloud
// IDCardOCR. cardSide: "FRONT" (portrait) or "BACK" (national emblem).
func (p *RealOCRProvider) RecognizeIDCard(imagePath, cardSide string) (*IDCardInfo, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	imgBase64 := base64.StdEncoding.EncodeToString(data)

	credential := common.NewCredential(p.secretID, p.secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "ocr.tencentcloudapi.com"

	client, err := ocr.NewClient(credential, p.region, cpf)
	if err != nil {
		return nil, fmt.Errorf("create OCR client: %w", err)
	}

	req := ocr.NewIDCardOCRRequest()
	req.ImageBase64 = &imgBase64
	// #1782 §1: 开启复印件/边框不完整/翻拍告警（辅助人工审核）。
	configJSON := `{"CopyWarn":true,"BorderCheckWarn":true,"ReshootWarn":true}`
	req.Config = &configJSON

	resp, err := client.IDCardOCR(req)
	if err != nil {
		return nil, fmt.Errorf("IDCardOCR API: %w", err)
	}

	info := &IDCardInfo{
		Name:      getString(resp.Response.Name),
		Sex:       getString(resp.Response.Sex),
		Nation:    getString(resp.Response.Nation),
		Birth:     getString(resp.Response.Birth),
		Address:   getString(resp.Response.Address),
		IdNum:     getString(resp.Response.IdNum),
		Authority: getString(resp.Response.Authority),
		ValidDate: getString(resp.Response.ValidDate),
		Warnings:  parseWarnInfos(resp.Response.AdvancedInfo),
	}
	return info, nil
}

// parseWarnInfos extracts WarnInfos codes from the AdvancedInfo JSON string and
// maps them to human-readable labels (#1782 §1 CopyWarn/BorderCheckWarn/ReshootWarn).
func parseWarnInfos(advancedInfo *string) []string {
	if advancedInfo == nil || *advancedInfo == "" {
		return nil
	}
	var adv struct {
		WarnInfos []struct {
			Code int `json:"Code"`
		} `json:"WarnInfos"`
	}
	if err := json.Unmarshal([]byte(*advancedInfo), &adv); err != nil {
		return nil
	}
	var warnings []string
	for _, w := range adv.WarnInfos {
		if label, ok := warnCodeLabels[w.Code]; ok {
			warnings = append(warnings, label)
		}
	}
	return warnings
}

func getString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
