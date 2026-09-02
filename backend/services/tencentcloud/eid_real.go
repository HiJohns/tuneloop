package tencentcloud

import (
	"encoding/json"

	tencentcloudsdk "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tchttp "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/http"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

// E证通 API（#1807 阶段1）。
// 关键：E证通接口挂在 **faceid** 服务下（faceid.tencentcloudapi.com，
// Version 2018-03-01），非独立 eid 服务——腾讯云未发布独立 SDK 包，
// 此 client 仿 faceid 结构手写，复用 common.Client 的 TC3 签名。
// GetEidToken 必填 MerchantId（E证通商户 ID，控制台自助接入申请）。

const eidAPIVersion = "2018-03-01"

// EidClient is a minimal client for the Tencent Cloud E证通 (EID) API.
type EidClient struct {
	tencentcloudsdk.Client
}

// NewEidClient creates an EID client with TC3 signing via common.Client.
func NewEidClient(credential tencentcloudsdk.CredentialIface, region string, clientProfile *profile.ClientProfile) (*EidClient, error) {
	client := &EidClient{}
	client.Init(region).
		WithCredential(credential).
		WithProfile(clientProfile)
	return client, nil
}

// GetEidTokenRequest requests an EID verification token.
// 必填：MerchantId（E证通商户 ID）+ CompareLib（BUSINESS）+ Name + IdCard。
type GetEidTokenRequest struct {
	*tchttp.BaseRequest
	MerchantId *string `json:"MerchantId,omitnil,omitempty" name:"MerchantId"`
	CompareLib *string `json:"CompareLib,omitnil,omitempty" name:"CompareLib"`
	Name       *string `json:"Name,omitnil,omitempty" name:"Name"`
	IdCard     *string `json:"IdCard,omitnil,omitempty" name:"IdCard"`
	SubType    *string `json:"SubType,omitnil,omitempty" name:"SubType"` // 证件类型："1"=身份证
}

func NewGetEidTokenRequest() (request *GetEidTokenRequest) {
	request = &GetEidTokenRequest{
		BaseRequest: &tchttp.BaseRequest{},
	}
	// 服务名 faceid：E证通接口与慧眼共用 faceid 产品
	request.Init().WithApiInfo("faceid", eidAPIVersion, "GetEidToken")
	return
}

// GetEidTokenResponseParams holds the response payload of GetEidToken.
type GetEidTokenResponseParams struct {
	EidToken *string `json:"EidToken,omitnil,omitempty" name:"EidToken"` // 核身凭证
}

// GetEidTokenResponse wraps the response.
type GetEidTokenResponse struct {
	*tchttp.BaseResponse
	Response *GetEidTokenResponseParams `json:"Response"`
}

// ToJsonString serializes the response (required by tchttp.Response).
func (r *GetEidTokenResponse) ToJsonString() string {
	b, _ := json.Marshal(r)
	return string(b)
}

// GetEidResultRequest queries the EID verification result.
type GetEidResultRequest struct {
	*tchttp.BaseRequest
	EidToken *string `json:"EidToken,omitnil,omitempty" name:"EidToken"`
}

func NewGetEidResultRequest() (request *GetEidResultRequest) {
	request = &GetEidResultRequest{
		BaseRequest: &tchttp.BaseRequest{},
	}
	request.Init().WithApiInfo("faceid", eidAPIVersion, "GetEidResult")
	return
}

// GetEidResultResponseParams holds the response payload of GetEidResult.
// 核验结果结构待真实联调定型（Text=DetectInfoText 文本类信息；实名信息
// DesKey/UserInfo 加密返回需商户私钥）——先宽松捕获原始 JSON。
type GetEidResultResponseParams struct {
	Text       json.RawMessage `json:"Text,omitempty"`
	Similarity *float64        `json:"Similarity,omitempty"`
}

// GetEidResultResponse wraps the response.
type GetEidResultResponse struct {
	*tchttp.BaseResponse
	Response *GetEidResultResponseParams `json:"Response"`
}

// ToJsonString serializes the response (required by tchttp.Response).
func (r *GetEidResultResponse) ToJsonString() string {
	b, _ := json.Marshal(r)
	return string(b)
}

// GetEidToken calls GetEidToken and returns the EID token.
func (c *EidClient) GetEidToken(request *GetEidTokenRequest) (response *GetEidTokenResponse, err error) {
	response = &GetEidTokenResponse{}
	if err = c.Send(request, response); err != nil {
		return nil, err
	}
	return response, nil
}

// GetEidResult calls GetEidResult and returns the verification outcome.
func (c *EidClient) GetEidResult(request *GetEidResultRequest) (response *GetEidResultResponse, err error) {
	response = &GetEidResultResponse{}
	if err = c.Send(request, response); err != nil {
		return nil, err
	}
	return response, nil
}
