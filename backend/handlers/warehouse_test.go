package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

func setupWarehouseTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{
		&models.Order{},
		&models.OrderStatusHistory{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestListWarehouseOrders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupWarehouseTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, UserID: uuid.New().String(),
		InstrumentID: uuid.New().String(), OrgID: uuid.New().String(), Status: models.OrderStatusShipped,
	}
	db.Create(&order)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/warehouse/orders", handler.ListOrders)

	req := httptest.NewRequest("GET", "/api/warehouse/orders", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Greater(t, len(response.Data.List), 0)
}

func TestUpdateShipping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupWarehouseTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	orderID := uuid.New().String()
	order := models.Order{
		ID: orderID, TenantID: tenantID, UserID: uuid.New().String(), OrgID: uuid.New().String(),
		Status: "paid", InstrumentID: uuid.New().String(),
	}
	db.Create(&order)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/shipping", handler.UpdateShipping)

	reqBody := map[string]interface{}{
		"tracking_number": "SF123456",
		"company":         "顺丰",
		"shipped_at":      time.Now(),
		"shipping_fee":    1.0, // #1754: 填 1 元 → 100 分
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)

	// #1754: 1 元 → shipping_fee = 100 分（非 1 分）。
	var after models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
	assert.Equal(t, models.Cents(100), after.ShippingFee, "shipping fee 1 元 → 100 分")

	db.Exec("DELETE FROM orders WHERE tenant_id = ?", tenantID)
	db.Exec("DELETE FROM order_status_history WHERE tenant_id = ?", tenantID)
}

// TestReportWechatShipping_OpenIDPriority verifies the #1731 authority order:
// record.OpenID (payer.openid persisted at callback) wins; users.wx_openid
// cache is only a fallback when record.OpenID is empty.
func TestReportWechatShipping_OpenIDPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupWarehouseTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}
	// users / order_payment_records keep their real (migration-built) schema:
	// IAMSub is tagged `-:migration`, so GORM CreateTable would drop iam_sub.
	// Use the existing tables and clean up test rows per case.
	_ = db.Migrator().HasTable(&models.User{})
	_ = db.Migrator().HasTable(&models.OrderPaymentRecord{})

	tenantID := uuid.New().String()

	cases := []struct {
		name         string
		recordOpenID string
		cacheOpenID  string
		want         string
	}{
		// record.OpenID present → authoritative, cache ignored.
		{"record openid wins over cache", "opjhZ3-record-openid", "opjhZ3-cache-openid", "opjhZ3-record-openid"},
		// record.OpenID empty → fall back to users.wx_openid cache.
		{"record empty falls back to cache", "", "opjhZ3-cache-openid", "opjhZ3-cache-openid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WX_APPID", "wx_test_appid")
			t.Setenv("WX_SECRET", "wx_test_secret")
			orderID := uuid.New().String()
			userID := uuid.New().String()
			instrumentID := uuid.New().String()
			outTradeNo := "rent" + uuid.New().String()[:12]
			txID := "4500000337" + uuid.New().String()[:10]

			if err := db.Create(&models.User{
				ID: userID, TenantID: tenantID, OrgID: tenantID, IAMSub: userID,
				WxOpenid: tc.cacheOpenID,
			}).Error; err != nil {
				t.Fatalf("create user: %v", err)
			}
			if err := db.Create(&models.Order{
				ID: orderID, TenantID: tenantID, UserID: userID,
				InstrumentID: instrumentID, OrgID: tenantID, Status: models.OrderStatusShipped,
			}).Error; err != nil {
				t.Fatalf("create order: %v", err)
			}
			outTradeNoCopy := outTradeNo
			orderIDCopy := orderID
			if err := db.Create(&models.OrderPaymentRecord{
				OutTradeNo: &outTradeNoCopy, TransactionID: &txID,
				OrderID: &orderIDCopy, UserID: userID, TenantID: tenantID, Status: "paid",
				OpenID: tc.recordOpenID,
			}).Error; err != nil {
				t.Fatalf("create payment record: %v", err)
			}

			var gotOpenID string
			var mu sync.Mutex
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/cgi-bin/token") {
					w.Write([]byte(`{"access_token":"tok","expires_in":7200}`))
					return
				}
				var body struct {
					Payer struct {
						OpenID string `json:"openid"`
					} `json:"payer"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				mu.Lock()
				gotOpenID = body.Payer.OpenID
				mu.Unlock()
				w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
			}))
			defer ts.Close()

			services.SetWxAPIBaseURLForTesting(ts.URL)
			defer services.SetWxAPIBaseURLForTesting("https://api.weixin.qq.com")

			reportWechatShipping(db, models.Order{ID: orderID, UserID: userID}, "顺丰", "SF123456")

			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				mu.Lock()
				v := gotOpenID
				mu.Unlock()
				if v != "" {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			mu.Lock()
			got := gotOpenID
			mu.Unlock()
			assert.Equal(t, tc.want, got, "upload_shipping_info payer.openid must resolve from %q", tc.want)

			db.Exec("DELETE FROM orders WHERE tenant_id = ?", tenantID)
			db.Exec("DELETE FROM order_payment_records WHERE tenant_id = ?", tenantID)
			db.Exec("DELETE FROM users WHERE tenant_id = ?", tenantID)
		})
	}
}
