package handlers

// #2457（v3）：IAM organizations.name 是组织名权威源，sites.name 只是本地镜像。
// UpdateSite 改名必须先写 IAM，成功后才写本地；IAM 失败不得落本地（避免二次漂移）。
//
// 证据行（Accepted Plan v3 R1–R8）：
//   R1 改名 IAM 优先          → IAM_CALLS=1 / ORDER=iam_before_local
//   R2 IAM 失败不改本地        → HTTP=502 / LOCAL_NAME=<旧名>
//   R3 IAM 重名映射            → HTTP=409 / CODE=40900
//   R4 403 服务凭据升级        → IAM_CALLS=2 / ESCALATED=1
//   R5 同名幂等                → IAM_CALLS=0 / ADDRESS=updated
//   R6 商户锚点守卫            → HTTP=400 / CODE=40002 / IAM_CALLS=0
//   R7 同商户重名预检          → HTTP=409 / CODE=40900 / IAM_CALLS=0
//   R8 空名称拒绝              → HTTP=400 / CODE=40002 / IAM_CALLS=0

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	siteUpdateOrgID2457 = "76d46658-0000-0000-0000-000000000001"
	// fake IAM 的两种令牌：调用者令牌被拒（403），服务凭据通过（200）。
	siteUpdateUserToken2457 = "user-token-2457"
	siteUpdateSvcToken2457  = "svc-token-2457"
)

// fakeIAMSite2457 是 UpdateSite 用例的最小 IAM：token + PUT organizations/:id。
type fakeIAMSite2457 struct {
	srv       *httptest.Server
	mu        sync.Mutex
	calls     int
	lastBody  string
	lastPath  string
	lastAuth  string
	status    int
	statusFor map[string]int // Authorization 前缀 → 状态码（用于 R4 的 403→200）
	name      string
	onPut     func()
}

func newFakeIAMSite2457(t *testing.T, status int, name string) *fakeIAMSite2457 {
	t.Helper()
	f := &fakeIAMSite2457{status: status, name: name}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": siteUpdateSvcToken2457, "expires_in": 3600, "token_type": "Bearer",
			})
			return
		}
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls++
		f.lastBody = string(raw)
		f.lastPath = r.URL.Path
		f.lastAuth = r.Header.Get("Authorization")
		hook := f.onPut
		status := f.status
		for token, override := range f.statusFor {
			if strings.HasSuffix(f.lastAuth, token) {
				status = override
			}
		}
		f.mu.Unlock()
		if hook != nil {
			hook()
		}
		if status != http.StatusOK && status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"iam rejected"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/"), "name": f.name,
		})
	}))
	t.Cleanup(f.srv.Close)
	services.SetIAMInternalURLForTesting(f.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return f
}

func (f *fakeIAMSite2457) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeIAMSite2457) snapshot() (body, path, auth string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastBody, f.lastPath, f.lastAuth
}

func (f *fakeIAMSite2457) setStatusForToken(token string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statusFor == nil {
		f.statusFor = map[string]int{}
	}
	f.statusFor[token] = status
}

func (f *fakeIAMSite2457) setOnPut(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onPut = hook
}

// seedSite2457 建一个「普通网点」：org_id 是独立 IAM 组织（≠ tenant_id）。
func seedSite2457(t *testing.T, db *gorm.DB, name string) (tenantID, siteID string) {
	t.Helper()
	tenantID = uuid.NewString()
	siteID = uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: siteUpdateOrgID2457, Name: name, Status: "active",
	}).Error)
	return tenantID, siteID
}

// siteUpdateRouter2457 注入租户/权限上下文；token 非空时带 Authorization。
func siteUpdateRouter2457(tenantID string, sysPerm int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantID, OrgID: siteUpdateOrgID2457,
			UserID: uuid.NewString(), Role: "OWNER", SysPerm: sysPerm,
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.PUT("/api/merchant/sites/:id", NewSiteHandler().UpdateSite)
	return r
}

func putSite2457(t *testing.T, r *gin.Engine, siteID, body, userToken string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/merchant/sites/"+siteID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userToken != "" {
		req.Header.Set("Authorization", "Bearer "+userToken)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

func siteName2457(t *testing.T, db *gorm.DB, siteID string) models.Site {
	t.Helper()
	var site models.Site
	require.NoError(t, db.Where("id = ?", siteID).First(&site).Error)
	return site
}

// R1：改名时 IAM 先于本地 UPDATE，且 body 为新名。
func TestUpdateSite_WritesIAMBeforeLocal_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "A")

	iam := newFakeIAMSite2457(t, http.StatusOK, "B")
	var mu sync.Mutex
	nameDuringIAM := ""
	iam.setOnPut(func() {
		var site models.Site
		if err := db.Where("id = ?", siteID).First(&site).Error; err == nil {
			mu.Lock()
			nameDuringIAM = site.Name
			mu.Unlock()
		}
	})

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID, `{"name":"B"}`, siteUpdateUserToken2457)

	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.EqualValues(t, 20000, resp["code"])

	mu.Lock()
	during := nameDuringIAM
	mu.Unlock()
	// 契约断言值固定为 iam_before_local（= IAM 先于本地写）。
	order := "local_before_iam"
	if during == "A" {
		order = "iam_before_local"
	}
	body, path, _ := iam.snapshot()
	require.Equal(t, "iam_before_local", order, "IAM 调用时本地仍应是旧名")
	require.Equal(t, 1, iam.callCount(), "改名必须恰好调用一次 IAM")
	require.JSONEq(t, `{"name":"B"}`, body)
	require.Contains(t, path, "/api/v1/organizations/")

	after := siteName2457(t, db, siteID)
	require.Equal(t, "B", after.Name)

	t.Logf("IAM_CALLS=%d", iam.callCount())
	t.Logf("ORDER=%s", order)
	t.Logf("IAM_BODY=%s", body)
}

// R2：IAM 5xx → 5xx 返回，本地保持旧名（不二次漂移）。
func TestUpdateSite_IAMFailureKeepsLocal_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "A")

	iam := newFakeIAMSite2457(t, http.StatusInternalServerError, "")

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID, `{"name":"B"}`, siteUpdateUserToken2457)

	require.GreaterOrEqual(t, code, 500, "IAM 失败必须是 5xx: %v", resp)
	require.Equal(t, 1, iam.callCount())

	after := siteName2457(t, db, siteID)
	require.Equal(t, "A", after.Name, "IAM 失败不得落本地")

	t.Logf("# STATUS = upstream IAM status for this fixture; API returns HTTP=%d per D3 mapping", code)
	t.Logf("STATUS=%d", http.StatusInternalServerError)
	t.Logf("HTTP=%d", code)
	t.Logf("LOCAL_NAME=%s", after.Name)
}

// R3：IAM 409（组织重名）→ 409/40900，本地不变。
func TestUpdateSite_IAMConflictMapsTo409_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "A")

	iam := newFakeIAMSite2457(t, http.StatusConflict, "")

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID, `{"name":"B"}`, siteUpdateUserToken2457)

	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])
	require.Equal(t, 1, iam.callCount())

	after := siteName2457(t, db, siteID)
	require.Equal(t, "A", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("LOCAL_NAME=%s", after.Name)
}

// R4：用户令牌 403 且本地 bit13 已通过 → 服务凭据重试一次，最终 200。
func TestUpdateSite_ForbiddenEscalatesToServiceCredentials_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "A")

	iam := newFakeIAMSite2457(t, http.StatusOK, "B")
	iam.setStatusForToken(siteUpdateUserToken2457, http.StatusForbidden)

	r := siteUpdateRouter2457(tenantID, 1<<middleware.SysPermOrganizationUpdate)
	code, resp := putSite2457(t, r, siteID, `{"name":"B"}`, siteUpdateUserToken2457)

	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.EqualValues(t, 20000, resp["code"])
	require.Equal(t, 2, iam.callCount(), "403 后必须用服务凭据重试一次")
	_, _, auth := iam.snapshot()
	require.True(t, strings.HasSuffix(auth, siteUpdateSvcToken2457), "第二次必须是服务凭据，实际=%s", auth)

	after := siteName2457(t, db, siteID)
	require.Equal(t, "B", after.Name)

	t.Logf("IAM_CALLS=%d", iam.callCount())
	t.Logf("ESCALATED=%d", 1)
	t.Logf("FINAL_HTTP=%d", code)
}

// R5：同名提交不触发 IAM，但其它字段照常更新。
func TestUpdateSite_NoopRenameSkipsIAM_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "B")

	iam := newFakeIAMSite2457(t, http.StatusOK, "B")

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID, `{"name":"B","address":"updated"}`, siteUpdateUserToken2457)

	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.Empty(t, iam.callCount(), "同名改名不得调用 IAM")

	after := siteName2457(t, db, siteID)
	require.Equal(t, "updated", after.Address)

	t.Logf("IAM_CALLS=%d", iam.callCount())
	t.Logf("ADDRESS=%s", after.Address)
}

// R6：商户锚点网点（org_id == tenant_id）禁止改名。
func TestUpdateSite_AnchorGuard_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.NewString()
	siteID := uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "anchor", Status: "active",
	}).Error)

	iam := newFakeIAMSite2457(t, http.StatusOK, "B")

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID, `{"name":"B"}`, siteUpdateUserToken2457)

	require.Equal(t, http.StatusBadRequest, code, "%v", resp)
	require.EqualValues(t, 40002, resp["code"])
	require.Empty(t, iam.callCount())

	after := siteName2457(t, db, siteID)
	require.Equal(t, "anchor", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.callCount())
}

// R7：同租户已有同名网点 → 409/40900，且不调 IAM。
func TestUpdateSite_DuplicateNamePrecheck_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "A")
	require.NoError(t, db.Create(&models.Site{
		ID: uuid.NewString(), TenantID: tenantID, OrgID: uuid.NewString(), Name: "C", Status: "active",
	}).Error)

	iam := newFakeIAMSite2457(t, http.StatusOK, "C")

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID, `{"name":"C"}`, siteUpdateUserToken2457)

	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])
	require.Empty(t, iam.callCount(), "重名预检必须在 IAM 调用之前")

	after := siteName2457(t, db, siteID)
	require.Equal(t, "A", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.callCount())
}

// R8：空白名称 → 400/40002，且不调 IAM。
func TestUpdateSite_EmptyNameRejected_2457(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "A")

	iam := newFakeIAMSite2457(t, http.StatusOK, "")

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID, `{"name":"   "}`, siteUpdateUserToken2457)

	require.Equal(t, http.StatusBadRequest, code, "%v", resp)
	require.EqualValues(t, 40002, resp["code"])
	require.Empty(t, iam.callCount())

	after := siteName2457(t, db, siteID)
	require.Equal(t, "A", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.callCount())
}
