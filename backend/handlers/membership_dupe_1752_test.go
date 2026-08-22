package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// #1752 regression tests: membership prepay duplicate-payment guard.
// A logged-in user with a paid membership record (or an activated level)
// must get 40002 — never a second prepay.

func membershipPrepayRouter(t *testing.T, user models.User) *gin.Engine {
	t.Helper()
	router := gin.New()
	customer := testutil.MakeCustomer("", user.ID)
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)
	return router
}

func doMembershipPrepay(t *testing.T, router *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0,
		"open_id":    "mem-openid",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/pay/prepay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestPrepayMembership_AlreadyPaid_40002 (#1752): user has a paid
// membership record → 40002, no second prepay.
func TestPrepayMembership_AlreadyPaid_40002(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000f1",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "PaidMember",
		Role:     "USER",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)
	outTradeNo := "mem-paid-otn-1"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: user.TenantID, OrgID: &user.OrgID,
		UserID: user.ID, OrderType: "membership", OutTradeNo: &outTradeNo,
		Amount: 9900, Type: "payment", Status: "paid",
	}).Error)

	router := membershipPrepayRouter(t, user)
	w := doMembershipPrepay(t, router)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40002, resp.Code)
	require.Contains(t, resp.Message, "会员已激活", "clear duplicate-payment message")
}

// TestPrepayMembership_ActivatedLevel_40002 (#1752): user has a
// membership_level_id (activated) → 40002 even without a paid record.
func TestPrepayMembership_ActivatedLevel_40002(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	levelID := 1
	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000f2",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "ActivatedMember",
		Role:     "USER",
		Status:   "active",
		MembershipLevelID: &levelID,
	}
	require.NoError(t, db.Create(&user).Error)

	router := membershipPrepayRouter(t, user)
	w := doMembershipPrepay(t, router)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40002, resp.Code)
}

// TestPrepayMembership_NotPaid_Proceeds (#1752): no paid record, no level →
// prepay proceeds (no regression).
func TestPrepayMembership_NotPaid_Proceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(stubJSAPIClient{}, &wechatpay.Config{
		AppID:           "wxcb44a1be70e356ed",
		NotifyURL:       "http://localhost:5553/api/wechatpay/notify",
		RefundNotifyURL: "http://localhost:5553/api/wechatpay/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000f3",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "FreshMember",
		Role:     "USER",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)

	router := membershipPrepayRouter(t, user)
	w := doMembershipPrepay(t, router)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
}
