package handlers

// #2261（P3 B3 批次3 前置）：orders/instruments 域在显式租户范围模式下的隔离守卫。
// 目标：证明该域读取（订单详情/我的订单、乐器列表/详情）不依赖 addTenantScope。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrdersInstrumentsDomain_ExplicitScopeIsolation_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgA, Name: "OI-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "OI-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	instA := uuid.NewString()
	instB := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{ID: instA, TenantID: orgA, OrgID: &orgA, SN: "OI-A-1", StockStatus: "available"}).Error)
	require.NoError(t, db.Create(&models.Instrument{ID: instB, TenantID: orgB, OrgID: &orgB, SN: "OI-B-1", StockStatus: "available"}).Error)

	// 订单：商户 A/B 各一张（订单详情/mine 列表）
	userA := uuid.NewString()
	require.NoError(t, db.Create(&models.User{ID: userA, IAMSub: userA, TenantID: orgA, OrgID: orgA, Status: "active", Name: "A用户"}).Error)
	orderA := models.Order{ID: uuid.NewString(), TenantID: orgA, OrgID: orgA, UserID: userA, InstrumentID: instA, Level: "entry", LeaseTerm: 30, Status: "in_lease", CashPaid: models.FromYuan(3500)}
	orderB := models.Order{ID: uuid.NewString(), TenantID: orgB, OrgID: orgB, UserID: uuid.NewString(), InstrumentID: instB, Level: "entry", LeaseTerm: 30, Status: "in_lease", CashPaid: models.FromYuan(3500)}
	require.NoError(t, db.Create(&orderA).Error)
	require.NoError(t, db.Create(&orderB).Error)

	// 商户 A 管理员 + 显式模式（关闭自动租户过滤）
	adminA := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
	withExplicit := func(c *gin.Context) {
		ctx := database.ExplicitTenantScopeOnly(adminA.InjectContext(c.Request.Context()))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}

	// 1) 订单详情：B 的单 → 404；A 的单 → 200
	r := gin.New()
	r.Use(withExplicit)
	r.GET("/orders/:id", GetOrder)
	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderB.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "显式模式下跨商户订单详情必须 404")

	req = httptest.NewRequest(http.MethodGet, "/orders/"+orderA.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "本商户订单详情必须可见")

	// 2) 乐器列表：仅 A 的乐器
	r2 := gin.New()
	r2.Use(withExplicit)
	r2.GET("/instruments", GetInstruments)
	req = httptest.NewRequest(http.MethodGet, "/instruments?page=1&pageSize=50", nil)
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listResp struct {
		Data struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	ids := map[string]bool{}
	for _, it := range listResp.Data.List {
		ids[it.ID] = true
	}
	assert.True(t, ids[instA], "本商户乐器必须可见")
	assert.False(t, ids[instB], "显式模式下跨商户乐器不得可见")

}
