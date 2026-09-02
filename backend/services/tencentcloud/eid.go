package tencentcloud

import (
	"fmt"

	tencentcloudsdk "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

// EidProvider implements FaceVerifyProvider using Tencent Cloud E证通 (EID) API.
// 与 FaceID（慧眼）的差异：E证通走「跳转腾讯官方 e证通小程序」核身，
// 服务端同样只负责签发凭证（GetEidToken）与查询结果（GetEidResult）。
// #1807 阶段1：FACE_VERIFY_PROVIDER=eid 时启用。
type EidProvider struct {
	secretID  string
	secretKey string
	region    string
}

func NewEidProvider(cfg Config) *EidProvider {
	return &EidProvider{
		secretID:  cfg.SecretID,
		secretKey: cfg.SecretKey,
		region:    cfg.Region,
	}
}

// GetToken requests an E证通 verification token (EidToken, 5 分钟有效)。
// CompareLib=BUSINESS：权威库（公安库）比对，需 Name+IdCard。
func (p *EidProvider) GetToken(name, idCard string) (string, error) {
	credential := tencentcloudsdk.NewCredential(p.secretID, p.secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "eid.tencentcloudapi.com"

	client, err := NewEidClient(credential, p.region, cpf)
	if err != nil {
		return "", fmt.Errorf("create eid client: %w", err)
	}

	compareLib := "BUSINESS"
	req := NewGetEidTokenRequest()
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
func (p *EidProvider) GetResult(eidToken string) (bool, float64, error) {
	credential := tencentcloudsdk.NewCredential(p.secretID, p.secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "eid.tencentcloudapi.com"

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

	passed := resp.Response.Result != nil && *resp.Response.Result == "Success"
	similarity := float64(0)
	if resp.Response.Similarity != nil {
		similarity = *resp.Response.Similarity
	}
	return passed, similarity, nil
}
