package tencentcloud

import (
	"fmt"

	tencentcloudsdk "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	faceid "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/faceid/v20180301"
)

// RealFaceProvider implements FaceVerifyProvider using Tencent Cloud FaceID SDK.
type RealFaceProvider struct {
	secretID  string
	secretKey string
	region    string
}

func NewRealFaceProvider(cfg Config) *RealFaceProvider {
	return &RealFaceProvider{
		secretID:  cfg.SecretID,
		secretKey: cfg.SecretKey,
		region:    cfg.Region,
	}
}

func (p *RealFaceProvider) newClient() (*faceid.Client, error) {
	credential := tencentcloudsdk.NewCredential(p.secretID, p.secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "faceid.tencentcloudapi.com"
	return faceid.NewClient(credential, p.region, cpf)
}

func (p *RealFaceProvider) GetToken(name, idCard string) (string, error) {
	client, err := p.newClient()
	if err != nil {
		return "", fmt.Errorf("create faceid client: %w", err)
	}

	compareLib := "BUSINESS"
	req := faceid.NewGetFaceIdTokenRequest()
	req.CompareLib = &compareLib
	req.Name = &name
	req.IdCard = &idCard

	resp, err := client.GetFaceIdToken(req)
	if err != nil {
		return "", fmt.Errorf("GetFaceIdToken: %w", err)
	}

	if resp.Response == nil || resp.Response.FaceIdToken == nil {
		return "", fmt.Errorf("GetFaceIdToken: empty response")
	}

	return *resp.Response.FaceIdToken, nil
}

func (p *RealFaceProvider) GetResult(faceIdToken string) (bool, float64, error) {
	client, err := p.newClient()
	if err != nil {
		return false, 0, fmt.Errorf("create faceid client: %w", err)
	}

	req := faceid.NewGetFaceIdResultRequest()
	req.FaceIdToken = &faceIdToken

	resp, err := client.GetFaceIdResult(req)
	if err != nil {
		return false, 0, fmt.Errorf("GetFaceIdResult: %w", err)
	}

	if resp.Response == nil {
		return false, 0, fmt.Errorf("GetFaceIdResult: empty response")
	}

	passed := resp.Response.Result != nil && *resp.Response.Result == "Success"
	similarity := float64(0)
	if resp.Response.Similarity != nil {
		similarity = *resp.Response.Similarity
	}

	return passed, similarity, nil
}
