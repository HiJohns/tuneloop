package handlers

// #1934: 转发会话分段/费用模型 + 生命周期测试
// （docs/cases/transit.md v1：承担矩阵——顾客承担 ①②③，商户承担 ④）

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// TestForwardingSessionLifecycleAndFees — outbound 全生命周期 + 分段①②计费
func TestForwardingSessionLifecycleAndFees(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1934ab0c000a")

	orderID := uuid.New().String()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, user_id, instrument_id, status, level, lease_term, monthly_rent, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'paid', '入门', 30, 10000, now(), now())`,
		orderID, tenantID, uuid.New().String(), uuid.New().String()).Error)

	session := seed1934Session(t, db, tenantID, orgID, orderID, models.ForwardingDirectionOutbound, models.ForwardingStatusPending)

	router := new1934RouterWith(tenantID, orgID)

	// ① 受控→中转：ship → in_transit + 记分段①（顾客承担，物流三件套留痕）
	w := put1934(router, "/api/forwarding/sessions/"+session.ID+"/ship", map[string]interface{}{
		"tracking_company": "顺丰",
		"tracking_number":  "SF001",
		"shipping_fee":     10.0,
	})
	require.Equal(t, 20000, code1934(w), w.Body.String())
	s1 := get1934Session(t, db, session.ID)
	assert.Equal(t, models.ForwardingStatusInTransit, s1.Status)
	assert.Equal(t, "顺丰", s1.TrackingCompany)
	assert.Equal(t, "SF001", s1.TrackingNumber)
	require1934Fee(t, db, orderID, TransitSegmentOutboundControlledToTransit, "outbound", 1000, "customer")

	// 收货（拍照留痕）→ received
	w = put1934(router, "/api/forwarding/sessions/"+session.ID+"/receive", map[string]interface{}{
		"photo_keys": []string{"/uploads/media/f1.jpg", "/uploads/media/f2.jpg"},
	})
	require.Equal(t, 20000, code1934(w), w.Body.String())
	s2 := get1934Session(t, db, session.ID)
	assert.Equal(t, models.ForwardingStatusReceived, s2.Status)
	require.NotNil(t, s2.Photos)
	assert.Contains(t, *s2.Photos, "/uploads/media/f1.jpg")

	// ready（等待转发）
	w = put1934(router, "/api/forwarding/sessions/"+session.ID+"/ready", map[string]interface{}{})
	require.Equal(t, 20000, code1934(w), w.Body.String())

	// ② 中转→顾客：last-mile → last_mile + 分段②（顾客承担）
	w = put1934(router, "/api/forwarding/sessions/"+session.ID+"/last-mile", map[string]interface{}{
		"tracking_company": "EMS",
		"tracking_number":  "E001",
		"logistics_fee":    12.5,
	})
	require.Equal(t, 20000, code1934(w), w.Body.String())
	s3 := get1934Session(t, db, session.ID)
	assert.Equal(t, models.ForwardingStatusLastMile, s3.Status)
	require1934Fee(t, db, orderID, TransitSegmentOutboundTransitToCustomer, "outbound", 1250, "customer")
	// audit #1934 Bug3: 会话级 logistics_fee_cents 与 tracking 三件套同源接线
	assert.Equal(t, int64(1250), int64(s3.LogisticsFeeCents))

	// complete
	w = put1934(router, "/api/forwarding/sessions/"+session.ID+"/complete", map[string]interface{}{})
	require.Equal(t, 20000, code1934(w), w.Body.String())
	s4 := get1934Session(t, db, session.ID)
	assert.Equal(t, models.ForwardingStatusCompleted, s4.Status)
}

// TestLastMileReturnFeesMerchantPays — ④ 中转→受控（商户承担）
func TestLastMileReturnFeesMerchantPays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1934ab1d000a")

	orderID := uuid.New().String()
	session := seed1934Session(t, db, tenantID, orgID, orderID, models.ForwardingDirectionReturn, models.ForwardingStatusReady)

	router := new1934RouterWith(tenantID, orgID)
	w := put1934(router, "/api/forwarding/sessions/"+session.ID+"/last-mile", map[string]interface{}{
		"tracking_company": "德邦",
		"tracking_number":  "DB001",
		"logistics_fee":    8.0,
	})
	require.Equal(t, 20000, code1934(w), w.Body.String())

	var s models.ForwardingSession
	require.NoError(t, db.Where("id = ?", session.ID).First(&s).Error)
	assert.Equal(t, models.ForwardingStatusLastMile, s.Status)
	require1934Fee(t, db, orderID, TransitSegmentReturnTransitToControlled, "return", 800, "merchant")

	// 聚合语义：paid_by 口径 —— 顾客合计不含 ④
	// audit #1934 Bug2: 列名为 amount（amount_cents 不存在）且必须检查 Scan 错误
	var custSum int64
	require.NoError(t, db.Table("transit_shipping_fees").
		Select("COALESCE(SUM(amount),0)").
		Where("order_id = ? AND paid_by = ?", orderID, "customer").
		Scan(&custSum).Error)
	assert.Equal(t, int64(0), custSum)
}

// TestRecordTransitFeeIdempotentUpdate — 同 order 同 segment 重复记录更新金额不新增行
func TestRecordTransitFeeIdempotentUpdate(t *testing.T) {
	testfixtures.SetupTestDB(t)
	db := database.GetDB()
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1934ab2e000a")

	orderID := uuid.New().String()
	session := seed1934Session(t, db, tenantID, orgID, orderID, models.ForwardingDirectionOutbound, models.ForwardingStatusPending)

	recordTransitFee(db, session, TransitSegmentOutboundControlledToTransit, 10, "staff-1")
	var fee models.TransitShippingFee
	require.NoError(t, db.Where("order_id = ? AND segment = ?", orderID, TransitSegmentOutboundControlledToTransit).First(&fee).Error)
	assert.Equal(t, int64(1000), int64(fee.Amount))

	recordTransitFee(db, session, TransitSegmentOutboundControlledToTransit, 20, "staff-1")
	var count int64
	db.Model(&models.TransitShippingFee{}).Where("order_id = ? AND segment = ?", orderID, TransitSegmentOutboundControlledToTransit).Count(&count)
	assert.Equal(t, int64(1), count, "幂等：不得新增行")

	var fee2 models.TransitShippingFee
	require.NoError(t, db.Where("order_id = ? AND segment = ?", orderID, TransitSegmentOutboundControlledToTransit).First(&fee2).Error)
	assert.Equal(t, int64(2000), int64(fee2.Amount))
}

// TestSessionSearchFilters — GET /forwarding/sessions 支持 order_id/session_code/status 过滤
func TestSessionSearchFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupTestDB(t)
	db := database.GetDB()
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1934ab3f000a")

	orderID := uuid.New().String()
	session := seed1934Session(t, db, tenantID, orgID, orderID, models.ForwardingDirectionOutbound, models.ForwardingStatusPending)

	router := new1934RouterWith(tenantID, orgID)

	// by session_code
	w := get1934(router, "/api/forwarding/sessions?session_code="+session.SessionCode)
	require.Equal(t, 20000, code1934(w))
	var r1 struct {
		Data struct {
			List []models.ForwardingSession `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r1))
	require.Len(t, r1.Data.List, 1)
	assert.Equal(t, orderID, r1.Data.List[0].OrderID)

	// by order_id
	w = get1934(router, "/api/forwarding/sessions?order_id="+orderID)
	require.Equal(t, 20000, code1934(w), w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r1))
	require.Len(t, r1.Data.List, 1)

	// by status
	w = get1934(router, "/api/forwarding/sessions?status=pending")
	require.Equal(t, 20000, code1934(w), w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r1))
	for _, s := range r1.Data.List {
		assert.Equal(t, models.ForwardingStatusPending, s.Status)
	}
}

func get1934(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// seed1934Session creates a forwarding session row directly.
func seed1934Session(t *testing.T, db *gorm.DB, tenantID, orgID, orderID, direction, status string) models.ForwardingSession {
	t.Helper()
	session := models.ForwardingSession{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		OrgID:            orgID,
		LeaseSessionID:   uuid.New().String(),
		OrderID:          orderID,
		ForwardingSiteID: uuid.New().String(),
		MerchantID:       uuid.New().String(),
		Direction:        direction,
		Status:           status,
		SessionCode:      "19" + uuid.NewString()[:4], // 6 位短码（唯一即可）
		TrackingNumbers:  "[]",                        // jsonb 禁空串（22P02）
		InstrumentID:     uuid.New().String(),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func get1934Session(t *testing.T, db *gorm.DB, id string) models.ForwardingSession {
	t.Helper()
	var s models.ForwardingSession
	require.NoError(t, db.Where("id = ?", id).First(&s).Error)
	return s
}

func require1934Fee(t *testing.T, db *gorm.DB, orderID string, segment int, direction string, cents int64, paidBy string) {
	t.Helper()
	var fee models.TransitShippingFee
	err := db.Where("order_id = ? AND segment = ?", orderID, segment).First(&fee).Error
	require.NoError(t, err)
	assert.Equal(t, cents, int64(fee.Amount))
	assert.Equal(t, direction, fee.Direction)
	assert.Equal(t, paidBy, fee.PaidBy)
}

func new1934Router() *gin.Engine {
	return new1934RouterWith(g1934Tenant, g1934Org)
}

// new1934RouterWith builds the router bound to a tenant (test isolation):
// handler 们均为租户作用域（GetTenantID），必须注入 test actor。
func new1934RouterWith(tenantID, orgID string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: uuid.New().String(), Role: "STAFF"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.GET("/api/forwarding/sessions", ListForwardingSessions)
	router.PUT("/api/forwarding/sessions/:id/ship", ShipForwardingSession)
	router.PUT("/api/forwarding/sessions/:id/receive", ReceiveForwardingSession)
	router.PUT("/api/forwarding/sessions/:id/ready", ReadyForwardingSession)
	router.PUT("/api/forwarding/sessions/:id/last-mile", LastMileForwardingSession)
	router.PUT("/api/forwarding/sessions/:id/complete", CompleteForwardingSession)
	return router
}

func put1934(router *gin.Engine, path string, body map[string]interface{}) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

var (
	g1934Tenant, g1934Org string
)

func code1934(w *httptest.ResponseRecorder) int {
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		return -1
	}
	return resp.Code
}

// TestGetOrderLogisticsFeeTotalAggregation — 订单详情聚合（audit Bug2 验收项）：
// logistics_fee_total = ①+②+③（paid_by=customer），④（merchant）不入顾客口径。
func TestGetOrderLogisticsFeeTotalAggregation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1934ac1d000a")

	// 受控商户（GetMerchantTransitInfo 按 tenant_id 查 merchants）
	require.NoError(t, db.Exec(`INSERT INTO merchants (id, tenant_id, org_id, name, code, merchant_type, transit_address, transit_phone, created_at, updated_at)
		VALUES (?, ?, ?, '受控商户A', 'ctrl-1934a', 'controlled', '北京市中转路1号', '010-88880000', now(), now())`,
		uuid.New().String(), tenantID, tenantID).Error)

	ownerID := uuid.New().String()
	orderID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO orders (id, tenant_id, user_id, instrument_id, status, level, lease_term, monthly_rent, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'shipped', '入门', 30, 10000, now(), now())`,
		orderID, tenantID, ownerID, uuid.New().String()).Error)

	seedFee := func(segment int, direction, paidBy string, cents int64) {
		require.NoError(t, db.Exec(`INSERT INTO transit_shipping_fees (order_id, direction, segment, amount, paid_by, recorded_by, created_at)
			VALUES (?, ?, ?, ?, ?, 'staff', now())`, orderID, direction, segment, cents, paidBy).Error)
	}
	seedFee(1, "outbound", "customer", 1000) // ① 受控→中转
	seedFee(2, "outbound", "customer", 1250) // ② 中转→顾客
	seedFee(3, "return", "customer", 800)    // ③ 顾客→中转
	seedFee(4, "return", "merchant", 3000)   // ④ 中转→受控（商户承担）

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// caller ≠ 下单人（受控商户员工视角）→ 同时验证脱敏
		actor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: uuid.New().String(), Role: "STAFF"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.Equal(t, float64(3050), resp.Data["logistics_fee_total"], "①+②+③=3050；④(merchant) 不入顾客口径")
	assert.Equal(t, "合作商户", resp.Data["merchant_name"], "受控商户名占位")
	assert.Equal(t, "合作商户客户", resp.Data["user_name"], "受控员工不可见下单人")
}

// TestOutboundSessionAutoCreatedOnCreateOrder — audit Bug2 验收项：
// 受控商户下单 → 自动创建 direction=outbound 会话（此前仅 return 方向被创建）。
func TestOutboundSessionAutoCreatedOnCreateOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1934ac2e000a")

	// 受控商户
	require.NoError(t, db.Exec(`INSERT INTO merchants (id, tenant_id, org_id, name, code, merchant_type, created_at, updated_at)
		VALUES (?, ?, ?, '受控商户B', 'ctrl-1934b', 'controlled', now(), now())`,
		uuid.New().String(), tenantID, tenantID).Error)

	instrumentID := uuid.New().String()
	userID := uuid.New().String()
	now := time.Now()
	require.NoError(t, db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, email, phone, credit_score, is_shadow, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 600, false, ?, ?)`,
		userID, userID, tenantID, orgID, "Guest User", "guest1934@example.com", "13900139001", now, now).Error)
	require.NoError(t, db.Exec(`INSERT INTO instruments (id, tenant_id, org_id, site_id, level, stock_status, images, specifications, pricing, created_at, updated_at)
		VALUES (?, ?, ?, NULL, 'standard', 'available', '[]', '{}', '[]', ?, ?)`,
		instrumentID, tenantID, orgID, now, now).Error)

	defer func() {
		db.Exec(`DELETE FROM forwarding_sessions WHERE tenant_id = ?`, tenantID)
		db.Exec(`DELETE FROM orders WHERE tenant_id = ?`, tenantID)
		db.Exec(`DELETE FROM instruments WHERE id = ?`, instrumentID)
		db.Exec(`DELETE FROM users WHERE id = ?`, userID)
	}()

	router := setupGuestTestRouter(t, userID)
	handler := &UserRentalHandler{}
	router.POST("/user/orders", handler.CreateOrder)

	body := map[string]interface{}{
		"instrument_id": instrumentID,
		"start_date":    "2026-09-20",
		"end_date":      "2026-10-20",
		"delivery_address": map[string]interface{}{
			"city":    "Beijing",
			"address": "Chaoyang District",
		},
	}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/user/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var order models.Order
	require.NoError(t, db.Where("user_id = ? AND instrument_id = ?", userID, instrumentID).First(&order).Error)

	var sess models.ForwardingSession
	require.NoError(t, db.Where("order_id = ? AND direction = ?", order.ID, models.ForwardingDirectionOutbound).First(&sess).Error,
		"受控商户下单必须自动创建 outbound 会话")
	assert.Equal(t, models.ForwardingStatusPending, sess.Status)
	assert.Equal(t, tenantID, sess.TenantID)
}
