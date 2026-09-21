package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
)

// #2010 S2: buildOrderDeliveryAddress 契约（字符串直存；对象 JSON 文本；空值 nil）
func TestBuildOrderDeliveryAddress_2010S2(t *testing.T) {
	s := "北京市朝阳区某路1号"
	got := buildOrderDeliveryAddress(s)
	require.NotNil(t, got)
	require.Equal(t, s, *got, "字符串应原样存储（无 JSON 引号）")

	obj := map[string]interface{}{"street": "某路1号", "phone": "13800000000"}
	got2 := buildOrderDeliveryAddress(obj)
	require.NotNil(t, got2)
	require.Contains(t, *got2, `"street"`, "对象应存为 JSON 文本")

	require.Nil(t, buildOrderDeliveryAddress(nil))
	require.Nil(t, buildOrderDeliveryAddress(""))
}

// #2010 S2: GET /orders/:id 的 delivery_address 读自 orders 列（不依赖 lease_sessions）
func TestGetOrder_DeliveryAddressFromOrders_2010S2(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	_, instID, userID := setupTestData(t, db, tenantID)
	defer cleanupTestData(db, tenantID)

	addr := "上海市徐汇区某路2号"
	orderID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO orders (id, tenant_id, org_id, user_id, instrument_id, level, lease_term, monthly_rent, deposit, status, shipping_fee, delivery_address, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'standard', 1, 0, 100000, 'in_lease', 0, ?, now(), now())`,
		orderID, tenantID, tenantID, userID, instID, addr).Error)
	// #2010 S4: lease_sessions 已废弃；地址读自 orders.delivery_address

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	req := httptest.NewRequest("GET", "/orders/"+orderID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DeliveryAddress string `json:"delivery_address"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, addr, resp.Data.DeliveryAddress, "地址应读自 orders.delivery_address")
}
