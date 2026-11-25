package handlers

// #2459：IAM organizations.name 是商户名的权威源，merchants.name 只是本地镜像。
// UpdateMerchant 改名必须先写 IAM，成功后才写本地；IAM 失败不得落库（含 admin_uid）。
// 另外落实 Owner O3 的两条业务规则：
//   - 仅商户管理员（OWNER）可修改商户名（平台/系统管理员 → 403/40301）；
//   - 商户名在 namespace 内唯一（大小写/空白归一后预检 → 409/40900）。
//
// 证据行（Accepted Plan v2 R1–R7/R12/R13）：
//   R1  改名 IAM 优先            → IAM_CALLS=1 / ORDER=iam_before_local / IAM_BODY / LOCAL_NAME
//   R2  IAM 失败不落库           → STATUS=500 / LOCAL_UNCHANGED=1
//   R3  同名幂等                 → IAM_CALLS=0 / PHONE=updated
//   R4  403 服务凭据升级         → IAM_CALLS=2 / ESCALATED=1
//   R5  IAM 409 映射             → STATUS=409 / CODE=40900
//   R6  IAM 404 映射             → STATUS=404 / CODE=40400
//   R7  空名称兼容               → IAM_CALLS=0 / NAME=unchanged
//   R12 仅商户管理员可改名       → STATUS=403 / CODE=40301 / IAM_CALLS=0
//   R13 namespace 内唯一预检     → STATUS=409 / CODE=40900 / IAM_CALLS=0

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
	merchantOrgID2459      = "5e6c7a48-0000-0000-0000-000000000459"
	merchantUserToken2459  = "user-token-2459"
	merchantSvcToken2459   = "svc-token-2459"
	merchantNamespace2459  = "ns-2459"
	merchantOtherOrgID2459 = "5e6c7a48-0000-0000-0000-000000000460"
)

// fakeIAMMerchant2459 是 UpdateMerchant 用例的最小 IAM：
// token 端点 + namespace 解析 + 同 namespace 组织列表 + PUT organizations/:id。
// 只有 PUT 计入「改名调用」（IAM_CALLS）；列表调用单独计数（LIST_CALLS）。
//
// #2459 审计（阻断项 A）：GET /organizations 必须按 Authorization 区分可见性 ——
// 服务凭据（namespace 级）看到 userOrgs 之外的全部组织，用户令牌只看到
// userOrgs（模拟 IAM ListOrgs 的 VisibleOrgIDs 裁剪，或 SelfOnly → 403）。
type fakeIAMMerchant2459 struct {
	srv            *httptest.Server
	mu             sync.Mutex
	putCalls       int
	listCalls      int
	tokenCalls     int
	lastBody       string
	lastPath       string
	lastAuth       string
	lastListAuth   string
	putStatus      int
	statusFor      map[string]int // Authorization 后缀 → 状态码（R4 的 403→200）
	orgs           []services.Organization
	userOrgs       []services.Organization
	userListStatus int
	onPut          func()
	responseOrg    string
}

func newFakeIAMMerchant2459(t *testing.T, putStatus int, orgs []services.Organization) *fakeIAMMerchant2459 {
	t.Helper()
	f := &fakeIAMMerchant2459{putStatus: putStatus, orgs: orgs}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/auth/token":
			f.mu.Lock()
			f.tokenCalls++
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": merchantSvcToken2459, "expires_in": 3600, "token_type": "Bearer",
			})
		case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": merchantNamespace2459})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations":
			auth := r.Header.Get("Authorization")
			f.mu.Lock()
			f.listCalls++
			f.lastListAuth = auth
			full := f.orgs
			visible := f.userOrgs
			userStatus := f.userListStatus
			f.mu.Unlock()
			// 服务凭据（客户端凭证 token）→ namespace 全量；其它 → 调用者可见范围。
			if !strings.HasSuffix(auth, merchantSvcToken2459) {
				if userStatus != 0 {
					w.WriteHeader(userStatus)
					_, _ = w.Write([]byte(`{"error":"insufficient permissions"}`))
					return
				}
				if visible != nil {
					full = visible
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"organizations": full, "total": len(full), "page": 1, "page_size": 1000,
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/organizations/"):
			raw, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.putCalls++
			f.lastBody = string(raw)
			f.lastPath = r.URL.Path
			f.lastAuth = r.Header.Get("Authorization")
			hook := f.onPut
			status := f.putStatus
			name := f.responseOrg
			for token, override := range f.statusFor {
				if strings.HasSuffix(f.lastAuth, token) {
					status = override
				}
			}
			if name == "" {
				name = f.orgs[0].Name
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
				"id": strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/"), "name": name,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"unexpected path"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	services.SetIAMInternalURLForTesting(f.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return f
}

func (f *fakeIAMMerchant2459) putCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putCalls
}

func (f *fakeIAMMerchant2459) listCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func (f *fakeIAMMerchant2459) snapshot() (body, path, auth string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastBody, f.lastPath, f.lastAuth
}

func (f *fakeIAMMerchant2459) setStatusForToken(token string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statusFor == nil {
		f.statusFor = map[string]int{}
	}
	f.statusFor[token] = status
}

func (f *fakeIAMMerchant2459) setOnPut(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onPut = hook
}

func (f *fakeIAMMerchant2459) setResponseOrg(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responseOrg = name
}

// setUserVisibleOrgs 模拟 IAM ListOrgs 的可见性裁剪：用户令牌只看到这些组织。
func (f *fakeIAMMerchant2459) setUserVisibleOrgs(orgs []services.Organization) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userOrgs = orgs
}

// setUserListStatus 模拟 SelfOnly 角色：用户令牌读组织列表直接 403。
func (f *fakeIAMMerchant2459) setUserListStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userListStatus = status
}

func (f *fakeIAMMerchant2459) lastListAuthHeader() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastListAuth
}

// seedMerchant2459 建一个商户：tenant_id == org_id == 商户根组织（商户管理员语义）。
func seedMerchant2459(t *testing.T, db *gorm.DB, name string) (tenantID, merchantID, adminUID string) {
	t.Helper()
	tenantID = uuid.NewString()
	merchantID = uuid.NewString()
	adminUID = uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: tenantID, Name: name,
		AdminUID: adminUID, Status: "active",
	}).Error)
	return tenantID, merchantID, adminUID
}

// merchantRouter2459 注入租户/组织/角色上下文；token 非空时带 Authorization。
// role=OWNER + tenantID==orgID → business_role=merchant_admin；
// role=NAMESPACE_ADMIN → business_role=system_admin（R12）。
func merchantRouter2459(tenantID, orgID, role string, sysPerm int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantID, OrgID: orgID, UserID: uuid.NewString(), Role: role, SysPerm: sysPerm,
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.PUT("/api/merchants/:id", NewMerchantHandler().UpdateMerchant)
	return r
}

func putMerchant2459(t *testing.T, r *gin.Engine, merchantID, body, userToken string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/merchants/"+merchantID, strings.NewReader(body))
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

func merchantByID2459(t *testing.T, db *gorm.DB, merchantID string) models.Merchant {
	t.Helper()
	var merchant models.Merchant
	require.NoError(t, db.Where("id = ?", merchantID).First(&merchant).Error)
	return merchant
}

func orgRefs2459(ownName string) []services.Organization {
	return []services.Organization{
		{ID: merchantOrgID2459, Name: ownName},
		{ID: merchantOtherOrgID2459, Name: "其它商户"},
	}
}

// R1：改名时 IAM 先于本地写入，body 为新名，落库同名。
func TestUpdateMerchant_WritesIAMBeforeLocal_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusOK, orgRefs2459("A"))
	iam.setResponseOrg("B")
	var mu sync.Mutex
	nameDuringIAM := ""
	iam.setOnPut(func() {
		var merchant models.Merchant
		if err := db.Where("id = ?", merchantID).First(&merchant).Error; err == nil {
			mu.Lock()
			nameDuringIAM = merchant.Name
			mu.Unlock()
		}
	})

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"B"}`, merchantUserToken2459)

	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.EqualValues(t, 20000, resp["code"])

	mu.Lock()
	during := nameDuringIAM
	mu.Unlock()
	order := "local_before_iam"
	if during == "A" {
		order = "iam_before_local"
	}
	body, path, _ := iam.snapshot()
	require.Equal(t, "iam_before_local", order, "IAM 调用时本地仍应是旧名")
	require.Equal(t, 1, iam.putCount(), "改名必须恰好调用一次 IAM 改名")
	require.JSONEq(t, `{"name":"B"}`, body)
	require.Contains(t, path, "/api/v1/organizations/")

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "B", after.Name)

	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("ORDER=%s", order)
	t.Logf("IAM_BODY=%s", body)
	t.Logf("LOCAL_NAME=%s", after.Name)
}

// merchantOrgID2459Or：商户根组织 == tenant（seed 约定），显式取值便于断言。
func merchantOrgID2459Or(tenantID string) string { return tenantID }

// R2：IAM 5xx 且同时请求变更 admin_uid → 5xx，本地 name 与 admin_uid 均不变。
func TestUpdateMerchant_IAMFailureKeepsLocal_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, adminUID := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusInternalServerError, orgRefs2459("A"))

	newAdmin := uuid.NewString()
	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID,
		`{"name":"B","admin_uid":"`+newAdmin+`"}`, merchantUserToken2459)

	require.Equal(t, http.StatusBadGateway, code, "%v", resp)
	require.EqualValues(t, 50000, resp["code"])
	require.Equal(t, 1, iam.putCount())

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "A", after.Name, "IAM 失败不得落本地 name")
	require.Equal(t, adminUID, after.AdminUID, "IAM 失败不得落本地 admin_uid（D3 原子性）")

	localUnchanged := 0
	if after.Name == "A" && after.AdminUID == adminUID {
		localUnchanged = 1
	}
	require.Equal(t, 1, localUnchanged)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("LOCAL_UNCHANGED=%d", localUnchanged)
	t.Logf("LOCAL_NAME=%s", after.Name)
}

// R3：名称未变 → 不调 IAM 改名，其它字段照常更新。
func TestUpdateMerchant_NoopRenameSkipsIAM_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusOK, orgRefs2459("A"))

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"A","phone":"13900000000"}`, merchantUserToken2459)

	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.Zero(t, iam.putCount(), "同名提交不得调用 IAM 改名")

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "13900000000", after.Phone)
	require.Equal(t, "A", after.Name)

	phone := after.Phone
	if phone == "13900000000" {
		phone = "updated"
	}
	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("PHONE=%s", phone)
}

// R4：用户令牌 403 且本地已通过 tenant_update（bit 8）→ 服务凭据重试一次，最终 200。
func TestUpdateMerchant_ForbiddenEscalatesToServiceCredentials_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusOK, orgRefs2459("A"))
	iam.setResponseOrg("B")
	iam.setStatusForToken(merchantUserToken2459, http.StatusForbidden)

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 1<<middleware.SysPermTenantUpdate)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"B"}`, merchantUserToken2459)

	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.EqualValues(t, 20000, resp["code"])
	require.Equal(t, 2, iam.putCount(), "403 后必须用服务凭据重试一次")
	_, _, auth := iam.snapshot()
	require.True(t, strings.HasSuffix(auth, merchantSvcToken2459), "第二次必须是服务凭据，实际=%s", auth)

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "B", after.Name)

	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("ESCALATED=%d", 1)
	t.Logf("FINAL_HTTP=%d", code)
}

// R5：IAM 409（组织重名）→ 409/40900，本地不变。
func TestUpdateMerchant_IAMConflictMapsTo409_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusConflict, orgRefs2459("A"))

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"B"}`, merchantUserToken2459)

	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])
	require.Equal(t, 1, iam.putCount())

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "A", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("LOCAL_NAME=%s", after.Name)
}

// R6：IAM 404（组织不存在）→ 404/40400，本地不变。
func TestUpdateMerchant_IAMNotFoundMapsTo404_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusNotFound, orgRefs2459("A"))

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"B"}`, merchantUserToken2459)

	require.Equal(t, http.StatusNotFound, code, "%v", resp)
	require.EqualValues(t, 40400, resp["code"])
	require.Equal(t, 1, iam.putCount())

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "A", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("LOCAL_NAME=%s", after.Name)
}

// R7：空名称/空白名称 → 不改名、返回 200、IAM 0 次（部分更新兼容，D2）。
func TestUpdateMerchant_EmptyNameKeepsCurrent_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusOK, orgRefs2459("A"))

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)

	code, resp := putMerchant2459(t, r, merchantID, `{"name":""}`, merchantUserToken2459)
	require.Equal(t, http.StatusOK, code, "%v", resp)

	code2, resp2 := putMerchant2459(t, r, merchantID, `{"name":"   "}`, merchantUserToken2459)
	require.Equal(t, http.StatusOK, code2, "%v", resp2)
	require.Zero(t, iam.putCount(), "空/空白名称不得调用 IAM 改名")

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "A", after.Name, "空名称不得清空商户名")

	name := "unchanged"
	if after.Name == "A" {
		name = "unchanged"
	} else {
		name = after.Name
	}
	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("NAME=%s", name)
}

// R12：平台/系统管理员改商户名 → 403/40301，IAM 改名 0 次，本地不变（D7）。
func TestUpdateMerchant_OnlyMerchantAdminMayRename_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusOK, orgRefs2459("A"))

	// 平台/系统管理员：role=NAMESPACE_ADMIN → business_role=system_admin。
	r := merchantRouter2459(tenantID, uuid.NewString(), "NAMESPACE_ADMIN", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"B"}`, merchantUserToken2459)

	require.Equal(t, http.StatusForbidden, code, "%v", resp)
	require.EqualValues(t, 40301, resp["code"])
	require.Zero(t, iam.putCount(), "平台管理员改名不得调用 IAM")

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "A", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("DB_NAME=%s", after.Name)
}

// R13：同 namespace 已有其它组织用该名（大小写/空白归一）→ 409/40900，
// IAM 改名 0 次，本地不变（D8）。
func TestUpdateMerchant_DuplicateNameInNamespace_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "旧名")

	iam := newFakeIAMMerchant2459(t, http.StatusOK, []services.Organization{
		{ID: tenantID, Name: "旧名"},
		{ID: merchantOtherOrgID2459, Name: "卡丹萨"},
	})

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":" 卡丹萨 "}`, merchantUserToken2459)

	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])
	require.Zero(t, iam.putCount(), "重名预检命中时不得调用 IAM 改名")
	require.Equal(t, 1, iam.listCount(), "重名预检必须读取同 namespace 组织列表")

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "旧名", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("LIST_CALLS=%d", iam.listCount())
	t.Logf("DB_NAME=%s", after.Name)
}

// R13-A（#2459 审计阻断项 A）：跨商户重名必须仍然 409。
// 真实形态：商户管理员的用户令牌只能看到本商户子树（IAM ListOrgs 的 VisibleOrgIDs），
// 对方商户组织对用户令牌不可见 —— 因此预检必须用服务凭据（namespace 级）读取。
func TestUpdateMerchant_CrossMerchantDuplicateName_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "旧名")

	self := services.Organization{ID: tenantID, Name: "旧名"}
	other := services.Organization{ID: merchantOtherOrgID2459, Name: "卡丹萨"}
	iam := newFakeIAMMerchant2459(t, http.StatusOK, []services.Organization{self, other})
	// 用户令牌视角：只看得到本商户子树（看不到 other）。
	iam.setUserVisibleOrgs([]services.Organization{self})

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"卡丹萨"}`, merchantUserToken2459)

	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])
	require.Zero(t, iam.putCount(), "跨商户重名不得调用 IAM 改名")
	require.Equal(t, 1, iam.listCount(), "重名预检必须读取同 namespace 组织列表")
	require.True(t, strings.HasSuffix(iam.lastListAuthHeader(), merchantSvcToken2459),
		"预检必须用服务凭据（namespace 级可见性），实际=%s", iam.lastListAuthHeader())

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "旧名", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("LIST_CALLS=%d", iam.listCount())
	t.Logf("CROSS_MERCHANT=1")
	t.Logf("LIST_AUTH=service")
	t.Logf("DB_NAME=%s", after.Name)
}

// R13-B（#2459 审计阻断项 A）：SelfOnly 角色下用户令牌读列表 403，
// 改名不得被误判为故障（502）—— 预检走服务凭据，权限判定仍在真实 PUT 上。
func TestUpdateMerchant_SelfOnlyUserTokenDoesNotBlockRename_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "A")

	iam := newFakeIAMMerchant2459(t, http.StatusOK, orgRefs2459("A"))
	iam.setResponseOrg("B")
	// 用户令牌读组织列表 → 403（IAM SelfOnly 分支）。
	iam.setUserListStatus(http.StatusForbidden)

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"B"}`, merchantUserToken2459)

	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.EqualValues(t, 20000, resp["code"])
	require.Equal(t, 1, iam.putCount())
	require.True(t, strings.HasSuffix(iam.lastListAuthHeader(), merchantSvcToken2459),
		"预检不得使用会被 SelfOnly 裁剪的用户令牌，实际=%s", iam.lastListAuthHeader())

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "B", after.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("SELF_ONLY=1")
	t.Logf("LIST_AUTH=service")
	t.Logf("DB_NAME=%s", after.Name)
}

// R13-C（#2459 审计建议 2）：本地兜底 —— IAM 列表完全看不到对方组织时，
// 本地 merchants 表仍能发现同 namespace（跨 tenant）重名 → 409。
func TestUpdateMerchant_LocalMerchantDuplicateName_2459(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, merchantID, _ := seedMerchant2459(t, db, "旧名")

	// 另一个商户（不同 tenant）用了目标名。
	otherTenantID, otherMerchantID, _ := seedMerchant2459(t, db, "卡丹萨")
	require.NotEqual(t, tenantID, otherTenantID)

	// IAM 列表只回本商户子树（模拟 IAM 不可见/漏项），本地仍必须拦住。
	iam := newFakeIAMMerchant2459(t, http.StatusOK, []services.Organization{{ID: tenantID, Name: "旧名"}})

	r := merchantRouter2459(tenantID, merchantOrgID2459Or(tenantID), "OWNER", 0)
	code, resp := putMerchant2459(t, r, merchantID, `{"name":"  卡丹萨  "}`, merchantUserToken2459)

	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])
	require.Zero(t, iam.putCount(), "本地重名命中不得调用 IAM 改名")

	after := merchantByID2459(t, db, merchantID)
	require.Equal(t, "旧名", after.Name)
	other := merchantByID2459(t, db, otherMerchantID)
	require.Equal(t, "卡丹萨", other.Name)

	t.Logf("STATUS=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", iam.putCount())
	t.Logf("LOCAL_CONFLICT=1")
	t.Logf("DB_NAME=%s", after.Name)
}
