package bindmsg

// 走 gin.ShouldBindJSON 的**真实路径**（生产就是这条）：确认 validator 错误能被
// Message() 识别并点名字段，而不是落进 JSON 语法分支。
// 预生产第 14 项首次改包后返回「请求体格式不正确」是因为请求体被写坏（非法字节），
// 若仅缺字段（JSON 合法）必须得到「参数不合法：名称不能为空」。

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMessageGinBinding2603(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type siteCreateReq struct {
		Name       string  `json:"name" binding:"required"`
		Address    string  `json:"address"`
		Latitude   float64 `json:"latitude"`
		PostalCode string  `json:"postal_code"`
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "仅缺必填字段（JSON 合法）",
			body: `{"address":"x","postal_code":""}`,
			want: "参数不合法：名称不能为空",
		},
		{
			name: "类型不匹配",
			body: `{"name":"海淀店","latitude":"abc"}`,
			want: "参数不合法：纬度类型不正确",
		},
		{
			name: "请求体被写坏（控制字符）",
			body: "{\"name\":\"x\"\x17}",
			want: "请求体格式不正确，请检查后重试",
		},
		{
			name: "空体",
			body: ``,
			want: "参数不合法，请检查后重试",
		},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/merchant/sites", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")

		var req siteCreateReq
		err := c.ShouldBindJSON(&req)
		require.Error(t, err, tc.name)
		got := Message(err)
		require.Equal(t, tc.want, got, tc.name)
		t.Logf("CASE=%s MSG=%s", tc.name, got)
	}
}
