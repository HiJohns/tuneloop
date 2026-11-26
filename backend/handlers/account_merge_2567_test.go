package handlers

// #2567：扫码绑定 = 账户合并（X→Y 的 tuneloop 侧缓存一致性）。
//
//   R1 合并后缓存镜像：Y 继承组织身份（role/tenant_id/org_id 来自 merchant_members），
//      X 回落顾客态（USER/zero/zero）且 status=obsolete，成员行不重复。
//   R2 成员列表可见性：存活账户在列表、废止账户不出现。
//   R3 审计：action=ACCOUNT_MERGE，details 含 from/to iam_sub 与 source=qr_bind。
//   R6 本地待办关闭：X 的 pending staff_invites → rejected(account_merged)、
//      waiting confirmation_sessions → cancelled（不碰 IAM 库）。
//
// 每条用例把机器可判定 token 打到 t.Log 里，由 scripts/verify-2567.sh 逐行抓取。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

type mergeEnv2567 struct {
	tenantID   string
	orgID      string
	merchantID string
	siteID     string
	x          models.User // 员工账户（合并后 obsolete）
	y          models.User // 顾客账户（保留方）
}

// seedMergeEnv2567 seeds a customer Y and a merchant-direct staff X in one
// tenant. Y is a pure customer (USER + nil-attribution zero UUIDs) exactly as a
// WeChat self-registered account looks before the merge.
func seedMergeEnv2567(t *testing.T) *mergeEnv2567 {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		t.Skip("test database unavailable")
	}
	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	siteID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "合并测试网点", Type: "normal", Status: "active",
	}).Error)

	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: tenantID, Name: "合并测试商户", Status: "active",
		// admin_uid 是 uuid 列：留空字符串会被 Postgres 拒绝（22P02）。
		AdminUID: uuid.NewString(), Code: testfixtures.UniqueCode("m2567"),
	}).Error)

	x := models.User{
		ID: uuid.NewString(), IAMSub: "iam-X-2567", TenantID: tenantID, OrgID: tenantID,
		Name: "原员工C", Username: "staff-2567", Phone: "13900000567", Role: "merchant_staff", Status: "active",
	}
	y := models.User{
		ID: uuid.NewString(), IAMSub: "iam-Y-2567", TenantID: zeroUUID, OrgID: zeroUUID,
		Name: "顾客A", Username: "cust-2567", Phone: "13900000568", Role: "USER", Status: "active",
	}
	require.NoError(t, db.Create(&x).Error)
	require.NoError(t, db.Create(&y).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		ID: uuid.NewString(), TenantID: tenantID, MerchantID: merchantID, UserID: x.ID,
		Role: "merchant_staff", Status: "active",
	}).Error)

	return &mergeEnv2567{tenantID: tenantID, orgID: orgID, merchantID: merchantID, siteID: siteID, x: x, y: y}
}

// R1：合并后 Y 的缓存三元组来自成员关系，X 回落顾客态且成员行唯一。
func TestAccountMergeReconcilesCache_2567(t *testing.T) {
	env := seedMergeEnv2567(t)
	db := database.GetDB()

	require.NoError(t, migrateMergedAccountLocal(db, env.x.IAMSub, env.y.IAMSub))

	var gotX, gotY models.User
	require.NoError(t, db.Where("id = ?", env.x.ID).First(&gotX).Error)
	require.NoError(t, db.Where("id = ?", env.y.ID).First(&gotY).Error)

	assert.Equal(t, "merchant_staff", gotY.Role, "A_ROLE")
	assert.Equal(t, env.tenantID, gotY.TenantID, "A_TENANT")
	assert.Equal(t, env.tenantID, gotY.OrgID, "A_ORG（商户组织 = 其 tenant）")
	assert.NotEqual(t, zeroUUID, gotY.TenantID, "A_TENANT_NON_NIL")
	assert.Equal(t, "active", gotY.Status, "A_STATUS")
	assert.Equal(t, "obsolete", gotX.Status, "C_STATUS")
	assert.Equal(t, "USER", gotX.Role, "C_ROLE")
	assert.Equal(t, zeroUUID, gotX.TenantID, "C_TENANT_ZERO")
	assert.Equal(t, zeroUUID, gotX.OrgID, "C_ORG_ZERO")

	var membershipRows int64
	require.NoError(t, db.Model(&models.MerchantMember{}).
		Where("user_id IN ?", []string{env.x.ID, env.y.ID}).Count(&membershipRows).Error)
	assert.EqualValues(t, 1, membershipRows, "MEMBERSHIP_ROWS")

	// 二次执行幂等：缓存三元组不漂移、成员行不复制。
	require.NoError(t, migrateMergedAccountLocal(db, env.x.IAMSub, env.y.IAMSub))
	require.NoError(t, db.Where("id = ?", env.y.ID).First(&gotY).Error)
	assert.Equal(t, "merchant_staff", gotY.Role, "IDEMPOTENT_ROLE")
	require.NoError(t, db.Model(&models.MerchantMember{}).
		Where("user_id IN ?", []string{env.x.ID, env.y.ID}).Count(&membershipRows).Error)
	assert.EqualValues(t, 1, membershipRows, "IDEMPOTENT_MEMBERSHIP_ROWS")

	t.Logf("A_ROLE=%s A_TENANT=%s A_ORG=%s A_TENANT_NON_NIL=1 C_STATUS=%s C_ROLE=%s MEMBERSHIP_ROWS=%d IDEMPOTENT_ROLE=%s IDEMPOTENT_MEMBERSHIP_ROWS=%d",
		gotY.Role, gotY.TenantID, gotY.OrgID, gotX.Status, gotX.Role, membershipRows, gotY.Role, membershipRows)
}

// R2：成员列表只列存活账户——废止账户不再以「禁用」形态出现。
func TestListStaffHidesObsolete_2567(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := seedMergeEnv2567(t)
	db := database.GetDB()

	// D3 的边界样本：待确认（pending）行必须保留（PC 列表渲染「待确认」、激活流程依赖），
	// disabled 行必须与 obsolete 一样被隐藏。allow-list 见 user_cache.go。
	pendingUser := models.User{
		ID: uuid.NewString(), IAMSub: "iam-P-2567", TenantID: env.tenantID, OrgID: env.tenantID,
		Name: "待确认员工", Username: "pending-2567", Phone: "13900000569", Role: "merchant_staff", Status: "pending",
	}
	disabledUser := models.User{
		ID: uuid.NewString(), IAMSub: "iam-D-2567", TenantID: env.tenantID, OrgID: env.tenantID,
		Name: "停用员工", Username: "disabled-2567", Phone: "13900000570", Role: "merchant_staff", Status: "disabled",
	}
	require.NoError(t, db.Create(&pendingUser).Error)
	require.NoError(t, db.Create(&disabledUser).Error)

	require.NoError(t, migrateMergedAccountLocal(db, env.x.IAMSub, env.y.IAMSub))

	// 观察者是该商户组织的管理员：org 取合并后 Y 归属的商户组织（=tenant）。
	r := staffRouter2567(env.tenantID, env.tenantID, env.y.ID)
	req := httptest.NewRequest(http.MethodGet, "/staff?page=1&page_size=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	var aVisible, cVisible bool
	var obsoleteRows, disabledRows, pendingRows int
	for _, item := range resp.Data.List {
		id, _ := item["id"].(string)
		status, _ := item["status"].(string)
		if status == "obsolete" {
			obsoleteRows++
		}
		if status == "disabled" {
			disabledRows++
		}
		if status == "pending" {
			pendingRows++
		}
		switch id {
		case env.y.ID:
			aVisible = true
		case env.x.ID:
			cVisible = true
		}
	}
	assert.True(t, aVisible, "A_IN_LIST：存活账户必须出现在成员列表")
	assert.False(t, cVisible, "C_IN_LIST：废止账户不得出现")
	assert.Equal(t, 0, obsoleteRows, "OBSOLETE_ROWS")
	assert.Equal(t, 0, disabledRows, "DISABLED_ROWS")
	assert.Equal(t, 1, pendingRows, "PENDING_ROWS：待确认行必须保留")

	t.Logf("A_IN_LIST=%d C_IN_LIST=%d OBSOLETE_ROWS=%d DISABLED_ROWS=%d PENDING_ROWS=%d",
		boolToken(aVisible), boolToken(cVisible), obsoleteRows, disabledRows, pendingRows)
}

// R2（人员管理三视图）：`GET /api/admin/personnel` 同样隐藏废止账户。
//
// 矩阵行把「成员列表可见性」的入口写成 `GET /api/staff` **与**
// `GET /api/admin/personnel`，两条路径实现不同（user_staff.go / personnel.go），
// 必须各自有证据，不能复用同一份。
func TestPersonnelMerchantViewHidesObsolete_2567(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := seedMergeEnv2567(t)
	db := database.GetDB()

	// 网点成员身份：合并后随 X→Y 迁移，废止的 X 不得再出现在人员视图。
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: env.tenantID, SiteID: env.siteID, UserID: env.x.ID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	require.NoError(t, migrateMergedAccountLocal(db, env.x.IAMSub, env.y.IAMSub))

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{
				TenantID: env.tenantID, OrgID: env.tenantID, UserID: env.y.ID,
				Role: "ADMIN", SysPerm: -1, CusPerm: -1,
			}.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := NewPersonnelHandler()
	r.GET("/admin/personnel", h.List)

	req := httptest.NewRequest(http.MethodGet, "/admin/personnel?page=1&page_size=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	var aVisible, cVisible bool
	var obsoleteRows int
	for _, item := range resp.Data.List {
		id, _ := item["user_id"].(string)
		status, _ := item["status"].(string)
		if status == "obsolete" {
			obsoleteRows++
		}
		switch id {
		case env.y.ID:
			aVisible = true
		case env.x.ID:
			cVisible = true
		}
	}
	assert.True(t, aVisible, "PERSONNEL_A_IN_LIST：存活账户必须出现在人员管理")
	assert.False(t, cVisible, "PERSONNEL_C_IN_LIST：废止账户不得出现")
	assert.Equal(t, 0, obsoleteRows, "PERSONNEL_OBSOLETE_ROWS")

	t.Logf("PERSONNEL_A_IN_LIST=%d PERSONNEL_C_IN_LIST=%d PERSONNEL_OBSOLETE_ROWS=%d",
		boolToken(aVisible), boolToken(cVisible), obsoleteRows)
}

// R3：合并写审计（from/to iam_sub + 来源 qr_bind），失败即整笔回滚。
func TestAccountMergeWritesAudit_2567(t *testing.T) {
	env := seedMergeEnv2567(t)
	db := database.GetDB()

	require.NoError(t, migrateMergedAccountLocal(db, env.x.IAMSub, env.y.IAMSub))

	var audits []models.AuditLog
	require.NoError(t, db.Where("action = ?", "ACCOUNT_MERGE").Find(&audits).Error)
	require.Len(t, audits, 1, "合并必须且只写一条 ACCOUNT_MERGE 审计")

	audit := audits[0]
	assert.Equal(t, "user", audit.ResourceType, "AUDIT_RESOURCE")
	assert.Equal(t, env.x.ID, audit.ResourceID, "AUDIT_RESOURCE_ID=被废止账户")

	var details map[string]interface{}
	require.NotNil(t, audit.Details)
	require.NoError(t, json.Unmarshal([]byte(*audit.Details), &details))

	hasFromTo := 0
	if details["from_iam_sub"] == env.x.IAMSub && details["to_iam_sub"] == env.y.IAMSub {
		hasFromTo = 1
	}
	assert.Equal(t, 1, hasFromTo, "AUDIT_HAS_FROM_TO")
	assert.Equal(t, "qr_bind", details["source"], "AUDIT_SOURCE")
	assert.Equal(t, env.x.ID, details["from_local"], "AUDIT_FROM_LOCAL")
	assert.Equal(t, env.y.ID, details["to_local"], "AUDIT_TO_LOCAL")

	// Rev.3（Round 1 Audit Bug 1）：组织归属必须来自合并后的缓存，不是合并前的空值。
	auditOrgID := 0
	if details["org_id"] == env.tenantID && audit.OrgID != nil && *audit.OrgID == env.tenantID {
		auditOrgID = 1
	}
	assert.Equal(t, 1, auditOrgID, "AUDIT_ORG_ID=NON_NIL")
	assert.Equal(t, "membership", details["reconcile_source"], "RECONCILE_SOURCE")

	t.Logf("AUDIT_ACTION=%s AUDIT_HAS_FROM_TO=%d AUDIT_SOURCE=%v AUDIT_RESOURCE=%s AUDIT_ORG_ID=%s RECONCILE_SOURCE=%v",
		audit.Action, hasFromTo, details["source"], audit.ResourceType,
		map[bool]string{true: "NON_NIL", false: "NIL"}[auditOrgID == 1], details["reconcile_source"])
}

// R7（Rev.3）：审计归属取合并后落库值；legacy 员工行（iam_sub 为空）的
// from_iam_sub 按合并前的历史值留痕（空），且不得覆盖存活账户的 iam_sub。
func TestAccountMergeAuditAttribution_2567(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		t.Skip("test database unavailable")
	}
	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	siteID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "归属测试网点", Type: "normal", Status: "active",
	}).Error)
	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: tenantID, Name: "归属测试商户", Status: "active",
		AdminUID: uuid.NewString(), Code: testfixtures.UniqueCode("m2567r7"),
	}).Error)

	// X = legacy 员工行：本地 iam_sub 为空，IAM sub 只存在于 users.id
	// （findLocalUserByIAMSub 的 legacy 回退路径）。
	legacySub := uuid.NewString()
	x := models.User{
		ID: legacySub, IAMSub: "", TenantID: tenantID, OrgID: tenantID,
		Name: "legacy员工C", Username: "legacy-2567", Phone: "13900000701",
		Role: "merchant_staff", Status: "active",
	}
	// Y = 存活顾客账户，已有自己的 iam_sub（合并后必须保持）。
	toSub := "iam-Y-r7-2567"
	y := models.User{
		ID: uuid.NewString(), IAMSub: toSub, TenantID: zeroUUID, OrgID: zeroUUID,
		Name: "顾客A", Username: "cust-2567-r7", Phone: "13900000702", Role: "USER", Status: "active",
	}
	require.NoError(t, db.Create(&x).Error)
	require.NoError(t, db.Create(&y).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		ID: uuid.NewString(), TenantID: tenantID, MerchantID: merchantID, UserID: legacySub,
		Role: "merchant_staff", Status: "active",
	}).Error)

	require.NoError(t, migrateMergedAccountLocal(db, legacySub, toSub))

	var audits []models.AuditLog
	require.NoError(t, db.Where("action = ?", "ACCOUNT_MERGE").Find(&audits).Error)
	require.Len(t, audits, 1)
	audit := audits[0]

	var details map[string]interface{}
	require.NotNil(t, audit.Details)
	require.NoError(t, json.Unmarshal([]byte(*audit.Details), &details))

	orgNonNil := 0
	if audit.OrgID != nil && *audit.OrgID == tenantID && details["org_id"] == tenantID {
		orgNonNil = 1
	}
	toSubOK := 0
	if details["to_iam_sub"] == toSub && toSub != "" {
		toSubOK = 1
	}
	legacyFromEmpty := 0
	if details["from_iam_sub"] == "" {
		legacyFromEmpty = 1
	}

	assert.Equal(t, 1, orgNonNil, "AUDIT_ORG_ID=NON_NIL")
	assert.Equal(t, 1, toSubOK, "AUDIT_TO_IAM_SUB=NON_EMPTY")
	assert.Equal(t, 1, legacyFromEmpty, "LEGACY_FROM_IAM_SUB_EMPTY")
	assert.Equal(t, "membership", details["reconcile_source"], "RECONCILE_SOURCE")
	assert.Equal(t, legacySub, details["from_local"], "LEGACY_TRACEABLE_BY_LOCAL_ID")

	t.Logf("AUDIT_ORG_ID=%s AUDIT_TO_IAM_SUB=%s LEGACY_FROM_IAM_SUB_EMPTY=%d RECONCILE_SOURCE=%v FROM_LOCAL_TRACEABLE=%d",
		map[bool]string{true: "NON_NIL", false: "NIL"}[orgNonNil == 1],
		map[bool]string{true: "NON_EMPTY", false: "EMPTY"}[toSubOK == 1],
		legacyFromEmpty, details["reconcile_source"], map[bool]int{true: 1, false: 0}[details["from_local"] == legacySub])
}

// R6：废止账户的本地待办/邀请被关闭（rejected / cancelled），不触碰 IAM 库。
func TestAccountMergeClosesLocalTodo_2567(t *testing.T) {
	env := seedMergeEnv2567(t)
	db := database.GetDB()

	invitee := env.x.ID
	require.NoError(t, db.Create(&models.StaffInvite{
		ID: uuid.NewString(), TenantID: env.tenantID, OrgID: env.orgID,
		Kind: "merchant_member", InviteeUserID: &invitee, Role: "merchant_staff",
		Code: "INV" + uuid.NewString()[:8], ExpiresAt: time.Now().Add(24 * time.Hour), Status: "pending",
	}).Error)
	require.NoError(t, db.Create(&models.ConfirmationSession{
		ID: uuid.NewString(), TenantID: env.tenantID, OrgID: env.orgID, UserID: env.x.ID,
		// MerchantID / ActionTargetID 是 uuid 列：置空字符串会被 Postgres 拒绝（22P02）。
		MerchantID: env.merchantID, ActionTargetID: env.x.ID,
		ConfirmType: "invite", ConfirmTarget: env.x.Phone, ActionType: "join_org",
		Status: "waiting", Token: "tok" + uuid.NewString()[:8], ExpiresAt: time.Now().Add(24 * time.Hour),
	}).Error)

	require.NoError(t, migrateMergedAccountLocal(db, env.x.IAMSub, env.y.IAMSub))

	var invite models.StaffInvite
	require.NoError(t, db.Where("invitee_user_id = ?", env.x.ID).First(&invite).Error)
	invitesClosed := 0
	if invite.Status == "rejected" && invite.RejectReason == "account_merged" {
		invitesClosed = 1
	}
	assert.Equal(t, 1, invitesClosed, "LOCAL_INVITES_CLOSED")

	var session models.ConfirmationSession
	require.NoError(t, db.Where("user_id = ?", env.x.ID).First(&session).Error)
	sessionsClosed := 0
	if session.Status == "cancelled" {
		sessionsClosed = 1
	}
	assert.Equal(t, 1, sessionsClosed, "CONFIRM_SESSIONS_CLOSED")

	// 红线：合并只操作本地表，不得出现直连 IAM 库的 user_task_queues。
	iamDBTouched := 0
	if db.Migrator().HasTable("user_task_queues") {
		iamDBTouched = 1
	}
	assert.Equal(t, 0, iamDBTouched, "IAM_DB_TOUCHED")

	t.Logf("LOCAL_INVITES_CLOSED=%d CONFIRM_SESSIONS_CLOSED=%d IAM_DB_TOUCHED=%d",
		invitesClosed, sessionsClosed, iamDBTouched)
}

// staffRouter2567 injects a tenant-scoped operator context for ListStaff.
func staffRouter2567(tenantID, orgID, operatorID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{
				TenantID: tenantID, OrgID: orgID, UserID: operatorID,
				Role: "ADMIN", SysPerm: -1, CusPerm: -1,
			}.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := &UserStaffHandler{}
	r.GET("/staff", h.ListStaff)
	return r
}

func boolToken(v bool) int {
	if v {
		return 1
	}
	return 0
}
