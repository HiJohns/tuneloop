package tencentcloud

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// #1807 阶段1: E证通 client 请求构造验证（无密钥可测——不发起真实 API 调用）。

func TestGetEidTokenRequestConstruction(t *testing.T) {
	req := NewGetEidTokenRequest()
	require.Equal(t, "GetEidToken", req.GetAction())
	require.Equal(t, eidAPIVersion, req.GetVersion())
}

// #2306: GetEidToken 不接受 CompareLib（那是 GetFaceIdToken 的参数）。
// 误传会在生产返回 UnknownParameter，回归测试锁定请求体。
func TestGetEidTokenRequest_NoCompareLib(t *testing.T) {
	req := NewGetEidTokenRequest()
	merchantID := "100052464139"
	name := "张三"
	idCard := "110101199001011234"
	req.MerchantId = &merchantID
	req.Name = &name
	req.IdCard = &idCard

	payload, err := json.Marshal(req)
	require.NoError(t, err)
	body := string(payload)
	require.NotContains(t, body, "CompareLib")
	require.Contains(t, body, "MerchantId")
	require.Contains(t, body, "Name")
	require.Contains(t, body, "IdCard")

	// 字段存在性断言：即使值为 nil（omitempty 不序列化），也不允许再声明 CompareLib。
	_, hasCompareLib := reflect.TypeOf(GetEidTokenRequest{}).FieldByName("CompareLib")
	require.False(t, hasCompareLib, "GetEidTokenRequest must not declare CompareLib")
}

func TestGetEidResultRequestConstruction(t *testing.T) {
	req := NewGetEidResultRequest()
	require.Equal(t, "GetEidResult", req.GetAction())
	require.Equal(t, eidAPIVersion, req.GetVersion())
}

func TestEidProvider_GetToken_EmptyCredentialFailsAtSend(t *testing.T) {
	// 空密钥：client 可构造，但真实调用必然失败（无凭据签名）。
	p := NewEidProvider(Config{SecretID: "", SecretKey: "", Region: "ap-guangzhou"})
	token, err := p.GetToken("张三", "110101199001011234")
	require.Error(t, err, "empty credentials must not yield a token")
	require.Equal(t, "", token)
}
