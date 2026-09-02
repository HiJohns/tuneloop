package tencentcloud

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// #1807 阶段1: E证通 client 请求构造验证（无密钥可测——不发起真实 API 调用）。

func TestGetEidTokenRequestConstruction(t *testing.T) {
	req := NewGetEidTokenRequest()
	require.Equal(t, "GetEidToken", req.GetAction())
	require.Equal(t, eidAPIVersion, req.GetVersion())
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
