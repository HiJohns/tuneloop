package handlers

// #2581：CreateSite 的 IAM 组织名冲突分支曾返回 ASCII 英文 message
// （`organization name already exists: ...`）。前端 resolveErrorMessage 的
// 「含 CJK 的 message 原样透传」口径（#2480）判其为「无具体原因」，于是降级成
// 40900 通用文案「数据冲突，请刷新后重试」——用户读不出「商户内网点名重复」。
//
// 证据行（Accepted Plan R1/R2）：
//   R1 同商户重名 → 409 / 40900 / 中文且含网点名 / 库内仍只有 1 行 / IAM 调用 2 次
//   R2 跨商户同名 → 两次 201 / 2 个 tenant / 同名行各 1 条（唯一性 scope = 商户内）
//
// 共用 #2479 的测试脚手架（testfixtures / testutil.TestActor /
// postCreateSite2479 / seedMerchantForSiteCreate2479），只新增本单的 fake IAM。

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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const siteCreateNilUUID2581 = "00000000-0000-0000-0000-000000000000"

// fakeIAMCreateSite2581 覆盖 CreateSite 使用的 IAM 端点。
// conflictFrom > 0 时，第 conflictFrom 次（含）之后的 POST /organizations 返回 409，
// 模拟「同商户内第二次同名建组织」；其余端点与 #2479 的同形。
type fakeIAMCreateSite2581 struct {
	srv          *httptest.Server
	mu           sync.Mutex
	orgCalls     int
	conflictFrom int
}

func newFakeIAMCreateSite2581(t *testing.T, conflictFrom int) *fakeIAMCreateSite2581 {
	t.Helper()
	f := &fakeIAMCreateSite2581{conflictFrom: conflictFrom}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token-2581", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organizations"):
			f.mu.Lock()
			f.orgCalls++
			calls, conflictFrom := f.orgCalls, f.conflictFrom
			f.mu.Unlock()
			if conflictFrom > 0 && calls >= conflictFrom {
				// 与真实 IAM 同形态：409 + 组织名冲突正文。
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":"organization name already exists"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"org_id": uuid.NewString(), "admin_id": siteCreateNilUUID2581,
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "role-templates"):
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "tpl-site-admin", "code": "site_admin"},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/namespaces/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "ns-2581"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000})
		}
	}))
	t.Cleanup(f.srv.Close)
	services.SetIAMInternalURLForTesting(f.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return f
}

func (f *fakeIAMCreateSite2581) orgCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.orgCalls
}

// hasCJK2581 判定 message 是否含 CJK（前端「含 CJK 原样透传」口径的机器判据）。
func hasCJK2581(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// R1：同商户第二次同名 → 409/40900 + 中文具体原因；本地不落第二个网点。
func TestCreateSiteDuplicateName2581(t *testing.T) {
	testfixtures.SetupTestDB(t)
	db := testfixtures.GetTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	iam := newFakeIAMCreateSite2581(t, 2) // 第 2 次 POST /organizations → 409
	captureStdLog2479(t)

	firstStatus, firstResp := postCreateSite2479(t, merchantOrg, tenantID, `{"name": "海淀店"}`)
	require.Equal(t, http.StatusCreated, firstStatus, "首次创建应成功; body=%v", firstResp)

	dupStatus, dupResp := postCreateSite2479(t, merchantOrg, tenantID, `{"name": "海淀店"}`)
	require.Equal(t, http.StatusConflict, dupStatus, "同商户重名必须 409; body=%v", dupResp)

	dupCode := 0
	if v, ok := dupResp["code"].(float64); ok {
		dupCode = int(v)
	}
	dupMsg, _ := dupResp["message"].(string)
	require.Equal(t, 40900, dupCode)
	require.True(t, hasCJK2581(dupMsg), "message 必须含中文具体原因，实际=%q", dupMsg)
	require.Contains(t, dupMsg, "海淀店", "message 必须点名冲突网点名，实际=%q", dupMsg)
	require.NotContains(t, dupMsg, "already exists", "不得再泄漏 ASCII 英文文案，实际=%q", dupMsg)

	var sites int64
	require.NoError(t, db.Model(&models.Site{}).
		Where("tenant_id = ? AND name = ?", merchantOrg, "海淀店").
		Count(&sites).Error)
	require.Equal(t, int64(1), sites, "409 分支不得在本地落第二个网点")

	orgCalls := iam.orgCallCount()
	require.Equal(t, 2, orgCalls)

	msgCJK := 0
	if hasCJK2581(dupMsg) {
		msgCJK = 1
	}
	msgHasName := 0
	if strings.Contains(dupMsg, "海淀店") {
		msgHasName = 1
	}
	t.Logf("DUP_STATUS=%d", dupStatus)
	t.Logf("DUP_CODE=%d", dupCode)
	t.Logf("DUP_MSG_CJK=%d", msgCJK)
	t.Logf("DUP_MSG_HAS_NAME=%d", msgHasName)
	t.Logf("DUP_SITES_COUNT=%d", sites)
	t.Logf("DUP_ORG_CALLS=%d", orgCalls)
}

// R2：跨商户同名仍允许（唯一性 scope = 商户内，防 #2477 预检过宽）。
func TestCreateSiteCrossMerchantSameName2581(t *testing.T) {
	testfixtures.SetupTestDB(t)
	db := testfixtures.GetTestDB(t)
	orgA, tenantA := seedMerchantForSiteCreate2479(t)
	orgB, tenantB := seedMerchantForSiteCreate2479(t)
	iam := newFakeIAMCreateSite2581(t, 0) // 永不冲突
	captureStdLog2479(t)

	firstStatus, firstResp := postCreateSite2479(t, orgA, tenantA, `{"name": "海淀店"}`)
	require.Equal(t, http.StatusCreated, firstStatus, "商户 A 建同名网点应成功; body=%v", firstResp)

	secondStatus, secondResp := postCreateSite2479(t, orgB, tenantB, `{"name": "海淀店"}`)
	require.Equal(t, http.StatusCreated, secondStatus, "跨商户同名必须允许; body=%v", secondResp)

	var tenants int64
	require.NoError(t, db.Model(&models.Site{}).
		Where("name = ?", "海淀店").
		Distinct("tenant_id").
		Count(&tenants).Error)
	require.Equal(t, int64(2), tenants)

	var sameName int64
	require.NoError(t, db.Model(&models.Site{}).
		Where("name = ?", "海淀店").
		Count(&sameName).Error)
	require.Equal(t, int64(2), sameName)

	require.Equal(t, 2, iam.orgCallCount())

	crossSameName := 0
	if sameName == 2 && tenants == 2 {
		crossSameName = 1
	}
	t.Logf("CROSS_FIRST=%d", firstStatus)
	t.Logf("CROSS_SECOND=%d", secondStatus)
	t.Logf("CROSS_TENANTS=%d", tenants)
	t.Logf("CROSS_SAME_NAME=%d", crossSameName)
}
