package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// #1860 regression tests: StaffRefundOrder 站点隔离改「乐器归属可见性桥」。
// 订单 OrgID 继承租户级乐器 OrgID，与网点员工 JWT oid（sites.org_id）结构性
// 不等——旧严格相等校验把网点员工的合法退款拦成 403（84ab8ebd 实证）。

// create1860Fixtures 建站点桥数据并返回 (db, tenantID, orderID)。
// siteOrgID 同时作为员工 JWT oid 与 site.org_id（T1 桥命中）；T2 用不同 oid。
func create1860Fixtures(t *testing.T, siteOrgID string) (db *gorm.DB, tenantID, orderID string) {
	t.Helper()
	db = testfixtures.SetupTestDB(t)

	tenantID = uuid.New().String()
	siteRowID := uuid.New().String()
	siteRowUUID := uuid.MustParse(siteRowID)
	userID := uuid.New().String()
	instID := uuid.New().String()

	require.NoError(t, db.Create(&models.Site{
		ID: siteRowID, TenantID: tenantID, OrgID: siteOrgID,
		Name: "1860-站点", Status: "active",
	}).Error)
	instOrg := tenantID // 乐器建在租户级 org（order.OrgID 继承它——病灶）
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &instOrg,
		SiteID: &siteRowUUID, // 乐器实际所在网点
		SN:     "SN-1860-" + uuid.New().String()[:6], StockStatus: "rented",
	}).Error)

	orderID = uuid.New().String()
	order := models.Order{
		ID: orderID, TenantID: tenantID, OrgID: tenantID,
		UserID: userID, InstrumentID: instID,
		Status:  models.OrderStatusDepositRefunding,
		Deposit: 0, CashPaid: models.FromYuan(0.72), ShippingFee: models.FromYuan(0.01),
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
			"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
			"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
	}
	require.NoError(t, db.Create(&order).Error)
	days1 := 1
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		TenantID: tenantID, UserID: userID, OrderID: &order.ID,
		OrderType: "renewal", Type: "payment", Status: "paid",
		Amount: models.FromYuan(0.36), Days: &days1,
	}).Error)
	return db, tenantID, orderID
}

// refund1860 以 site_member（oid=staffOID）身份调用员工退款。
func refund1860(t *testing.T, tenantID, staffOID, orderID string) (httpCode, bizCode int) {
	t.Helper()
	staff := testutil.MakeSiteMember(tenantID, staffOID, uuid.New().String())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := staff.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/orders/:id/refund", (&UserSettlementHandler{}).StaffRefundOrder)

	req := httptest.NewRequest("POST", "/orders/"+orderID+"/refund", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Code
}

// TestStaffRefundOrder_SiteBridgeAllowsTenantOrgOrder (#1860 T1): 乐器挂租户
// org（order.OrgID=租户）但所在网点的 org_id=员工 oid → 站点桥命中，放行。
func TestStaffRefundOrder_SiteBridgeAllowsTenantOrgOrder(t *testing.T) {
	siteOrgID := uuid.New().String()
	db, tenantID, orderID := create1860Fixtures(t, siteOrgID)

	httpCode, bizCode := refund1860(t, tenantID, siteOrgID, orderID)
	require.Equal(t, http.StatusOK, httpCode)
	require.Equal(t, 20000, bizCode)

	var after models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
	require.Equal(t, models.OrderStatusCompleted, after.Status, "退款执行 → 订单关单")
}

// TestStaffRefundOrder_ForeignSiteRejected (#1860 T2): 乐器所在网点 org 不在
// 员工可见集（另一网点的 oid）→ 403。
func TestStaffRefundOrder_ForeignSiteRejected(t *testing.T) {
	siteOrgID := uuid.New().String()
	_, tenantID, orderID := create1860Fixtures(t, siteOrgID)

	httpCode, bizCode := refund1860(t, tenantID, uuid.New().String(), orderID)
	require.Equal(t, http.StatusForbidden, httpCode)
	require.Equal(t, 40300, bizCode)
}

// TestStaffRefundOrder_DirectOrgHitCompatible (#1860 T3 兼容): 订单 OrgID 直接
// 等于员工 oid（乐器挂网点 org 的老布局）→ 放行。
func TestStaffRefundOrder_DirectOrgHitCompatible(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	siteOrgID := uuid.New().String()
	userID := uuid.New().String()
	instID := uuid.New().String()
	instOrg := siteOrgID // 乐器直接挂员工 org
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &instOrg,
		SN: "SN-1860C-" + uuid.New().String()[:6], StockStatus: "rented",
	}).Error)
	orderID := uuid.New().String()
	order := models.Order{
		ID: orderID, TenantID: tenantID, OrgID: siteOrgID,
		UserID: userID, InstrumentID: instID,
		Status:  models.OrderStatusDepositRefunding,
		Deposit: 0, CashPaid: models.FromYuan(0.72), ShippingFee: models.FromYuan(0.01),
	}
	require.NoError(t, db.Create(&order).Error)

	httpCode, bizCode := refund1860(t, tenantID, siteOrgID, orderID)
	require.Equal(t, http.StatusOK, httpCode)
	require.Equal(t, 20000, bizCode)

	var after models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
	require.Equal(t, models.OrderStatusCompleted, after.Status, "退款执行 → 订单关单")
}
