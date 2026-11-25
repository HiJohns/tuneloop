package handlers

// #2479：CreateSite 未指定负责人时，IAM CreateOrganization 返回 nil UUID 作为
// admin_id（不是空串）。旧实现只判 `adminID != ""`，于是把 nil UUID 传给
// AssignRoleTemplateToUserWithToken → 3 次 404 + 3 条 error 日志（静默降级）。
//
// 证据行（Accepted Plan R1–R4）：
//   R1 nil UUID 负责人 → STATUS=201、ASSIGN_CALLS=0、ERR_LOG=0、INFO_SKIP=1
//   R2 空串负责人     → STATUS=201、ASSIGN_CALLS=0
//   R3 正常负责人     → ASSIGN_CALLS=1、TEMPLATE_CODE=site_admin
//   R4 前两次 404 第三次 200 → ASSIGN_CALLS=3、RETRY_OK=1、ERR_LOG_FAILED_AFTER=0

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	siteCreateNilUUID2479 = "00000000-0000-0000-0000-000000000000"
	siteCreateUserTok2479 = "user-token-2479"
)

// fakeIAMCreateSite2479 覆盖 CreateSite 使用的 IAM 端点：
// token / CreateOrganization / role-templates / assign role template。
type fakeIAMCreateSite2479 struct {
	srv           *httptest.Server
	mu            sync.Mutex
	adminID       string
	siteOrgID     string
	assignCalls   int
	templateCalls int
	lastRoleCode  string
	assignStatus  []int // 依次返回的状态码；用尽后默认 200
}

func newFakeIAMCreateSite2479(t *testing.T, adminID string, assignStatus ...int) *fakeIAMCreateSite2479 {
	t.Helper()
	f := &fakeIAMCreateSite2479{adminID: adminID, siteOrgID: uuid.NewString(), assignStatus: assignStatus}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token-2479", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organizations"):
			f.mu.Lock()
			orgID, admin := f.siteOrgID, f.adminID
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"org_id": orgID, "admin_id": admin})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "role-templates"):
			f.mu.Lock()
			f.templateCalls++
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "tpl-site-admin", "code": "site_admin"},
				{"id": "tpl-site-member", "code": "site_member"},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/namespaces/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "ns-2479"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/") && strings.HasSuffix(r.URL.Path, "/roles"):
			var body struct {
				RoleIDs []string `json:"role_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.assignCalls++
			if len(body.RoleIDs) > 0 {
				f.lastRoleCode = body.RoleIDs[0]
			}
			status := http.StatusOK
			if strings.Contains(r.URL.Path, siteCreateNilUUID2479) {
				// 与真实 IAM 一致：nil UUID 目标不存在 → 404（生产 3 次重试即源于此）。
				status = http.StatusNotFound
			}
			if len(f.assignStatus) >= f.assignCalls {
				status = f.assignStatus[f.assignCalls-1]
			}
			f.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"user not found in organization"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000})
		}
	}))
	t.Cleanup(f.srv.Close)
	services.SetIAMInternalURLForTesting(f.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return f
}

func (f *fakeIAMCreateSite2479) counts() (assign, template int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.assignCalls, f.templateCalls
}

func (f *fakeIAMCreateSite2479) roleCode() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastRoleCode
}

// captureStdLog2479 捕获标准库日志（CreateSite 只用 log.Printf）。
func captureStdLog2479(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func seedMerchantForSiteCreate2479(t *testing.T) (merchantOrg, tenantID string) {
	t.Helper()
	db := testfixtures.GetTestDB(t)
	tenantID = uuid.NewString()
	merchantOrg = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), Code: testfixtures.UniqueCode("M2479"), TenantID: tenantID,
		OrgID: merchantOrg, Name: "商户2479", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	return merchantOrg, tenantID
}

func postCreateSite2479(t *testing.T, merchantOrg, tenantID, body string) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantID, OrgID: merchantOrg, UserID: uuid.NewString(), Role: "OWNER",
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/api/merchant/sites", NewSiteHandler().CreateSite)

	req := httptest.NewRequest(http.MethodPost, "/api/merchant/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+siteCreateUserTok2479)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

// R1：admin_id = nil UUID → 跳过模板分配（0 次 IAM），INFO 可见，无 error 日志。
func TestCreateSite_NilUUIDAdminSkipsTemplate_2479(t *testing.T) {
	testfixtures.SetupTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	iam := newFakeIAMCreateSite2479(t, siteCreateNilUUID2479)
	logs := captureStdLog2479(t)

	code, resp := postCreateSite2479(t, merchantOrg, tenantID, `{"name":"测试网点 2479-A"}`)

	require.Equal(t, http.StatusCreated, code, "既有成功契约是 201 Created; body=%v", resp)
	assign, template := iam.counts()
	require.Zero(t, assign, "nil UUID 不得调用 AssignRoleTemplate")
	require.Zero(t, template, "无负责人时不必拉角色模板")

	logText := logs.String()
	require.NotContains(t, logText, "AssignRoleTemplate attempt")
	require.NotContains(t, logText, "AssignRoleTemplate failed after retries")
	require.Contains(t, logText, "skip role template assignment")

	infoSkip := 0
	if strings.Contains(logText, "skip role template assignment") {
		infoSkip = 1
	}
	errLog := 0
	if strings.Contains(logText, "AssignRoleTemplate") && strings.Contains(logText, "failed") {
		errLog = 1
	}
	require.Equal(t, 0, errLog)
	require.Equal(t, 1, infoSkip)

	t.Logf("STATUS=%d", code)
	t.Logf("ASSIGN_CALLS=%d", assign)
	t.Logf("ERR_LOG=%d", errLog)
	t.Logf("INFO_SKIP=%d", infoSkip)
}

// R2：admin_id = 空串 → 同样跳过（既有语义保持）。
func TestCreateSite_EmptyAdminSkipsTemplate_2479(t *testing.T) {
	testfixtures.SetupTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	iam := newFakeIAMCreateSite2479(t, "")
	logs := captureStdLog2479(t)

	code, resp := postCreateSite2479(t, merchantOrg, tenantID, `{"name":"测试网点 2479-B"}`)

	require.Equal(t, http.StatusCreated, code, "既有成功契约是 201 Created; body=%v", resp)
	assign, _ := iam.counts()
	require.Zero(t, assign)
	require.NotContains(t, logs.String(), "AssignRoleTemplate attempt")

	t.Logf("STATUS=%d", code)
	t.Logf("ASSIGN_CALLS=%d", assign)
}

// R3：正常负责人 → 恰好一次模板分配，code=site_admin。
func TestCreateSite_WithAdminAssignsTemplate_2479(t *testing.T) {
	testfixtures.SetupTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	adminID := uuid.NewString()
	iam := newFakeIAMCreateSite2479(t, adminID)
	captureStdLog2479(t)

	code, resp := postCreateSite2479(t, merchantOrg, tenantID, `{"name":"测试网点 2479-C"}`)

	require.Equal(t, http.StatusCreated, code, "既有成功契约是 201 Created; body=%v", resp)
	assign, template := iam.counts()
	require.Equal(t, 1, assign, "有效负责人必须分配一次模板")
	require.Equal(t, 1, template)
	require.Equal(t, "site_admin", iam.roleCode())

	t.Logf("STATUS=%d", code)
	t.Logf("ASSIGN_CALLS=%d", assign)
	t.Logf("TEMPLATE_CODE=%s", iam.roleCode())
}

// R4：前两次 404、第三次 200 → 保留既有重试逻辑（3 次后成功）。
func TestCreateSite_AdminTemplateRetryKept_2479(t *testing.T) {
	testfixtures.SetupTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	adminID := uuid.NewString()
	iam := newFakeIAMCreateSite2479(t, adminID, http.StatusNotFound, http.StatusNotFound, http.StatusOK)
	logs := captureStdLog2479(t)

	code, resp := postCreateSite2479(t, merchantOrg, tenantID, `{"name":"测试网点 2479-D"}`)

	require.Equal(t, http.StatusCreated, code, "既有成功契约是 201 Created; body=%v", resp)
	assign, _ := iam.counts()
	require.Equal(t, 3, assign, "前两次 404 后必须重试到第 3 次")
	require.NotContains(t, logs.String(), "failed after retries")

	retryOK := 0
	if assign == 3 {
		retryOK = 1
	}
	errFailedAfter := 0
	if strings.Contains(logs.String(), "failed after retries") {
		errFailedAfter = 1
	}
	require.Equal(t, 1, retryOK)
	require.Equal(t, 0, errFailedAfter)

	t.Logf("STATUS=%d", code)
	t.Logf("ASSIGN_CALLS=%d", assign)
	t.Logf("RETRY_OK=%d", retryOK)
	t.Logf("ERR_LOG_FAILED_AFTER=%d", errFailedAfter)
}

// R2b（Input Truth「损坏输入拒绝」）：空白 / 非 UUID 的 admin_id 一律视为「无负责人」，
// 不得把非法值传给 IAM（0 次 AssignRoleTemplate）。
func TestCreateSite_InvalidAdminSkipsTemplate_2479(t *testing.T) {
	for _, adminID := range []string{"   ", "not-a-uuid"} {
		adminID := adminID
		t.Run(strings.ReplaceAll(adminID, " ", "blank"), func(t *testing.T) {
			testfixtures.SetupTestDB(t)
			merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
			iam := newFakeIAMCreateSite2479(t, adminID)
			logs := captureStdLog2479(t)

			code, resp := postCreateSite2479(t, merchantOrg, tenantID, `{"name":"测试网点 2479-E"}`)

			require.Equal(t, http.StatusCreated, code, "既有成功契约是 201 Created; body=%v", resp)
			assign, template := iam.counts()
			require.Zero(t, assign, "非法 admin_id 不得调用 AssignRoleTemplate")
			require.Zero(t, template)
			require.Contains(t, logs.String(), "skip role template assignment")
			require.NotContains(t, logs.String(), "AssignRoleTemplate attempt")

			t.Logf("STATUS=%d", code)
			t.Logf("ASSIGN_CALLS=%d", assign)
			t.Logf("INFO_SKIP=1")
		})
	}
}

// R7（v3 审计 Bug 1）：nil UUID 负责人 + 请求携带 manager_name 时，本地写支路
// （旧守卫 `adminID != ""`）会把 nil UUID 写进 users / site_members / sites.manager_id。
// hasAdmin 必须贯穿两支路：响应仍为 201，但本地不得出现任何 nil UUID 行。
//
// R7b（同函数的另一条本地写支路）：请求给出 manager_id（本地尚无该用户），
// IAM 仍返回 nil UUID 负责人 → 不得把 nil UUID 写进该用户的 users.iam_sub。
func TestCreateSite_NilUUIDAdminSkipsLocalRows_2479(t *testing.T) {
	var nilUsers, nilMembers, nilManagerSites, iamSubNil int64

	t.Run("manager_name_without_valid_admin", func(t *testing.T) {
		testfixtures.SetupTestDB(t)
		db := testfixtures.GetTestDB(t)
		merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
		iam := newFakeIAMCreateSite2479(t, siteCreateNilUUID2479)
		captureStdLog2479(t)

		body := `{"name":"测试网点 2479-F","manager_name":"临时负责人","manager_phone":"13800002479"}`
		code, resp := postCreateSite2479(t, merchantOrg, tenantID, body)

		require.Equal(t, http.StatusCreated, code, "既有成功契约是 201 Created; body=%v", resp)
		assign, template := iam.counts()
		require.Zero(t, assign)
		require.Zero(t, template)

		var users, members, managerSites int64
		require.NoError(t, db.Model(&models.User{}).
			Where("id = ? OR iam_sub = ?", siteCreateNilUUID2479, siteCreateNilUUID2479).
			Count(&users).Error)
		require.Zero(t, users, "NIL_LOCAL_USERS：nil UUID 不得写入 users")

		require.NoError(t, db.Model(&models.SiteMember{}).
			Where("user_id = ?", siteCreateNilUUID2479).
			Count(&members).Error)
		require.Zero(t, members, "NIL_SITE_MEMBERS：nil UUID 不得写入 site_members")

		require.NoError(t, db.Model(&models.Site{}).
			Where("manager_id = ?", siteCreateNilUUID2479).
			Count(&managerSites).Error)
		require.Zero(t, managerSites, "SITES_MANAGER_ID_NIL：nil UUID 不得写入 sites.manager_id")

		var created models.Site
		require.NoError(t, db.Where("tenant_id = ? AND name = ?", merchantOrg, "测试网点 2479-F").First(&created).Error)
		require.Nil(t, created.ManagerID, "无负责人时 sites.manager_id 必须为 NULL")

		nilUsers += users
		nilMembers += members
		nilManagerSites += managerSites
	})

	t.Run("manager_id_with_nil_iam_admin", func(t *testing.T) {
		testfixtures.SetupTestDB(t)
		db := testfixtures.GetTestDB(t)
		merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
		iam := newFakeIAMCreateSite2479(t, siteCreateNilUUID2479)
		captureStdLog2479(t)

		// manager_id 指向本地不存在的用户：handler 会补建本地行（既有 FK 语义），
		// 但 iam_sub 不得取自 nil UUID 负责人。
		managerID := uuid.NewString()
		body := `{"name":"测试网点 2479-G","manager_id":"` + managerID + `"}`
		code, resp := postCreateSite2479(t, merchantOrg, tenantID, body)

		require.Equal(t, http.StatusCreated, code, "既有成功契约是 201 Created; body=%v", resp)
		assign, template := iam.counts()
		require.Zero(t, assign)
		require.Zero(t, template)

		var manager models.User
		require.NoError(t, db.Where("id = ?", managerID).First(&manager).Error)
		require.NotEqual(t, siteCreateNilUUID2479, manager.IAMSub, "IAM_SUB_NIL：nil UUID 不得写入 users.iam_sub")

		var users int64
		require.NoError(t, db.Model(&models.User{}).
			Where("id = ? OR iam_sub = ?", siteCreateNilUUID2479, siteCreateNilUUID2479).
			Count(&users).Error)
		require.Zero(t, users, "NIL_LOCAL_USERS：nil UUID 不得写入 users")
		nilUsers += users

		var members int64
		require.NoError(t, db.Model(&models.SiteMember{}).
			Where("user_id = ?", siteCreateNilUUID2479).
			Count(&members).Error)
		require.Zero(t, members, "NIL_SITE_MEMBERS：nil UUID 不得写入 site_members")
		nilMembers += members

		var managerSites int64
		require.NoError(t, db.Model(&models.Site{}).
			Where("manager_id = ?", siteCreateNilUUID2479).
			Count(&managerSites).Error)
		require.Zero(t, managerSites, "SITES_MANAGER_ID_NIL：nil UUID 不得写入 sites.manager_id")
		nilManagerSites += managerSites

		if manager.IAMSub == siteCreateNilUUID2479 {
			iamSubNil++
		}
	})

	t.Logf("STATUS=201")
	t.Logf("NIL_LOCAL_USERS=%d", nilUsers)
	t.Logf("NIL_SITE_MEMBERS=%d", nilMembers)
	t.Logf("SITES_MANAGER_ID_NIL=%d", nilManagerSites)
	t.Logf("IAM_SUB_NIL=%d", iamSubNil)
}
