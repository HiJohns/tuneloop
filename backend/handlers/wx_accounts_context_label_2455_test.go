package handlers

// #2455 Phase B R6：身份上下文必须带商户名 + 网点名，且跨商户同名网点可区分。
//
// 场景（预生产真实形态）：两家不同商户各有一个叫「卡丹萨乐器」的网点，
// 旧实现只把 IAM 的 org_name/label 透传（或前端只显示 sites.name），
// 两处上下文都显示「卡丹萨乐器」，用户无法辨认。
//
// 证据 token：LABEL_A=cadenza · 卡丹萨乐器 / LABEL_B=卡丹萨 · 卡丹萨乐器 /
//            ORG_NAME_B=卡丹萨西铁营 / DISTINCT=1 / LEGACY_LABEL=… / CUSTOMER_LABEL=顾客

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWxAccounts_ContextLabelDisambiguatesSameNamedSites_2455(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	// 两家商户（tenant）+ 两个跨商户同名网点（org 不同、sites.name 相同）。
	tenantCadenza, tenantKadansa := uuid.NewString(), uuid.NewString()
	orgHaidian, orgXitieying := uuid.NewString(), uuid.NewString()

	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), Code: testfixtures.UniqueCode("L2455"), TenantID: tenantCadenza,
		OrgID: uuid.NewString(), Name: "cadenza", Address: "x", ContactName: "c",
		ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), Code: testfixtures.UniqueCode("L2455"), TenantID: tenantKadansa,
		OrgID: uuid.NewString(), Name: "卡丹萨", Address: "x", ContactName: "c",
		ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	for _, s := range []models.Site{
		{ID: uuid.NewString(), TenantID: tenantCadenza, OrgID: orgHaidian, Name: "卡丹萨乐器", Status: "active"},
		{ID: uuid.NewString(), TenantID: tenantKadansa, OrgID: orgXitieying, Name: "卡丹萨乐器", Status: "active"},
	} {
		require.NoError(t, db.Create(&s).Error)
	}

	staffID := uuid.NewString()
	accounts := []map[string]interface{}{
		{
			"user_id": staffID, "name": "员工小王", "nickname": "小王", "role": "STAFF",
			"org_id": orgHaidian, "tenant_id": tenantCadenza,
			"contexts": []map[string]interface{}{
				{
					"type": "org", "org_id": orgHaidian, "org_name": "海淀店",
					"role": "site_admin", "label": "海淀店",
				},
				{
					"type": "org", "org_id": orgXitieying, "org_name": "卡丹萨西铁营",
					"role": "site_member", "label": "卡丹萨西铁营",
				},
				{"type": "customer", "label": "顾客"},
			},
		},
		{
			// legacy IAM response（无 contexts）：只能靠 account 级 label 区分。
			"user_id": uuid.NewString(), "name": "员工小李", "nickname": "小李", "role": "STAFF",
			"org_id": orgXitieying, "tenant_id": tenantKadansa,
		},
		{
			"user_id": uuid.NewString(), "name": "顾客小张", "nickname": "小张", "role": "USER",
			"org_id": "", "tenant_id": "",
		},
	}

	srv := newWxAccountsMockServer(t, accounts)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	router := gin.New()
	router.GET("/api/auth/wx-accounts", NewAuthHandler(db).WxAccounts)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/wx-accounts?code=label-2455", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Accounts []map[string]interface{} `json:"accounts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 20000, resp.Code)
	require.Len(t, resp.Data.Accounts, 3)

	contexts, ok := resp.Data.Accounts[0]["contexts"].([]interface{})
	require.True(t, ok, "contexts 必须是数组: %v", resp.Data.Accounts[0]["contexts"])
	require.Len(t, contexts, 3)

	ctxA := contexts[0].(map[string]interface{})
	ctxB := contexts[1].(map[string]interface{})
	ctxCustomer := contexts[2].(map[string]interface{})

	labelA, _ := ctxA["label"].(string)
	labelB, _ := ctxB["label"].(string)
	require.Equal(t, "cadenza · 卡丹萨乐器", labelA, "LABEL_A")
	require.Equal(t, "卡丹萨 · 卡丹萨乐器", labelB, "LABEL_B")
	require.NotEqual(t, labelA, labelB, "DISTINCT：跨商户同名网点的 label 必须互不相同")

	orgNameB, _ := ctxB["org_name"].(string)
	require.Equal(t, "卡丹萨西铁营", orgNameB, "ORG_NAME_B：org_name 仍取 IAM 组织名")
	require.Equal(t, "卡丹萨乐器", ctxB["site_name"], "context 必须显式返回 site_name")
	require.Equal(t, "卡丹萨", ctxB["merchant_name"], "context 必须显式返回 merchant_name")

	customerLabel, _ := ctxCustomer["label"].(string)
	require.Equal(t, "顾客", customerLabel, "CUSTOMER_LABEL：顾客上下文保持 IAM/默认 label")

	// legacy 分支：account 级 label 同样由服务端组合（前端不再自拼）。
	legacyLabel, _ := resp.Data.Accounts[1]["label"].(string)
	require.Equal(t, "卡丹萨 · 卡丹萨乐器", legacyLabel, "LEGACY_LABEL")
	require.Equal(t, "卡丹萨乐器", resp.Data.Accounts[1]["site_name"])
	require.Equal(t, "卡丹萨", resp.Data.Accounts[1]["merchant_name"])

	distinct := 0
	if labelA != labelB {
		distinct = 1
	}

	t.Logf("LABEL_A=%s", labelA)
	t.Logf("LABEL_B=%s", labelB)
	t.Logf("ORG_NAME_B=%s", orgNameB)
	t.Logf("DISTINCT=%d", distinct)
	t.Logf("LEGACY_LABEL=%s", legacyLabel)
	t.Logf("CUSTOMER_LABEL=%s", customerLabel)
}

// TestWxAccounts_ContextLabelNoCrossMerchantBorrow_2455 覆盖 #2455 Round 1 审计 Bug 4：
// 某个 context 的 sites 行缺失（未同步 / 软删）时，**不得**用 account 级 tenant 去猜商户名
// —— 那个 tenant 只属于同一账号的另一个 context，猜出来的就是别家商户的名字（甚至让两条
// label 相同）。D4 规定：解析不到就回退该 context 自己的 IAM org_name。
func TestWxAccounts_ContextLabelNoCrossMerchantBorrow_2455(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantCadenza := uuid.NewString()
	orgHaidian := uuid.NewString()
	orgUnsynced := uuid.NewString() // IAM 有组织，本地 sites 行缺失

	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), Code: testfixtures.UniqueCode("L2455"), TenantID: tenantCadenza,
		OrgID: uuid.NewString(), Name: "cadenza", Address: "x", ContactName: "c",
		ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	// 只有第一个 context 有本地镜像行；第二个 context 故意缺失。
	require.NoError(t, db.Create(&models.Site{
		ID: uuid.NewString(), TenantID: tenantCadenza, OrgID: orgHaidian,
		Name: "卡丹萨乐器", Status: "active",
	}).Error)

	accounts := []map[string]interface{}{
		{
			"user_id": uuid.NewString(), "name": "员工小王", "nickname": "小王", "role": "STAFF",
			// account 级 tenant 属于第一个 context —— 不能被第二个 context 借用。
			"org_id": orgHaidian, "tenant_id": tenantCadenza,
			"contexts": []map[string]interface{}{
				{"type": "org", "org_id": orgHaidian, "org_name": "海淀店", "role": "site_admin"},
				{"type": "org", "org_id": orgUnsynced, "org_name": "未同步网点", "role": "site_member"},
			},
		},
	}

	srv := newWxAccountsMockServer(t, accounts)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	router := gin.New()
	router.GET("/api/auth/wx-accounts", NewAuthHandler(db).WxAccounts)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/wx-accounts?code=label-2455-borrow", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Accounts []map[string]interface{} `json:"accounts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 20000, resp.Code)
	require.Len(t, resp.Data.Accounts, 1)

	contexts, ok := resp.Data.Accounts[0]["contexts"].([]interface{})
	require.True(t, ok, "contexts 必须是数组: %v", resp.Data.Accounts[0]["contexts"])
	require.Len(t, contexts, 2)

	ctxSynced := contexts[0].(map[string]interface{})
	ctxUnsynced := contexts[1].(map[string]interface{})

	// 有 sites 行的 context：仍按 org → sites → tenant 解析。
	require.Equal(t, "cadenza", ctxSynced["merchant_name"], "SYNCED_MERCHANT")
	require.Equal(t, "卡丹萨乐器", ctxSynced["site_name"], "SYNCED_SITE")
	require.Equal(t, "cadenza · 卡丹萨乐器", ctxSynced["label"], "SYNCED_LABEL")

	// 缺失 sites 行的 context：不得借用另一上下文的商户名。
	borrowed, _ := ctxUnsynced["merchant_name"].(string)
	require.Empty(t, borrowed, "MERCHANT_BORROWED=0：缺失 sites 行时不得借用 account 级 tenant 的商户名")
	require.Empty(t, ctxUnsynced["site_name"], "缺失 sites 行时 site_name 应为空")
	require.Equal(t, "未同步网点", ctxUnsynced["label"], "FALLBACK_ORG_NAME：回退该 context 自己的 IAM org_name")

	t.Logf("MERCHANT_BORROWED=%d", map[bool]int{true: 1, false: 0}[borrowed != ""])
	t.Logf("FALLBACK_LABEL_OK=%d", map[bool]int{true: 1, false: 0}[ctxUnsynced["label"] == "未同步网点"])
	t.Logf("SYNCED_LABEL=%s", ctxSynced["label"])
}
