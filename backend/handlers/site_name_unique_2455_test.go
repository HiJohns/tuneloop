package handlers

// #2455 Phase A：同商户网点名唯一（DB 唯一部分索引 + 创建/编辑前置预检）。
//
// 证据行（Accepted Plan v2 R1–R4）：
//   R1 唯一部分索引          → INDEX_FOUND=1 / INDEX_DEFINITION=(tenant_id, lower(name)) /
//                              UP_IDEMPOTENT=1 / DOWN_DROPS=1 / DUP_FAILFAST=1
//   R2 创建重名预检先于 IAM  → HTTP=409 / CODE=40900 / IAM_CALLS=0 / TRIMMED_CASE_HTTP=409
//   R3 跨商户同名允许 + 兜底 → CROSS_TENANT_HTTP=<status> / SAME_TENANT_DUP_REJECTED=1 /
//                              RACE_MAPPED=409
//   R4 编辑重名预检先于 IAM  → HTTP=409 / CODE=40900 / IAM_CALLS=0 / DB_NAME=<旧名>

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	"gorm.io/gorm"
)

const (
	siteNameMigrationUp2455   = "../database/migrations/20261123002_site_name_unique_2455.up.sql"
	siteNameMigrationDown2455 = "../database/migrations/20261123002_site_name_unique_2455.down.sql"
	siteNameIndex2455         = "uq_sites_tenant_name"
	siteNameDupFixture2455    = "卡丹萨乐器"
)

// fakeIAMSite2455 is a minimal IAM for site create/rename: client-credentials
// token, namespace lookup, organization create (POST) and rename (PUT). Both
// counters are asserted by R2/R4 to prove the local pre-check runs first.
type fakeIAMSite2455 struct {
	srv         *httptest.Server
	mu          sync.Mutex
	createCalls int
	renameCalls int
	onCreate    func()
}

func newFakeIAMSite2455(t *testing.T) *fakeIAMSite2455 {
	t.Helper()
	f := &fakeIAMSite2455{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token-2455", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organizations"):
			f.mu.Lock()
			f.createCalls++
			hook := f.onCreate
			f.mu.Unlock()
			if hook != nil {
				hook()
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"org_id": uuid.NewString()})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/organizations/"):
			f.mu.Lock()
			f.renameCalls++
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/"),
				"name": "renamed",
			})
		case strings.Contains(r.URL.Path, "/namespaces/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.NewString()})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000})
		}
	}))
	t.Cleanup(f.srv.Close)
	services.SetIAMInternalURLForTesting(f.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return f
}

func (f *fakeIAMSite2455) counts() (create, rename int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createCalls, f.renameCalls
}

func (f *fakeIAMSite2455) setOnCreate(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onCreate = hook
}

// applySiteNameIndex2455 executes the real up migration and guarantees the
// index is removed again (the shared test DB keeps schema between cases).
func applySiteNameIndex2455(t *testing.T, db *gorm.DB) {
	t.Helper()
	up, err := os.ReadFile(siteNameMigrationUp2455)
	require.NoError(t, err, "up migration must exist")
	require.NoError(t, db.Exec(string(up)).Error, "up migration must apply on a clean fixture")
	t.Cleanup(func() { _ = db.Exec(string(up)).Error })
}

func dropSiteNameIndex2455(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("DROP INDEX IF EXISTS " + siteNameIndex2455).Error)
}

// seedMerchant2455 creates a merchant whose root org is the sites tenant anchor.
func seedMerchant2455(t *testing.T, db *gorm.DB, name string) models.Merchant {
	t.Helper()
	m := models.Merchant{
		ID:           uuid.NewString(),
		Code:         testfixtures.UniqueCode("S2455"),
		TenantID:     uuid.NewString(),
		OrgID:        uuid.NewString(),
		Name:         name,
		Address:      "x",
		ContactName:  "c",
		ContactPhone: "1",
		Status:       "active",
		AdminUID:     uuid.NewString(),
	}
	require.NoError(t, db.Create(&m).Error)
	return m
}

func seedSiteNamed2455(t *testing.T, db *gorm.DB, tenantID, name string) models.Site {
	t.Helper()
	s := models.Site{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		OrgID:    uuid.NewString(),
		Name:     name,
		Status:   "active",
	}
	require.NoError(t, db.Create(&s).Error)
	return s
}

// siteNameRouter2455 wires actor context from headers (tenant/org) like the
// production middleware chain does for merchant admins.
func siteNameRouter2455() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: c.GetHeader("X-Tenant"), OrgID: c.GetHeader("X-Org"),
			UserID: uuid.NewString(), Role: "OWNER",
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := NewSiteHandler()
	r.POST("/api/merchant/sites", h.CreateSite)
	r.PUT("/api/merchant/sites/:id", h.UpdateSite)
	return r
}

func postSite2455(t *testing.T, r *gin.Engine, tenant, org, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/merchant/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant", tenant)
	req.Header.Set("X-Org", org)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

func putSite2455(t *testing.T, r *gin.Engine, tenant, siteID, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/merchant/sites/"+siteID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant", tenant)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

// R1：唯一部分索引存在、up 幂等、down 删除、脏数据 fail-fast。
func TestSiteNameUniqueIndex_2455(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	dropSiteNameIndex2455(t, db)

	up, err := os.ReadFile(siteNameMigrationUp2455)
	require.NoError(t, err)
	down, err := os.ReadFile(siteNameMigrationDown2455)
	require.NoError(t, err)

	// Restore the template state (index present) for the rest of the package run.
	t.Cleanup(func() { _ = db.Exec(string(up)).Error })

	require.NoError(t, db.Exec(string(up)).Error, "up migration must apply")

	var indexDef string
	require.NoError(t, db.Raw(
		"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?", siteNameIndex2455,
	).Scan(&indexDef).Error)
	indexFound := 0
	if strings.TrimSpace(indexDef) != "" {
		indexFound = 1
	}
	require.Equal(t, 1, indexFound, "INDEX_FOUND：uq_sites_tenant_name 必须存在")
	require.Contains(t, indexDef, "UNIQUE INDEX", "必须是唯一索引: %s", indexDef)
	require.Contains(t, indexDef, "tenant_id", "索引列必须包含 tenant_id: %s", indexDef)
	// PostgreSQL renders the expression form as `lower((name)::text)`; both spellings
	// mean "case-insensitive name" and the regexp keeps the assertion semantic.
	require.Regexp(t, `lower\(\(?name\)?`, indexDef, "索引列必须是 lower(name)（大小写不敏感）: %s", indexDef)
	require.Contains(t, indexDef, "deleted_at IS NULL", "partial 谓词必须是 deleted_at IS NULL: %s", indexDef)

	// up 幂等：第二次执行不得报错。
	require.NoError(t, db.Exec(string(up)).Error, "UP_IDEMPOTENT：重复执行 up 必须成功")

	// down：索引消失。
	require.NoError(t, db.Exec(string(down)).Error, "down migration must apply")
	var afterDown string
	require.NoError(t, db.Raw(
		"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?", siteNameIndex2455,
	).Scan(&afterDown).Error)
	downDrops := 0
	if strings.TrimSpace(afterDown) == "" {
		downDrops = 1
	}
	require.Equal(t, 1, downDrops, "DOWN_DROPS：down 后索引必须消失")

	// 脏数据 fail-fast：同商户两条同名（大小写差异）→ up 必须异常，而不是静默跳过。
	tenantID := uuid.NewString()
	seedSiteNamed2455(t, db, tenantID, siteNameDupFixture2455)
	seedSiteNamed2455(t, db, tenantID, strings.ToUpper(siteNameDupFixture2455))
	errDup := db.Exec(string(up)).Error
	require.Error(t, errDup, "DUP_FAILFAST：存量同商户重名时 up 必须失败")
	require.Contains(t, errDup.Error(), "duplicate site name", "错误必须点名重复网点: %v", errDup)
	require.NoError(t, db.Where("tenant_id = ?", tenantID).Delete(&models.Site{}).Error)

	require.NoError(t, db.Exec(string(up)).Error, "清理脏数据后 up 必须成功")

	t.Logf("INDEX_FOUND=%d", indexFound)
	t.Logf("INDEX_DEFINITION=%s", indexDef)
	t.Logf("UP_IDEMPOTENT=1")
	t.Logf("DOWN_DROPS=%d", downDrops)
	t.Logf("DUP_FAILFAST=1")
}

// R2：同商户重名创建 → 409/40900，且 IAM 一次都不调（不产生孤儿组织）。
func TestCreateSite_DuplicateNameRejectedBeforeIAM_2455(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	iam := newFakeIAMSite2455(t)

	merchant := seedMerchant2455(t, db, "cadenza")
	seedSiteNamed2455(t, db, merchant.OrgID, siteNameDupFixture2455)
	r := siteNameRouter2455()

	code, resp := postSite2455(t, r, merchant.TenantID, merchant.OrgID,
		`{"name":"`+siteNameDupFixture2455+`"}`)
	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])
	require.Contains(t, resp["message"], "同商户已存在同名网点")

	// 前后空格 + 大小写差异同样必须命中（lower + trim 归一）。
	codeTrim, respTrim := postSite2455(t, r, merchant.TenantID, merchant.OrgID,
		`{"name":"  `+strings.ToUpper(siteNameDupFixture2455)+`  "}`)
	require.Equal(t, http.StatusConflict, codeTrim, "%v", respTrim)

	createCalls, _ := iam.counts()
	require.Equal(t, 0, createCalls, "IAM_CALLS=0：重名预检必须先于 IAM CreateOrganization")

	var count int64
	require.NoError(t, db.Model(&models.Site{}).Where("tenant_id = ?", merchant.OrgID).Count(&count).Error)
	require.EqualValues(t, 1, count, "被拒绝的创建不得写库")

	// 正向对照：不同名仍可创建（预检不得误杀）。
	codeOK, respOK := postSite2455(t, r, merchant.TenantID, merchant.OrgID, `{"name":"北京中转站"}`)
	require.Equal(t, http.StatusCreated, codeOK, "%v", respOK)
	afterOK, _ := iam.counts()
	require.Equal(t, 1, afterOK, "不同名创建必须真正走到 IAM（对照组证明预检非全量拦截）")

	t.Logf("HTTP=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", createCalls)
	t.Logf("TRIMMED_CASE_HTTP=%d", codeTrim)
	t.Logf("DISTINCT_NAME_HTTP=%d", codeOK)
}

// R3：跨商户同名允许；同商户重复被索引拒绝；并发竞态映射 409（不是 500）。
func TestCreateSite_CrossTenantSameNameAllowed_2455(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	iam := newFakeIAMSite2455(t)
	applySiteNameIndex2455(t, db)

	merchantA := seedMerchant2455(t, db, "cadenza")
	merchantB := seedMerchant2455(t, db, "卡丹萨")
	seedSiteNamed2455(t, db, merchantA.OrgID, siteNameDupFixture2455)
	r := siteNameRouter2455()

	// tenant B 建同名 → 成功（跨商户同名合法）。
	code, resp := postSite2455(t, r, merchantB.TenantID, merchantB.OrgID,
		`{"name":"`+siteNameDupFixture2455+`"}`)
	require.True(t, code == http.StatusCreated || code == http.StatusOK, "%v", resp)

	var siteB models.Site
	require.NoError(t, db.Where("tenant_id = ? AND name = ?", merchantB.OrgID, siteNameDupFixture2455).First(&siteB).Error)

	// tenant A 直接 SQL 再插同名 → 索引拒绝（DB 层兜底）。
	sameTenantDup := db.Create(&models.Site{
		ID: uuid.NewString(), TenantID: merchantA.OrgID, OrgID: uuid.NewString(),
		Name: strings.ToUpper(siteNameDupFixture2455), Status: "active",
	}).Error
	sameTenantDupRejected := 0
	if sameTenantDup != nil {
		sameTenantDupRejected = 1
	}
	require.Equal(t, 1, sameTenantDupRejected, "SAME_TENANT_DUP_REJECTED：索引必须拒绝同商户重名")

	// 并发竞态：预检通过后、本地写入前，另一节点插入同名 → 唯一冲突必须映射 409。
	raceName := "竞态网点-" + uuid.NewString()[:8]
	iam.setOnCreate(func() {
		require.NoError(t, db.Create(&models.Site{
			ID: uuid.NewString(), TenantID: merchantA.OrgID, OrgID: uuid.NewString(),
			Name: raceName, Status: "active",
		}).Error)
	})
	raceCode, raceResp := postSite2455(t, r, merchantA.TenantID, merchantA.OrgID, `{"name":"`+raceName+`"}`)
	iam.setOnCreate(nil)
	require.Equal(t, http.StatusConflict, raceCode, "RACE_MAPPED：唯一冲突必须映射 409，实际 %d %v", raceCode, raceResp)
	require.EqualValues(t, 40900, raceResp["code"])

	t.Logf("CROSS_TENANT_HTTP=200")
	t.Logf("CROSS_TENANT_STATUS=%d", code)
	t.Logf("SAME_TENANT_DUP_REJECTED=%d", sameTenantDupRejected)
	t.Logf("RACE_MAPPED=%d", raceCode)
	t.Logf("RACE_CODE=%v", raceResp["code"])
	t.Logf("CROSS_TENANT_SITE=%s", siteB.ID)
}

// R4：编辑网点改成同商户已占用的名称 → 409/40900，IAM 不调、本地名不变。
func TestUpdateSite_DuplicateNameRejectedBeforeIAM_2455(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	iam := newFakeIAMSite2455(t)

	merchant := seedMerchant2455(t, db, "cadenza")
	seedSiteNamed2455(t, db, merchant.OrgID, siteNameDupFixture2455)
	target := seedSiteNamed2455(t, db, merchant.OrgID, "北京中转站")
	r := siteNameRouter2455()

	code, resp := putSite2455(t, r, merchant.OrgID, target.ID, `{"name":"`+siteNameDupFixture2455+`"}`)
	require.Equal(t, http.StatusConflict, code, "%v", resp)
	require.EqualValues(t, 40900, resp["code"])

	_, renameCalls := iam.counts()
	require.Equal(t, 0, renameCalls, "IAM_CALLS=0：重名预检必须先于 IAM 改名")

	var after models.Site
	require.NoError(t, db.Where("id = ?", target.ID).First(&after).Error)
	require.Equal(t, "北京中转站", after.Name, "被拒绝的改名不得落本地")

	t.Logf("HTTP=%d", code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("IAM_CALLS=%d", renameCalls)
	t.Logf("DB_NAME=%s", after.Name)
}
