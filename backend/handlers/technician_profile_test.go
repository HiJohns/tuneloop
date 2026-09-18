package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1974 T1：师傅档案（直属商户）+ 顾客侧列表/详情 + 活跃会话计数。

type tpFixture struct {
	tenantID, siteID string
	techID, adminSub string
	router           *gin.Engine
}

func setupTPFixture(t *testing.T) tpFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	siteID := uuid.New().String()
	techID := uuid.New().String()
	adminSub := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "S"}).Error)
	for _, u := range []string{techID, adminSub} {
		require.NoError(t, db.Create(&models.User{
			ID: u, IAMSub: u, TenantID: tenantID, OrgID: tenantID,
			Username: "u-" + u[:8], Name: "师傅" + u[:4], Status: "active",
		}).Error)
	}
	h := NewTechnicianProfileHandler()
	r := gin.New()
	r.GET("/technician-profiles", h.List)
	r.POST("/technician-profiles", h.Create)
	r.PUT("/technician-profiles/:id", h.Update)
	r.PUT("/technician-profiles/:id/status", h.SetStatus)
	r.GET("/common/repair-technicians", h.PublicList)
	r.GET("/common/repair-technicians/:id", h.PublicGet)
	r.GET("/common/repair-technicians/active-session-count", h.ActiveSessionCount)
	return tpFixture{tenantID: tenantID, siteID: siteID, techID: techID, adminSub: adminSub, router: r}
}

func tpDo(t *testing.T, f tpFixture, method, path string, body interface{}, actor testutil.TestActor) (int, map[string]interface{}) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ctx := actor.InjectContext(req.Context())
	f.router.ServeHTTP(w, req.WithContext(ctx))
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// CRUD：创建（含照片/介绍/经验）→ 更新 → 停用 → 列表按状态过滤
func TestTechnicianProfile_CRUD(t *testing.T) {
	f := setupTPFixture(t)
	admin := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.adminSub, Role: "ADMIN"}

	// 创建
	code, resp := tpDo(t, f, http.MethodPost, "/technician-profiles", gin.H{
		"user_id": f.techID, "photo": "photo.jpg", "bio": "钢琴维修 12 年 · 小提琴维修 8 年",
		"experience": []map[string]interface{}{{"craft": "钢琴", "years": 12}, {"craft": "小提琴", "years": 8}},
	}, admin)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	profileID := resp["data"].(map[string]interface{})["id"].(string)

	// 重复创建 → 409
	code, resp = tpDo(t, f, http.MethodPost, "/technician-profiles", gin.H{"user_id": f.techID}, admin)
	assert.Equal(t, http.StatusConflict, code)

	// 更新
	code, resp = tpDo(t, f, http.MethodPut, "/technician-profiles/"+profileID, gin.H{
		"bio": "更新后的介绍",
	}, admin)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)

	// 列表（active）
	code, resp = tpDo(t, f, http.MethodGet, "/technician-profiles", nil, admin)
	require.Equal(t, http.StatusOK, code)
	list := resp["data"].(map[string]interface{})["list"].([]interface{})
	require.Len(t, list, 1)
	row := list[0].(map[string]interface{})
	assert.Equal(t, "更新后的介绍", row["bio"])
	assert.NotEmpty(t, row["name"], "JOIN users 取姓名")
	assert.Equal(t, "active", row["status"])

	// 停用 → 列表 active 为空
	code, resp = tpDo(t, f, http.MethodPut, "/technician-profiles/"+profileID+"/status", gin.H{"status": "inactive"}, admin)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	code, resp = tpDo(t, f, http.MethodGet, "/technician-profiles?status=active", nil, admin)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, resp["data"].(map[string]interface{})["list"].([]interface{}), 0)
}

// 字段/越权校验
func TestTechnicianProfile_Validation(t *testing.T) {
	f := setupTPFixture(t)
	admin := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.adminSub, Role: "ADMIN"}

	// 不存在的 user → 40003
	code, _ := tpDo(t, f, http.MethodPost, "/technician-profiles", gin.H{"user_id": uuid.New().String()}, admin)
	assert.Equal(t, http.StatusBadRequest, code)

	// 非法 status → 40002
	code, _ = tpDo(t, f, http.MethodPut, "/technician-profiles/"+uuid.New().String()+"/status", gin.H{"status": "xx"}, admin)
	assert.Equal(t, http.StatusBadRequest, code)

	// 他租户不可见/不可改 → 404
	other := testutil.TestActor{TenantID: uuid.New().String(), OrgID: uuid.New().String(), UserID: uuid.New().String(), Role: "ADMIN"}
	code, _ = tpDo(t, f, http.MethodPut, "/technician-profiles/"+uuid.New().String(), gin.H{"bio": "x"}, other)
	assert.Equal(t, http.StatusNotFound, code)

	// 无租户 → 403
	code, _ = tpDo(t, f, http.MethodGet, "/technician-profiles", nil, testutil.TestActor{Role: "USER"})
	assert.Equal(t, http.StatusForbidden, code)
}

// 顾客侧：空 tid/oid 可读；仅 active；含档案字段；不含手机号/邮箱
func TestTechnicianProfile_PublicListAndGet(t *testing.T) {
	f := setupTPFixture(t)
	admin := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.adminSub, Role: "ADMIN"}
	code, resp := tpDo(t, f, http.MethodPost, "/technician-profiles", gin.H{
		"user_id": f.techID, "photo": "p.jpg", "bio": "b", "experience": []map[string]interface{}{{"craft": "钢琴", "years": 12}},
	}, admin)
	require.Equal(t, http.StatusOK, code)
	profileID := resp["data"].(map[string]interface{})["id"].(string)

	customer := testutil.TestActor{TenantID: "", OrgID: "", UserID: uuid.New().String(), Role: "USER"}
	code, resp = tpDo(t, f, http.MethodGet, "/common/repair-technicians", nil, customer)
	require.Equal(t, http.StatusOK, code, "顾客上下文（空 tid/oid）应可读")
	list := resp["data"].(map[string]interface{})["list"].([]interface{})
	require.Len(t, list, 1)
	row := list[0].(map[string]interface{})
	assert.Equal(t, f.techID, row["technician_id"], "technician_id = users.id")
	assert.NotEmpty(t, row["name"])
	assert.Equal(t, "p.jpg", row["avatar"])
	assert.NotContains(t, row, "phone", "不暴露手机号")
	assert.NotContains(t, row, "email", "不暴露邮箱")

	// 详情
	code, resp = tpDo(t, f, http.MethodGet, "/common/repair-technicians/"+f.techID, nil, customer)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, f.techID, resp["data"].(map[string]interface{})["technician_id"])

	// 停用后顾客侧不可见
	code, resp = tpDo(t, f, http.MethodPut, "/technician-profiles/"+profileID+"/status", gin.H{"status": "inactive"}, admin)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	code, resp = tpDo(t, f, http.MethodGet, "/common/repair-technicians", nil, customer)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, resp["data"].(map[string]interface{})["list"].([]interface{}), 0, "停用后不出现在顾客侧")
}

// 活跃会话计数：非 closed 计；他师傅不计；未登录 401
func TestTechnicianProfile_ActiveSessionCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	techID := uuid.New().String()
	otherTech := uuid.New().String()
	require.NoError(t, db.Create(&models.User{ID: techID, IAMSub: techID, TenantID: tenantID, OrgID: tenantID, Username: "t1", Name: "师傅A", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.User{ID: otherTech, IAMSub: otherTech, TenantID: tenantID, OrgID: tenantID, Username: "t2", Name: "师傅B", Status: "active"}).Error)

	// 3 单：pending_quote（活跃）、repairing（活跃）、closed（不计）；另有他师傅 1 单
	for i, st := range []string{"pending_quote", "repairing", "closed"} {
		require.NoError(t, db.Create(&models.RepairRequest{
			ID: uuid.New().String(), UserID: uuid.New().String(), UserInstrumentID: uuid.New().String(),
			SiteID: uuid.New().String(), Type: "service", TechnicianID: &techID, TenantID: tenantID, Status: st,
		}).Error)
		_ = i
	}
	require.NoError(t, db.Create(&models.RepairRequest{
		ID: uuid.New().String(), UserID: uuid.New().String(), UserInstrumentID: uuid.New().String(),
		SiteID: uuid.New().String(), Type: "service", TechnicianID: &otherTech, TenantID: tenantID, Status: "repairing",
	}).Error)

	h := NewTechnicianProfileHandler()
	r := gin.New()
	r.GET("/common/repair-technicians/active-session-count", h.ActiveSessionCount)

	// 师傅本人 → 2（排除 closed 与他师傅）
	me := testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: techID, Role: "STAFF"}
	req := httptest.NewRequest(http.MethodGet, "/common/repair-technicians/active-session-count", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req.WithContext(me.InjectContext(req.Context())))
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Count int64 `json:"count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, w.Body.String())
	assert.Equal(t, int64(2), resp.Data.Count)

	// 未登录 → 401
	req2 := httptest.NewRequest(http.MethodGet, "/common/repair-technicians/active-session-count", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	var resp2 struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	assert.Equal(t, 40100, resp2.Code)
}
