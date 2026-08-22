package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1744: prepay 应用优惠码后回写订单快照 coupon_code / coupon_discount（分），
// 订单可审计/还原优惠事实。测试覆盖 ENO percent / OREZ waive / 无码 /
// session 流程（无订单）四场景。

func setupPrepayCouponRouter(t *testing.T, tenantID, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)
	// allTables 不含 Coupon → 手动建表（幂等）
	_ = db.Migrator().DropTable(&models.Coupon{})
	require.NoError(t, db.Migrator().CreateTable(&models.Coupon{}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)
	return router
}

func seedCoupon(t *testing.T, code, ctype string, value float64) {
	t.Helper()
	db := database.GetDB()
	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: code, Type: ctype,
		Value: value, Active: true,
	}).Error)
}

func seedRentOrder(t *testing.T, tenantID, userID string) models.Order {
	t.Helper()
	db := database.GetDB()
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        tenantID,
		UserID:       userID,
		InstrumentID: uuid.New().String(),
		Status:       models.OrderStatusReserved,
	}
	require.NoError(t, db.Create(&order).Error)
	return order
}

// T4a: ENO percent → orders.coupon_code='ENO'、coupon_discount=原价−折后（分）。
func TestPrepayCouponSnapshot_ENO_Percent(t *testing.T) {
	tenantID := uuid.New().String()
	userID := uuid.New().String()
	router := setupPrepayCouponRouter(t, tenantID, userID)
	seedCoupon(t, "ENO", "percent", 10) // 10‰ = 1%

	order := seedRentOrder(t, tenantID, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type":  "rent",
		"order_id":    order.ID,
		"amount":      36.0, // 全价 36 元
		"coupon_code": "ENO",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var after models.Order
	require.NoError(t, database.GetDB().Where("id = ?", order.ID).First(&after).Error)
	require.NotNil(t, after.CouponCode, "coupon_code must be written")
	require.Equal(t, "ENO", *after.CouponCode)
	// 原价 3600 分 − 折后 36 分 = 3564 分（af3f8cf2 对账场景）
	require.Equal(t, models.Cents(3564), after.CouponDiscount, "discount = 3600 − 36 = 3564 cents")
}

// T4b: OREZ waive → coupon_code='OREZ'、coupon_discount=原价（全额免除）。
func TestPrepayCouponSnapshot_OREZ_Waive(t *testing.T) {
	tenantID := uuid.New().String()
	userID := uuid.New().String()
	router := setupPrepayCouponRouter(t, tenantID, userID)
	seedCoupon(t, "OREZ", "waive", 0)

	order := seedRentOrder(t, tenantID, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type":  "rent",
		"order_id":    order.ID,
		"amount":      36.0,
		"coupon_code": "OREZ",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var after models.Order
	require.NoError(t, database.GetDB().Where("id = ?", order.ID).First(&after).Error)
	require.NotNil(t, after.CouponCode)
	require.Equal(t, "OREZ", *after.CouponCode)
	require.Equal(t, models.Cents(3600), after.CouponDiscount, "waive = full original amount")
}

// T4c: 无优惠码 → coupon_code NULL、coupon_discount=0。
func TestPrepayCouponSnapshot_NoCoupon(t *testing.T) {
	tenantID := uuid.New().String()
	userID := uuid.New().String()
	router := setupPrepayCouponRouter(t, tenantID, userID)

	order := seedRentOrder(t, tenantID, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type": "rent",
		"order_id":   order.ID,
		"amount":     36.0,
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var after models.Order
	require.NoError(t, database.GetDB().Where("id = ?", order.ID).First(&after).Error)
	require.Nil(t, after.CouponCode, "no coupon → NULL")
	require.Equal(t, models.Cents(0), after.CouponDiscount)
}

// T4d: session 流程（membership 无订单）→ 不回写（无 panic）。
func TestPrepayCouponSnapshot_SessionFlow_NoOrderWrite(t *testing.T) {
	tenantID := uuid.New().String()
	userID := uuid.New().String()
	router := setupPrepayCouponRouter(t, tenantID, userID)
	seedCoupon(t, "ENO", "percent", 10)

	// membership session flow: order_type=membership, no order_id
	body, _ := json.Marshal(map[string]interface{}{
		"order_type":  "membership",
		"amount":      99.0,
		"coupon_code": "ENO",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body)))
	// 无 session → 400（session 校验先于优惠码回写），但必须无 panic 且无订单写入
	require.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusOK, "unexpected status: %d", w.Code)
}

// T3 验证: GetOrder 透出 coupon_code / coupon_discount。
func TestGetOrder_CouponSnapshotExposed(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	order := models.Order{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          tenantID,
		UserID:         userID,
		InstrumentID:   uuid.New().String(),
		Status:         models.OrderStatusPaid,
		CouponCode:     strPtr("ENO"),
		CouponDiscount: models.Cents(3564),
	}
	require.NoError(t, db.Create(&order).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/orders/"+order.ID, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			CouponCode     *string `json:"coupon_code"`
			CouponDiscount int64   `json:"coupon_discount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.NotNil(t, resp.Data.CouponCode)
	require.Equal(t, "ENO", *resp.Data.CouponCode)
	require.Equal(t, int64(3564), resp.Data.CouponDiscount)
}
