package handlers

// #2259（B0 隔离守卫测试网）：把 A5 矩阵的 ISO-14/15 落为自动化机械测试。
// 语义口径（依 #2245 Epic「锚定 = 商户 org」）：
//   - 订单/乐器详情隔离按「商户 org」层级断言（跨商户 → 404）
//   - 站点级列表隔离由 #1911/#2116 现有测试覆盖（矩阵注释引用）
//   - 顾客零租户：下单不被拒 + 订单落商户 org + 本人可读 + 跨顾客负例（#1694 家族）

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
	"gorm.io/gorm"
)

type isoEnv struct {
	o1, o2 string
	subA   string
	instID string
}

func newIsoEnv(t *testing.T) (*gorm.DB, *gin.Engine, isoEnv) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	o1, o2 := uuid.NewString(), uuid.NewString()
	t1, t2 := uuid.NewString(), uuid.NewString()
	orgS1, orgS2 := uuid.NewString(), uuid.NewString()

	mkMerchant := func(tenant, org, name string) {
		require.NoError(t, db.Create(&models.Merchant{
			ID: uuid.NewString(), TenantID: tenant, OrgID: org, Name: name,
			Address: "x", ContactName: "c", ContactPhone: "1", Status: "active",
			AdminUID: uuid.NewString(),
		}).Error)
	}
	mkMerchant(t1, o1, "ISO-A")
	mkMerchant(t2, o2, "ISO-B")
	require.NoError(t, db.Create(&models.Site{ID: uuid.NewString(), TenantID: o1, OrgID: orgS1, Name: "S-ISO-A"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: uuid.NewString(), TenantID: o2, OrgID: orgS2, Name: "S-ISO-B"}).Error)

	// 顾客 A：零租户缓存 + 身份材料（过 #2040 下单门控）
	subA := uuid.NewString()
	localA := uuid.NewString()
	photos := "/uploads/media/iso-a-id.jpg"
	method := "manual"
	require.NoError(t, db.Create(&models.User{
		ID: localA, IAMSub: subA, Name: "顾客ISO-A", Phone: "13900003301",
		TenantID:     "00000000-0000-0000-0000-000000000000",
		OrgID:        "00000000-0000-0000-0000-000000000000",
		IdPhotoFront: &photos, IdPhotoBack: &photos, FaceVerified: true,
		FaceVerifyMethod: &method, Status: "active",
	}).Error)

	instID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: o1, OrgID: &orgS1,
		SN: "SN-ISO-1", StockStatus: "available",
		Pricing: `[{"daily_rent": 10.0, "weekly_rent": 70.0, "monthly_rent": 240.0, "deposit": 500.0}]`,
	}).Error)

	r := gin.New()
	r.POST("/user/orders", NewUserRentalHandler().CreateOrder)
	r.GET("/orders/:id", GetOrder)
	r.GET("/orders", GetOrders)

	return db, r, isoEnv{o1: o1, o2: o2, subA: subA, instID: instID}
}

func createIsoOrder(t *testing.T, db *gorm.DB, r *gin.Engine, env isoEnv) string {
	t.Helper()
	customerA := testutil.TestActor{TenantID: "", OrgID: "", UserID: env.subA, Role: "USER"}
	code, resp := isoDo(t, r, http.MethodPost, "/user/orders", customerA, map[string]interface{}{
		"instrument_id": env.instID, "start_date": "2024-01-01", "end_date": "2024-01-31", "rent_days": 30,
	})
	require.Equal(t, http.StatusCreated, code, "%v", resp)
	data, _ := resp["data"].(map[string]interface{})
	orderID, _ := data["order_id"].(string)
	require.NotEmpty(t, orderID)

	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	assert.Equal(t, env.o1, order.TenantID, "订单归属必须落商户 org（数据归属）")
	return orderID
}

func isoDo(t *testing.T, r *gin.Engine, method, path string, actor testutil.TestActor, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var raw *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		raw = bytes.NewReader(b)
	} else {
		raw = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, raw)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// ISO-14：网点管理员跨商户读订单详情 → 404；本商户网点管理员可读（商户 org 锚语义）。
func TestIsolationMatrix_OrderDetail_CrossMerchant_ISO14(t *testing.T) {
	db, r, env := newIsoEnv(t)
	orderID := createIsoOrder(t, db, r, env)

	siteAdminA := testutil.TestActor{TenantID: env.o1, OrgID: uuid.NewString(), UserID: uuid.NewString(), Role: "ADMIN"}
	siteAdminB := testutil.TestActor{TenantID: env.o2, OrgID: uuid.NewString(), UserID: uuid.NewString(), Role: "ADMIN"}

	code, _ := isoDo(t, r, http.MethodGet, "/orders/"+orderID, siteAdminB, nil)
	assert.Contains(t, []int{http.StatusNotFound, http.StatusForbidden}, code, "跨商户订单详情必须被拒")

	code, resp := isoDo(t, r, http.MethodGet, "/orders/"+orderID, siteAdminA, nil)
	assert.Equal(t, http.StatusOK, code, "本商户网点管理员可读: %v", resp)
}

// ISO-15：顾客零租户链路（tid 空不被拒；本人可读；列表可见；跨顾客负例）。
func TestIsolationMatrix_CustomerZeroTenant_Chain_ISO15(t *testing.T) {
	db, r, env := newIsoEnv(t)
	orderID := createIsoOrder(t, db, r, env)
	customerA := testutil.TestActor{TenantID: "", OrgID: "", UserID: env.subA, Role: "USER"}
	customerB := testutil.TestActor{TenantID: "", OrgID: "", UserID: uuid.NewString(), Role: "USER"}

	// 本人可读
	code, _ := isoDo(t, r, http.MethodGet, "/orders/"+orderID, customerA, nil)
	assert.Equal(t, http.StatusOK, code)

	// 我的订单列表可见
	code, resp := isoDo(t, r, http.MethodGet, "/orders", customerA, nil)
	require.Equal(t, http.StatusOK, code)
	found := false
	if d, ok := resp["data"].(map[string]interface{}); ok {
		if list, ok := d["list"].([]interface{}); ok {
			for _, it := range list {
				if m, ok := it.(map[string]interface{}); ok && m["id"] == orderID {
					found = true
				}
			}
		}
	}
	assert.True(t, found, "我的订单列表必须包含新订单: %v", resp)

	// 跨顾客负例（#1694 家族）
	code, _ = isoDo(t, r, http.MethodGet, "/orders/"+orderID, customerB, nil)
	assert.Contains(t, []int{http.StatusNotFound, http.StatusForbidden}, code)
}
