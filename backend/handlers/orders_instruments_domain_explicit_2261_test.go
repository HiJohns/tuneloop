package handlers

// #2261（P3 B3 批次3 前置）：orders/instruments 域在显式租户范围模式下的隔离守卫。
// 目标：证明该域读取（订单详情/我的订单、乐器列表/详情）不依赖 addTenantScope。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
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

	// 商户 A 管理员；对比「自动范围 on/off」两种模式（#2261 B3-B 路由级守卫）
	adminA := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}

	newRouter := func(t *testing.T, explicit bool) *gin.Engine {
		t.Helper()
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(adminA.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.GET("/orders/:id", GetOrder)
		r.GET("/instruments", GetInstruments)
		return r
	}

	run := func(t *testing.T, explicit bool) (int, int, map[string]bool) {
		t.Helper()
		r := newRouter(t, explicit)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/orders/"+orderB.ID, nil))
		crossCode := w.Code
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/orders/"+orderA.ID, nil))
		ownCode := w.Code

		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments?page=1&pageSize=50", nil))
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
		return crossCode, ownCode, ids
	}

	crossOff, ownOff, idsOff := run(t, false)
	crossOn, ownOn, idsOn := run(t, true)

	assert.Equal(t, http.StatusNotFound, crossOff, "自动范围：跨商户订单 404")
	assert.Equal(t, http.StatusOK, ownOff, "自动范围：本商户订单 200")
	assert.Equal(t, crossOff, crossOn, "开关 on/off 状态码必须等价（跨商户）")
	assert.Equal(t, ownOff, ownOn, "开关 on/off 状态码必须等价（本商户）")
	assert.Equal(t, http.StatusNotFound, crossOn, "显式模式：跨商户订单 404")
	assert.True(t, idsOff[instA] && idsOn[instA], "本商户乐器在两种模式下均可见")
	assert.False(t, idsOff[instB], "自动范围：跨商户乐器不可见")
	assert.False(t, idsOn[instB], "显式模式：跨商户乐器不可见")
	assert.Equal(t, idsOff, idsOn, "乐器列表 on/off 必须等价")
}
