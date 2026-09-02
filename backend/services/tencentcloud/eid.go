package tencentcloud

import (
	"encoding/json"
	"fmt"

	tencentcloudsdk "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

// EidProvider implements FaceVerifyProvider using Tencent Cloud E证通 (EID) API.
// 与 FaceID（慧眼）的差异：E证通走「跳转腾讯官方 e证通小程序」核身，
// 服务端同样只负责签发凭证（GetEidToken）与查询结果（GetEidResult）。
// #1807 阶段1：FACE_VERIFY_PROVIDER=eid 时启用。
type EidProvider struct {
	secretID     string
	secretKey    string
	region       string
	merchantID   string
}

func NewEidProvider(cfg Config) *EidProvider {
	return &EidProvider{
		secretID:   cfg.SecretID,
		secretKey:  cfg.SecretKey,
		region:     cfg.Region,
		merchantID: cfg.EIDMerchantID,
	}
}

// GetToken requests an E证通 verification token (EidToken)。
// 必填 MerchantId（E证通商户 ID）+ CompareLib=BUSINESS + Name + IdCard。
func (p *EidProvider) GetToken(name, idCard string) (string, error) {
	if p.merchantID == "" {
		return "", fmt.Errorf("EID merchant id not configured (EID_MERCHANT_ID)")
	}
	credential := tencentcloudsdk.NewCredential(p.secretID, p.secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "faceid.tencentcloudapi.com"

	client, err := NewEidClient(credential, p.region, cpf)
	if err != nil {
		return "", fmt.Errorf("create eid client: %w", err)
	}

	compareLib := "BUSINESS"
	req := NewGetEidTokenRequest()
	req.MerchantId = &p.merchantID
	req.CompareLib = &compareLib
	req.Name = &name
	req.IdCard = &idCard

	resp, err := client.GetEidToken(req)
	if err != nil {
		return "", fmt.Errorf("GetEidToken: %w", err)
	}
	if resp.Response == nil || resp.Response.EidToken == nil || *resp.Response.EidToken == "" {
		return "", fmt.Errorf("GetEidToken: empty response")
	}
	return *resp.Response.EidToken, nil
}

// GetResult queries the E证通 verification result.
// 核验结果解析待真实联调定型：当前宽松探测 Text（DetectInfoText）中的
// ErrCode/Text 字段；结构确认后收紧（#1807 阶段1 联调中）。
func (p *EidProvider) GetResult(eidToken string) (bool, float64, error) {
	credential := tencentcloudsdk.NewCredential(p.secretID, p.secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "faceid.tencentcloudapi.com"

	client, err := NewEidClient(credential, p.region, cpf)
	if err != nil {
		return false, 0, fmt.Errorf("create eid client: %w", err)
	}

	req := NewGetEidResultRequest()
	req.EidToken = &eidToken

	resp, err := client.GetEidResult(req)
	if err != nil {
		return false, 0, fmt.Errorf("GetEidResult: %w", err)
	}
	if resp.Response == nil {
		return false, 0, fmt.Errorf("GetEidResult: empty response")
	}

	// TODO(#1807 联调): 依据真实响应结构收紧判定。
	passed := false
	similarity := float64(0)
	if resp.Response.Similarity != nil {
		similarity = *resp.Response.Similarity
	}
	if len(resp.Response.Text) > 0 {
		var di struct {
			ErrCode *int64  `json:"ErrCode"`
			Text    *string `json:"Text"`
		}
		if err := json.Unmarshal(resp.Response.Text, &di); err == nil {
			if di.ErrCode != nil && *di.ErrCode == 0 && di.Text != nil && *di.Text == "验证通过" {
				passed = true
			}
		}
	}
	return passed, similarity, nil
}
