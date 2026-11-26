package handlers

// #2519：批量上架/下架端点（新增 `PUT /instruments/batch/status`）+ 单条状态去 DebugMode。
//
// 缺陷：前端 `/instruments/list` 的「批量上架/下架」调用 `/instruments/batch/status`，
// 后端从来没有该路由（恒 404）；单条 `PUT /instruments/:id/status` 又挂
// `RequireDebugMode()`，生产不可用。
//
// Evidence rows: R1（3 条成功）、R2（跨租户 + 无权限）、R3（单条生产可用）、R4（部分失败）。
// Token lines use fmt.Printf so scripts/verify-2519.sh can grep them from `go test -v`.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// cusPermRegistry2519 emulates the production registry (instrument:update = bit 2).
type cusPermRegistry2519 struct{}

func (cusPermRegistry2519) GetSysPermBit(string) (int, bool) { return -1, false }
func (cusPermRegistry2519) GetCusPermBit(string) int         { return 2 }
func (cusPermRegistry2519) GetCusPermMapping() map[string]int {
	return map[string]int{"instrument:update": 2}
}

func flag2519(cond bool) int {
	if cond {
		return 1
	}
	return 0
}

type batchStatusResp2519 struct {
	status int
	Code   int `json:"code"`
	Data   struct {
		Updated int `json:"updated"`
		Failed  []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"failed"`
	} `json:"data"`
}

func serveBatchStatus2519(t *testing.T, r *gin.Engine, payload interface{}) batchStatusResp2519 {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/instruments/batch/status", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out batchStatusResp2519
	out.status = w.Code
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

// batchRouter2519 装配与生产同形的中间件链（ExplicitTenantScope + RequireCusPerm），
// 但**不含** RequireDebugMode（R3 的运行时一面；静态一面由 verify 脚本读 main.go）。
func batchRouter2519(actor testutil.TestActor, cusPerm int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		a := actor
		a.CusPerm = cusPerm
		c.Request = c.Request.WithContext(a.InjectContext(c.Request.Context()))
		c.Next()
	}, middleware.ExplicitTenantScope(), middleware.RequireCusPerm("instrument:update"))
	r.PUT("/instruments/batch/status", BatchUpdateInstrumentStatus)
	r.PUT("/instruments/:id/status", UpdateInstrumentStatus)
	return r
}

func seedInstrument2519(t *testing.T, db *gorm.DB, tenantID, sn, status string) string {
	t.Helper()
	id := uuid.New().String()
	org := tenantID
	require.NoError(t, db.Create(&models.Instrument{
		ID: id, TenantID: tenantID, OrgID: &org, SN: sn, StockStatus: status,
	}).Error)
	return id
}

// R1：3 条合法状态全部落库，响应 updated=3 / failed=[]。
func TestBatchUpdateInstrumentStatus_Success_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	ids := []string{
		seedInstrument2519(t, db, tenantID, "BAT-2519-A", "available"),
		seedInstrument2519(t, db, tenantID, "BAT-2519-B", "available"),
		seedInstrument2519(t, db, tenantID, "BAT-2519-C", "available"),
	}
	actor := testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"}
	r := batchRouter2519(actor, -1)

	resp := serveBatchStatus2519(t, r, map[string]interface{}{"ids": ids, "status": "maintenance"})
	require.Equal(t, http.StatusOK, resp.status)
	require.EqualValues(t, 20000, resp.Code)

	changed := 0
	for _, id := range ids {
		var row models.Instrument
		require.NoError(t, db.Where("id = ?", id).First(&row).Error)
		if row.StockStatus == "maintenance" {
			changed++
		}
	}

	fmt.Printf("R1 STATUS=%d UPDATED=%d FAILED=%d DB_CHANGED=%d\n",
		resp.Code, resp.Data.Updated, len(resp.Data.Failed), changed)
	require.Equal(t, 3, resp.Data.Updated)
	require.Equal(t, 0, len(resp.Data.Failed))
	require.Equal(t, 3, changed)
}

// R2：他租户 id 被拒且不改写；无 instrument:update 权限 → 403。
func TestBatchUpdateInstrumentStatus_CrossTenantAndPermission_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantA, tenantB := uuid.New().String(), uuid.New().String()
	ownID := seedInstrument2519(t, db, tenantA, "BAT-2519-OWN", "available")
	foreignID := seedInstrument2519(t, db, tenantB, "BAT-2519-FOREIGN", "available")

	actor := testutil.TestActor{TenantID: tenantA, OrgID: tenantA, UserID: uuid.New().String(), Role: "ADMIN"}
	r := batchRouter2519(actor, -1)

	resp := serveBatchStatus2519(t, r, map[string]interface{}{"ids": []string{ownID, foreignID}, "status": "archived"})
	require.Equal(t, http.StatusOK, resp.status)

	var foreign models.Instrument
	require.NoError(t, db.Where("id = ?", foreignID).First(&foreign).Error)
	blocked := foreign.StockStatus == "available"

	// CusPerm 只置 bit0（instrument:update = bit2）→ 403 40305。默认 mock registry 对
	// cus_perm 返回 -1（放行），因此这里临时装上真实位图（#1964 stub 同型）以证明门控生效。
	prevRegistry := middleware.PermissionRegistry
	middleware.PermissionRegistry = cusPermRegistry2519{}
	defer func() { middleware.PermissionRegistry = prevRegistry }()

	deniedRouter := batchRouter2519(actor, 1)
	denied := serveBatchStatus2519(t, deniedRouter, map[string]interface{}{"ids": []string{ownID}, "status": "archived"})

	fmt.Printf("R2 STATUS=%d UPDATED=%d FAILED=%d CROSS_TENANT_BLOCKED=%d NO_PERM_STATUS=%d DB_UNCHANGED=%d\n",
		resp.Code, resp.Data.Updated, len(resp.Data.Failed), flag2519(blocked), denied.status, flag2519(blocked))
	require.Equal(t, 1, resp.Data.Updated)
	require.Equal(t, 1, len(resp.Data.Failed))
	require.Equal(t, foreignID, resp.Data.Failed[0].ID)
	require.Equal(t, "not_found_or_forbidden", resp.Data.Failed[0].Reason)
	require.True(t, blocked, "cross-tenant instrument must not be modified")
	require.Equal(t, http.StatusForbidden, denied.status)
}

// R4：混合合法/非法 id → failed[] 每项含 id+reason，成功项已落库。
func TestBatchUpdateInstrumentStatus_PartialFail_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	okID := seedInstrument2519(t, db, tenantID, "BAT-2519-OK", "available")
	missingID := uuid.New().String()
	actor := testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"}
	r := batchRouter2519(actor, -1)

	resp := serveBatchStatus2519(t, r, map[string]interface{}{"ids": []string{okID, missingID}, "status": "maintenance"})
	require.Equal(t, http.StatusOK, resp.status)

	var okRow models.Instrument
	require.NoError(t, db.Where("id = ?", okID).First(&okRow).Error)
	reasonPresent := len(resp.Data.Failed) == 1 && resp.Data.Failed[0].ID == missingID && resp.Data.Failed[0].Reason != ""
	changed := okRow.StockStatus == "maintenance"

	fmt.Printf("R4 STATUS=%d UPDATED=%d FAILED=%d REASON_PRESENT=%d DB_CHANGED=%d\n",
		resp.Code, resp.Data.Updated, len(resp.Data.Failed), flag2519(reasonPresent), flag2519(changed))
	require.Equal(t, 1, resp.Data.Updated)
	require.Equal(t, 1, len(resp.Data.Failed))
	require.True(t, reasonPresent)
	require.True(t, changed)
}

// R3（运行时一面）：单条状态在**无 RequireDebugMode** 的中间件链上可用。
func TestUpdateInstrumentStatusRoute_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.New().String()
	org := tenantID
	id := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{ID: id, TenantID: tenantID, OrgID: &org, SN: "ONE-2519", StockStatus: "available"}).Error)

	actor := testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"}
	r := batchRouter2519(actor, -1)

	raw, _ := json.Marshal(map[string]interface{}{"stock_status": "maintenance"})
	req := httptest.NewRequest(http.MethodPut, "/instruments/"+id+"/status", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	fmt.Printf("R3 STATUS=%d HTTP=%d\n", resp.Code, w.Code)
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 20000, resp.Code)
}
