package services

// #2546：IAM 转让所有权客户端契约（beaconiam#508）。
// 只证明 method/path/body 与状态码→类型化错误的映射；IAM 侧授权规则由
// beaconiam 自身用例覆盖（工具盲区已在计划中声明）。

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type transferMock2546 struct {
	calls  int
	method string
	path   string
	body   string
	status int // 0 == 200
	reason string
}

func setupTransferMock2546(t *testing.T, m *transferMock2546) {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		m.calls++
		m.method = r.Method
		m.path = r.URL.Path
		m.body = string(raw)
		status := m.status
		if status == 0 {
			status = http.StatusOK
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			writeJSON(w, map[string]string{"error": m.reason})
			return
		}
		writeJSON(w, map[string]interface{}{
			"org_id":            "org-2546",
			"previous_owner_id": "prev-2546",
			"new_owner_id":      "next-2546",
		})
	}))
	t.Cleanup(srv.Close)
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_NAMESPACE", "test-ns")
	t.Setenv("IAM_SECRET", "test-secret")
}

func TestTransferOwnershipClientContract_2546(t *testing.T) {
	mock := &transferMock2546{}
	setupTransferMock2546(t, mock)

	client := NewIAMClientWithCredentials("ns", "secret")
	require.NoError(t, client.TransferOwnershipWithToken("user-token", "org-2546", "iam-sub-2546"))

	assert.Equal(t, 1, mock.calls, "IAM_TRANSFER_CALLS=1")
	assert.Equal(t, http.MethodPost, mock.method, "IAM_METHOD=POST")
	assert.Equal(t, "/api/v1/organizations/org-2546/transfer-ownership", mock.path, "IAM_PATH")

	var body map[string]string
	require.NoError(t, json.Unmarshal([]byte(mock.body), &body))
	assert.Equal(t, map[string]string{"user_id": "iam-sub-2546"}, body, "IAM_BODY")

	t.Logf("IAM_TRANSFER_CALLS=%d IAM_METHOD=%s IAM_PATH=%s", mock.calls, mock.method, mock.path)
}

func TestTransferOwnershipClientErrorMapping_2546(t *testing.T) {
	cases := []struct {
		status int
		reason string
	}{
		{http.StatusForbidden, "only the current owner can transfer ownership"},
		{http.StatusNotFound, "target user is not an active member of this organization"},
		{http.StatusConflict, "organization has no active owner"},
		{http.StatusBadRequest, "user is already the owner"},
		{http.StatusInternalServerError, "failed to commit ownership transfer"},
	}

	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			mock := &transferMock2546{status: tc.status, reason: tc.reason}
			setupTransferMock2546(t, mock)

			client := NewIAMClientWithCredentials("ns", "secret")
			err := client.TransferOwnershipWithToken("user-token", "org-2546", "iam-sub-2546")
			require.Error(t, err)

			var iamErr *IAMTransferOwnershipError
			require.True(t, errors.As(err, &iamErr), "必须是类型化错误（不得只返回字符串让调用方猜）：%v", err)
			assert.Equal(t, tc.status, iamErr.StatusCode, "STATUS_CODE")
			assert.Equal(t, tc.reason, iamErr.Message, "REASON")
			assert.Equal(t, 1, mock.calls, "IAM_TRANSFER_CALLS=1（不得重试/吞错）")

			t.Logf("IAM_STATUS=%d IAM_ERROR=%s", iamErr.StatusCode, iamErr.Message)
		})
	}
}
