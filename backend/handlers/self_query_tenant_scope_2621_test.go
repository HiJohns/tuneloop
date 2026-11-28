package handlers

// #2621 回归：其余身份自查询（共享解析器 + `/user/*` 家庭）不得被自动租户过滤。
//
// 与 #2620 同源的缺陷：users.tenant_id 是 IAM 关系的本地缓存，可能与登录 JWT 的 tid 不一致；
// 此时「按 JWT sub 查我自己那行」若走带 tid 的句柄，会被 addTenantScope 追加
// `tenant_id = tid` → 404 / 静默空 / 误报 profile_not_ready / 建 shadow user 冲突 500。
//
// 测试保真（计划 D6）：上下文必须由 testutil.TestActor.InjectContext 构造——它同时写
// middleware 与 database 两套 key，自动租户回调才真实执行。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// selfQuery2621Context 构造带完整双套 context key 的请求上下文。
func selfQuery2621Context(actor testutil.TestActor, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req = req.WithContext(actor.InjectContext(req.Context()))
	c.Request = req
	return c, w
}

func TestSelfQueryTenantScope2621(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	// 预生产故障形态：本地缓存 tenant（IAM 关系解析出的商户根组织）≠ JWT tid。
	cacheTenant := uuid.NewString()
	cacheOrg := uuid.NewString()
	jwtTenant := uuid.NewString()
	jwtOrg := uuid.NewString()
	otherTenant := uuid.NewString()
	iamSub := "iam-sub-2621"

	self := models.User{
		ID:          uuid.NewString(),
		IAMSub:      iamSub,
		TenantID:    cacheTenant,
		OrgID:       cacheOrg,
		Name:        "Self 2621",
		Username:    "self2621",
		Status:      "active",
		CreditScore: 700,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&self).Error)

	// 与 JWT tid 同租户的对照行（用于证明自动 scope 未被全局关闭）。
	jwtTenantUser := models.User{
		ID: uuid.NewString(), IAMSub: "iam-sub-2621-jwt", TenantID: jwtTenant, OrgID: jwtOrg,
		Name: "Same Tenant 2621", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&jwtTenantUser).Error)

	// 第三个租户的行（必须不可见）。
	otherUser := models.User{
		ID: uuid.NewString(), IAMSub: "iam-sub-2621-other", TenantID: otherTenant, OrgID: otherTenant,
		Name: "Other Tenant 2621", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&otherUser).Error)

	// 域数据行（cacheTenant 下的待处理订单）：域查询必须继续被租户过滤。
	domainOrder := models.PendingOrder{
		ID: uuid.NewString(), TenantID: cacheTenant, UserID: self.ID,
		Payload: `{"probe":"2621"}`, Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&domainOrder).Error)

	// 离线 IAM：RegisterAsCustomer 通过 POST /api/v1/users/:id/customer-role 授权。
	iamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/token":
			fmt.Fprint(w, `{"access_token":"mock-token","expires_in":3600,"token_type":"Bearer"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/customer-role"):
			fmt.Fprint(w, `{"code":20000,"message":"success"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer iamSrv.Close()
	services.SetIAMInternalURLForTesting(iamSrv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	actor := testutil.TestActor{
		TenantID: jwtTenant,
		OrgID:    jwtOrg,
		UserID:   iamSub,
		Role:     "USER",
	}

	countUsers := func() int64 {
		var n int64
		require.NoError(t, database.GetDB().Model(&models.User{}).Count(&n).Error)
		return n
	}

	cases := 0

	t.Run("ensure_local_user", func(t *testing.T) {
		cases++
		ctx := actor.InjectContext(t.Context())
		scopedDB := database.GetDB().WithContext(ctx) // 调用方句柄带 tid（生产形态）
		before := countUsers()
		id, err := middleware.EnsureLocalUser(ctx, scopedDB)
		after := countUsers()

		require.NoError(t, err, "EnsureLocalUser 不得因缓存 tenant ≠ tid 而失败")
		require.Equal(t, self.ID, id)
		require.Equal(t, before, after, "不得创建 shadow user（iam_sub 唯一索引会冲突）")
		t.Logf("ENSURE_HIT=1")
		t.Logf("ENSURE_NO_NEW_ROW=1")
		t.Logf("USERS_ROW_COUNT_DELTA=%d", after-before)
	})

	t.Run("local_user_id", func(t *testing.T) {
		cases++
		ctx := actor.InjectContext(t.Context())
		scopedDB := database.GetDB().WithContext(ctx)
		id, err := middleware.LocalUserID(ctx, scopedDB)
		require.NoError(t, err)
		require.Equal(t, self.ID, id, "LocalUserID 不得静默返回空")
		t.Logf("LOCAL_USER_ID_HIT=1")
	})

	t.Run("points_balance", func(t *testing.T) {
		cases++
		c, w := selfQuery2621Context(actor, http.MethodGet, "/user/points/balance", "")
		NewUserPointsHandler().GetBalance(c)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		t.Logf("POINTS_BALANCE_HTTP=%d", w.Code)
	})

	t.Run("points_transactions", func(t *testing.T) {
		cases++
		c, w := selfQuery2621Context(actor, http.MethodGet, "/user/points/transactions?page=1&page_size=20", "")
		NewUserPointsHandler().ListTransactions(c)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		t.Logf("POINTS_TX_HTTP=%d", w.Code)
	})

	t.Run("pending_orders", func(t *testing.T) {
		cases++
		// 入口 A：写入（成功语义是 201 Created，保持现网契约不变）。
		c, w := selfQuery2621Context(actor, http.MethodPost, "/user/pending-orders", `{"payload":{"probe":"2621"}}`)
		CreatePendingOrder(c)
		require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
		t.Logf("PENDING_ORDER_CREATE_HTTP=%d", w.Code)

		// 入口 B：读取（200）。两个入口都经 currentLocalUser —— 缺陷点在此。
		c2, w2 := selfQuery2621Context(actor, http.MethodGet, "/user/pending-orders", "")
		ListPendingOrders(c2)
		require.Equal(t, http.StatusOK, w2.Code, "body=%s", w2.Body.String())
		t.Logf("PENDING_ORDER_HTTP=%d", w2.Code)
	})

	t.Run("face_capture", func(t *testing.T) {
		cases++
		c, w := selfQuery2621Context(actor, http.MethodGet, "/user/face-capture/status", "")
		(&FaceCaptureHandler{}).GetFaceCaptureStatus(c)
		require.NotEqual(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
		t.Logf("FACE_CAPTURE_NOT_404=1")
	})

	t.Run("addresses", func(t *testing.T) {
		cases++
		c, w := selfQuery2621Context(actor, http.MethodGet, "/user/addresses", "")
		NewUserAddressHandler().ListAddresses(c)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		t.Logf("ADDRESSES_HTTP=%d", w.Code)
	})

	t.Run("pay_calculate", func(t *testing.T) {
		cases++
		ctx := actor.InjectContext(t.Context())
		scopedDB := database.GetDB().WithContext(ctx)

		// 故障点：钱包解析（users 行）必须命中，而不是 record not found。
		wallet, err := getWalletInfo(ctx, scopedDB, iamSub, jwtTenant, 0)
		require.NoError(t, err, "getWalletInfo 不得因缓存 tenant ≠ tid 而失败")
		require.NotNil(t, wallet)

		// 入口同路径：handler 响应里不得出现 user not found（订单查询另计）。
		c, w := selfQuery2621Context(actor, http.MethodPost, "/pay/calculate",
			`{"type":"rent","id":"`+uuid.NewString()+`"}`)
		CalculatePayment(c)
		require.NotContains(t, strings.ToLower(w.Body.String()), "user not found")
		t.Logf("PAY_CALC_NO_USER_ERROR=1")
	})

	t.Run("deposit_waiver", func(t *testing.T) {
		cases++
		c, w := selfQuery2621Context(actor, http.MethodGet, "/user/deposit-waiver/eligibility", "")
		GetDepositWaiverEligibility(c)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var resp struct {
			Code int `json:"code"`
			Data struct {
				Eligible bool     `json:"eligible"`
				Reasons  []string `json:"reasons"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		onlyProfileNotReady := len(resp.Data.Reasons) == 1 && resp.Data.Reasons[0] == "profile_not_ready"
		require.False(t, onlyProfileNotReady, "不得因身份查询失败而误报 profile_not_ready：%s", w.Body.String())
		t.Logf("DEPOSIT_WAIVER_NOT_PROFILE_NOT_READY=1")
	})

	t.Run("customer_role", func(t *testing.T) {
		cases++
		c, w := selfQuery2621Context(actor, http.MethodPost, "/user/register-as-customer", "")
		RegisterAsCustomer(c)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		t.Logf("CUSTOMER_ROLE_HTTP=%d", w.Code)
	})

	t.Run("cross_tenant_scoped", func(t *testing.T) {
		cases++
		// 负例守卫 1：他人用户行仍被自动租户过滤隐藏。
		ctx := actor.InjectContext(t.Context())
		scoped := database.GetDB().WithContext(ctx)

		var other models.User
		err := scoped.Where("iam_sub = ?", otherUser.IAMSub).First(&other).Error
		require.ErrorIs(t, err, gorm.ErrRecordNotFound, "他人用户行必须仍被租户过滤隐藏")

		var visible []models.User
		require.NoError(t, scoped.Model(&models.User{}).Find(&visible).Error)
		require.Len(t, visible, 1, "Find 只能返回 JWT tid 租户下的行")
		require.Equal(t, jwtTenantUser.ID, visible[0].ID)
		t.Logf("CROSS_TENANT_HIDDEN=1")

		// 负例守卫 2：域数据查询（带 TenantID 的 pending_orders）仍受租户过滤。
		// 按夹具行 id 精确断言，避免 pending_orders 子用例新增行干扰计数。
		var domainVisible []models.PendingOrder
		require.NoError(t, scoped.Model(&models.PendingOrder{}).Where("id = ?", domainOrder.ID).Find(&domainVisible).Error)
		require.Len(t, domainVisible, 0, "域查询不得因本修复而失去租户过滤")

		var domainViaIdentity []models.PendingOrder
		require.NoError(t, database.GetDB().WithContext(database.IdentityCtx(ctx)).
			Model(&models.PendingOrder{}).Where("id = ?", domainOrder.ID).Find(&domainViaIdentity).Error)
		require.Len(t, domainViaIdentity, 1, "对照：行确实存在（只是被租户过滤挡住）")
		t.Logf("DOMAIN_QUERY_STILL_SCOPED=1")
	})

	if t.Failed() {
		t.Logf("CASES_FAIL=1")
	} else {
		t.Logf("CASES_TOTAL=%d", cases)
		t.Logf("CASES_FAIL=0")
	}
}
