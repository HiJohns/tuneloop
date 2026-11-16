package handlers

// #2394（父 #2385）成员移除审批流回归：
//   R1 申请与唯一性（首次 201，重复 409）
//   R2 批准执行（删除 site_members + IAM 解绑 1 次 + 状态 executed + 审计/通知）
//   R3 边界拦截（网点管理员直接 remove → 403 40901；商户管理员仍可直接移除）
//   R4 迁移 up → down → up（表/部分唯一索引）

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type unbindCall2394 struct {
	OrgID  string
	UserID string
	Action string
}

type unbindRecorder2394 struct {
	mu    sync.Mutex
	calls []unbindCall2394
}

func (r *unbindRecorder2394) record(call unbindCall2394) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *unbindRecorder2394) snapshot() []unbindCall2394 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]unbindCall2394, len(r.calls))
	copy(out, r.calls)
	return out
}

type removalFixture2394 struct {
	db           *gorm.DB
	merchantOrg  string // JWT tid = 商户 org
	namespaceOrg string // merchants.tenant_id = 命名空间 org（≠ JWT tid）
	siteOrg      string // 网点 IAM org
	merchantID   string
	siteID       string
	requesterID  string // 申请人本地 id
	requesterSub string // 申请人 IAM sub
	targetID     string // 被移除成员本地 id
	targetSub    string // 被移除成员 IAM sub
	adminSub     string // 商户管理员 IAM sub
	rec          *unbindRecorder2394
	createSrv    *httptest.Server
}

func newIAMMock2394(t *testing.T, rec *unbindRecorder2394) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2394", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/bind") {
			var body struct {
				Action string `json:"action"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			// /api/v1/organizations/{org}/users/{user}/bind
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/"), "/")
			if len(parts) >= 4 {
				rec.record(unbindCall2394{OrgID: parts[0], UserID: parts[2], Action: body.Action})
			}
			writeJSON(w, map[string]interface{}{"code": 20000, "message": "success"})
			return
		}
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/perm-version/increment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	return httptest.NewServer(mux)
}

func setupRemoval2394(t *testing.T) *removalFixture2394 {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.MemberRemovalRequest{}))
	// 该表不在 testfixtures 的 truncate 清单内，用例内自行清理。
	require.NoError(t, db.Exec("DELETE FROM member_removal_requests").Error)

	f := &removalFixture2394{
		db:           db,
		merchantOrg:  uuid.NewString(),
		namespaceOrg: uuid.NewString(),
		siteOrg:      uuid.NewString(),
		merchantID:   uuid.NewString(),
		siteID:       uuid.NewString(),
		requesterID:  uuid.NewString(),
		requesterSub: uuid.NewString(),
		targetID:     uuid.NewString(),
		targetSub:    uuid.NewString(),
		adminSub:     uuid.NewString(),
		rec:          &unbindRecorder2394{},
	}

	require.NoError(t, db.Create(&models.Merchant{
		ID: f.merchantID, TenantID: f.namespaceOrg, OrgID: f.merchantOrg,
		Name: "M2394", Code: "M2394-" + f.merchantOrg[:8],
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active",
		AdminUID: uuid.NewString(),
	}).Error)

	require.NoError(t, db.Create(&models.Site{
		ID: f.siteID, TenantID: f.merchantOrg, OrgID: f.siteOrg, Name: "S2394", Status: "active",
	}).Error)

	// site_members.tenant_id 有 FK → tenants(id)
	require.NoError(t, db.Create(&models.Tenant{
		ID: f.merchantOrg, Name: "T2394-" + f.merchantOrg[:8], Status: "active",
	}).Error)

	require.NoError(t, db.Create(&models.User{
		ID: f.requesterID, IAMSub: f.requesterSub, TenantID: f.merchantOrg, OrgID: f.siteOrg,
		Username: "requester2394", Name: "网点管理员", Role: "ADMIN", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: f.targetID, IAMSub: f.targetSub, TenantID: f.merchantOrg, OrgID: f.siteOrg,
		Username: "target2394", Name: "被移除成员", Role: "STAFF", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: uuid.NewString(), IAMSub: f.adminSub, TenantID: f.merchantOrg, OrgID: f.merchantOrg,
		Username: "merchantadmin2394", Name: "商户管理员", Role: "OWNER", Status: "active",
	}).Error)

	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: f.merchantOrg, SiteID: f.siteID,
		UserID: f.targetID, Role: "site_member", Roles: []string{"site_member"},
	}).Error)

	f.createSrv = newIAMMock2394(t, f.rec)
	t.Cleanup(f.createSrv.Close)
	services.SetIAMInternalURLForTesting(f.createSrv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	return f
}

func (f *removalFixture2394) siteAdminActor() testutil.TestActor {
	return testutil.TestActor{TenantID: f.merchantOrg, OrgID: f.siteOrg, UserID: f.requesterSub, Role: "ADMIN"}
}

func (f *removalFixture2394) merchantAdminActor() testutil.TestActor {
	return testutil.TestActor{TenantID: f.merchantOrg, OrgID: f.merchantOrg, UserID: f.adminSub, Role: "OWNER"}
}

func removalRouter2394(actor testutil.TestActor) (*gin.Engine, *MemberRemovalRequestHandler) {
	h := NewMemberRemovalRequestHandler()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/sites/:id/members/:user_id/removal-requests", h.CreateSiteMemberRemovalRequest)
	r.GET("/admin/removal-requests", h.ListRemovalRequests)
	r.POST("/admin/removal-requests/:id/approve", h.ApproveRemovalRequest)
	r.POST("/admin/removal-requests/:id/reject", h.RejectRemovalRequest)
	r.POST("/admin/removal-requests/:id/cancel", h.CancelRemovalRequest)
	return r, h
}

func doJSON2394(t *testing.T, r *gin.Engine, method, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(raw))
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// R1：连续两次申请 → 首次 201，第二条 409，库中仅 1 条 pending。
func TestMemberRemovalCreateRequest_2394(t *testing.T) {
	f := setupRemoval2394(t)
	r, _ := removalRouter2394(f.siteAdminActor())

	code, resp := doJSON2394(t, r, http.MethodPost,
		"/sites/"+f.siteID+"/members/"+f.targetID+"/removal-requests", map[string]interface{}{"reason": "长期不到岗"})
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)
	require.EqualValues(t, 20100, resp["code"])

	code2, resp2 := doJSON2394(t, r, http.MethodPost,
		"/sites/"+f.siteID+"/members/"+f.targetID+"/removal-requests", map[string]interface{}{"reason": "重复提交"})
	require.Equal(t, http.StatusConflict, code2, "resp=%v", resp2)
	require.EqualValues(t, 40900, resp2["code"])

	var rows []models.MemberRemovalRequest
	require.NoError(t, f.db.Where("site_id = ? AND target_user_id = ? AND status = ?", f.siteID, f.targetID, removalStatusPending).
		Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, f.merchantID, rows[0].MerchantID)
	assert.Equal(t, f.requesterID, rows[0].RequesterID)
	assert.Equal(t, "长期不到岗", rows[0].Reason)
}

// R2：商户管理员批准 → IAM 解绑 1 次（网点 org + IAM sub）、site_members 删除、状态 executed、审计两条。
func TestMemberRemovalApprove_2394(t *testing.T) {
	f := setupRemoval2394(t)
	siteRouter, _ := removalRouter2394(f.siteAdminActor())
	code, resp := doJSON2394(t, siteRouter, http.MethodPost,
		"/sites/"+f.siteID+"/members/"+f.targetID+"/removal-requests", map[string]interface{}{"reason": "长期不到岗"})
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)
	data, _ := resp["data"].(map[string]interface{})
	require.NotNil(t, data)
	requestID, _ := data["request_id"].(string)
	require.NotEmpty(t, requestID)

	adminRouter, _ := removalRouter2394(f.merchantAdminActor())
	code2, resp2 := doJSON2394(t, adminRouter, http.MethodPost, "/admin/removal-requests/"+requestID+"/approve", nil)
	require.Equal(t, http.StatusOK, code2, "resp=%v", resp2)
	require.EqualValues(t, 20000, resp2["code"])

	var req models.MemberRemovalRequest
	require.NoError(t, f.db.Where("id = ?", requestID).First(&req).Error)
	assert.Equal(t, removalStatusExecuted, req.Status)
	require.NotNil(t, req.DecidedBy)

	var members int64
	require.NoError(t, f.db.Model(&models.SiteMember{}).
		Where("tenant_id = ? AND site_id = ? AND user_id = ?", f.merchantOrg, f.siteID, f.targetID).
		Count(&members).Error)
	assert.EqualValues(t, 0, members, "批准后 site_members 必须删除")

	calls := f.rec.snapshot()
	require.Len(t, calls, 1, "IAM 解绑必须恰好调用 1 次")
	assert.Equal(t, "unbind", calls[0].Action)
	assert.Equal(t, f.siteOrg, calls[0].OrgID)
	assert.Equal(t, f.targetSub, calls[0].UserID)

	var approvedAudits, executedAudits int64
	require.NoError(t, f.db.Model(&models.AuditLog{}).
		Where("resource_id = ? AND action = ?", requestID, "member_removal_approved").
		Count(&approvedAudits).Error)
	require.NoError(t, f.db.Model(&models.AuditLog{}).
		Where("resource_id = ? AND action = ?", requestID, "member_removal_executed").
		Count(&executedAudits).Error)
	assert.EqualValues(t, 1, approvedAudits, "批准审计一条")
	assert.EqualValues(t, 1, executedAudits, "执行审计一条")

	var notices int64
	require.NoError(t, f.db.Model(&models.Notification{}).
		Where("user_id = ? AND type = ?", f.requesterID, "removal_approved").
		Count(&notices).Error)
	assert.EqualValues(t, 1, notices, "申请人必须收到结果通知")
}

// R3：网点管理员直接 remove → 403 removal_requires_approval；商户管理员仍可直接移除。
func TestSiteRemoveRequiresApproval_2394(t *testing.T) {
	f := setupRemoval2394(t)

	siteMemberHandler := NewSiteMemberHandler()
	siteRouter := gin.New()
	siteRouter.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(f.siteAdminActor().InjectContext(c.Request.Context()))
		c.Next()
	})
	siteRouter.DELETE("/sites/:id/members/:uid", siteMemberHandler.RemoveMember)

	code, resp := doJSON2394(t, siteRouter, http.MethodDelete, "/sites/"+f.siteID+"/members/"+f.targetID, nil)
	require.Equal(t, http.StatusForbidden, code, "resp=%v", resp)
	assert.EqualValues(t, 40901, resp["code"])
	assert.Equal(t, "removal_requires_approval", resp["message"])

	var still int64
	require.NoError(t, f.db.Model(&models.SiteMember{}).
		Where("site_id = ? AND user_id = ?", f.siteID, f.targetID).Count(&still).Error)
	assert.EqualValues(t, 1, still, "被拦截时成员关系不得变化")

	// 商户管理员仍可直接移除（D-R2：直接执行）
	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(f.merchantAdminActor().InjectContext(c.Request.Context()))
		c.Next()
	})
	adminRouter.DELETE("/sites/:id/members/:uid", siteMemberHandler.RemoveMember)
	code2, resp2 := doJSON2394(t, adminRouter, http.MethodDelete, "/sites/"+f.siteID+"/members/"+f.targetID, nil)
	require.Equal(t, http.StatusOK, code2, "resp=%v", resp2)

	require.NoError(t, f.db.Model(&models.SiteMember{}).
		Where("site_id = ? AND user_id = ?", f.siteID, f.targetID).Count(&still).Error)
	assert.EqualValues(t, 0, still)
}

// R4：迁移文件成对，且 up → down → up 可重复执行（表 + 部分唯一索引存在）。
func TestMemberRemovalMigration_2394(t *testing.T) {
	const (
		upPath   = "../database/migrations/20261116002_member_removal_requests_2394.up.sql"
		downPath = "../database/migrations/20261116002_member_removal_requests_2394.down.sql"
	)
	upSQL, err := os.ReadFile(upPath)
	require.NoError(t, err, "up migration missing")
	downSQL, err := os.ReadFile(downPath)
	require.NoError(t, err, "down migration missing")
	require.Contains(t, string(upSQL), "CREATE TABLE IF NOT EXISTS member_removal_requests")
	require.Contains(t, string(upSQL), "uq_member_removal_pending_site_user")
	require.Contains(t, string(downSQL), "DROP TABLE IF EXISTS member_removal_requests")

	// DB 级 up → down → up（测试库；simple protocol 允许文件内多语句）
	t.Setenv("TUNELOOP_DB_SIMPLE_PROTOCOL", "1")
	db, err := database.InitDB(database.LoadConfig())
	require.NoError(t, err, "test database unavailable")
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS member_removal_requests").Error)

	apply := func(sql string) {
		for _, stmt := range strings.Split(sql, ";") {
			var lines []string
			for _, line := range strings.Split(stmt, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "--") {
					continue
				}
				lines = append(lines, line)
			}
			cleaned := strings.TrimSpace(strings.Join(lines, "\n"))
			if cleaned == "" {
				continue
			}
			require.NoError(t, db.Exec(cleaned).Error, "statement failed: %s", cleaned)
		}
	}
	tableExists := func() bool {
		var n int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'member_removal_requests'`).Scan(&n).Error)
		return n > 0
	}
	indexExists := func() bool {
		var n int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'member_removal_requests' AND indexname = 'uq_member_removal_pending_site_user'`).Scan(&n).Error)
		return n > 0
	}

	apply(string(upSQL))
	require.True(t, tableExists(), "up migration must create the table")
	require.True(t, indexExists(), "up migration must create the partial unique index")

	apply(string(downSQL))
	require.False(t, tableExists(), "down migration must drop the table")

	apply(string(upSQL))
	require.True(t, tableExists(), "second up must recreate the table")
	require.True(t, indexExists(), "second up must recreate the index")

	// 保留表供后续运行（handlers 其他用例依赖 AutoMigrate；此处不清理）
}
