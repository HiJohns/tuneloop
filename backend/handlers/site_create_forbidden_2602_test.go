package handlers

// #2602：CreateSite 调用 IAM 建组织被拒（403/401）时，曾把 IAM 英文原文塞进
// HTTP 500 + code 50000：
//
//	{"code":50000,"message":"Failed to create sub-organization in IAM: permission denied: {\"error\":\"insufficient permissions\"}"}
//
// 前端 resolveErrorMessage 的 5xx 规则（#2497）不直出英文，于是落到兜底
// 「服务器开小差了，请稍后再试」——用户无法得知这是权限问题（预生产实测 2026-11-27，
// 见 tuneloop#2602 / beaconiam#515）。
//
// 本用例锁定修复后的口径：
//   1) HTTP 403 + code 40300；
//   2) message 含中文、不得出现 IAM 英文原文（insufficient permissions / permission denied）；
//   3) 权限拒绝不得在本地落网点行（不产生半态）；
//   4) IAM 仍是被调用方（org_calls=1），说明不是本地预检短路。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/stretchr/testify/require"
)

type fakeIAMCreateSiteForbidden2602 struct {
	srv      *httptest.Server
	mu       sync.Mutex
	orgCalls int
}

func newFakeIAMCreateSiteForbidden2602(t *testing.T) *fakeIAMCreateSiteForbidden2602 {
	t.Helper()
	f := &fakeIAMCreateSiteForbidden2602{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token-2602", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organizations"):
			f.mu.Lock()
			f.orgCalls++
			f.mu.Unlock()
			// 与真实 IAM 同形态：403 + 英文机器串（不得外溢给用户）。
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient permissions"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "role-templates"):
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "tpl-site-admin", "code": "site_admin"},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/namespaces/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "ns-2602"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000})
		}
	}))
	t.Cleanup(f.srv.Close)
	services.SetIAMInternalURLForTesting(f.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return f
}

func (f *fakeIAMCreateSiteForbidden2602) orgCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.orgCalls
}

func hasCJK2602(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func TestCreateSiteIAMForbidden2602(t *testing.T) {
	testfixtures.SetupTestDB(t)
	db := testfixtures.GetTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	iam := newFakeIAMCreateSiteForbidden2602(t)
	captureStdLog2479(t)

	status, resp := postCreateSite2479(t, merchantOrg, tenantID, `{"name": "海淀店"}`)

	require.Equal(t, http.StatusForbidden, status, "IAM 权限不足必须映射 403 而非 5xx; body=%v", resp)

	code := 0
	if v, ok := resp["code"].(float64); ok {
		code = int(v)
	}
	msg, _ := resp["message"].(string)

	require.Equal(t, 40300, code, "code 应为 40300（权限不足）")
	require.True(t, hasCJK2602(msg), "message 必须含中文可行动原因，实际=%q", msg)
	require.NotContains(t, msg, "insufficient permissions", "不得外溢 IAM 英文原文，实际=%q", msg)
	require.NotContains(t, msg, "permission denied", "不得外溢 IAM 英文原文，实际=%q", msg)
	require.NotContains(t, msg, "Failed to create sub-organization", "不得外溢内部包装串，实际=%q", msg)

	var sites int64
	require.NoError(t, db.Model(&models.Site{}).
		Where("tenant_id = ? AND name = ?", merchantOrg, "海淀店").
		Count(&sites).Error)
	require.Equal(t, int64(0), sites, "权限拒绝不得在本地落网点行")

	require.Equal(t, 1, iam.orgCallCount(), "IAM 仍应被调用一次（非本地预检短路）")

	t.Logf("FORBIDDEN_STATUS=%d", status)
	t.Logf("FORBIDDEN_CODE=%d", code)
	t.Logf("FORBIDDEN_MSG_CJK=%d", boolToInt2602(hasCJK2602(msg)))
	t.Logf("FORBIDDEN_SITES_COUNT=%d", sites)
	t.Logf("FORBIDDEN_ORG_CALLS=%d", iam.orgCallCount())
}

func boolToInt2602(b bool) int {
	if b {
		return 1
	}
	return 0
}
