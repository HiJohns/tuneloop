package handlers

// #2620 回归：`/users/me/*` 身份自查询不得套用自动租户 scope。
//
// 故障形态（预生产 1.0.102）：users.tenant_id 是 IAM 关系的本地缓存，可能不等于
// 登录 JWT 的 tid（平台级商户/多组织用户）。此时按 `iam_sub = JWT sub` 查「我自己那行」
// 会被 addTenantScope 追加 `tenant_id = tid` → 404 40004 user not found。
//
// 关键测试保真要求（计划 D4）：上下文必须由 testutil.TestActor.InjectContext 构造——
// 它会同时写 middleware 与 database 两套 key，自动租户回调才真实执行；只写
// middleware.ContextKeyUserID 的旧写法会让 addTenantScope 静默不生效（本缺陷的测试盲点）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// selfQueryActorContext 构造带完整双套 context key 的请求上下文（含 database tenant 键）。
func selfQueryActorContext(actor testutil.TestActor) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(actor.InjectContext(req.Context()))
	c.Request = req
	return c, w
}

func TestSelfQueryTenantScope2620(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	// 预生产故障形态：本地缓存 tenant 来自 IAM 关系（merchant 根组织），
	// 而 JWT tid 是另一个租户（例如平台组织）。
	cacheTenant := uuid.NewString()
	cacheOrg := uuid.NewString()
	jwtTenant := uuid.NewString()
	jwtOrg := uuid.NewString()
	refereeTenant := uuid.NewString()

	self := models.User{
		ID:        uuid.NewString(),
		IAMSub:    "iam-sub-2620",
		TenantID:  cacheTenant,
		OrgID:     cacheOrg,
		Name:      "Self 2620",
		Username:  "self2620",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&self).Error)

	// 被推荐人刻意放在第三个租户下：证明 referee 查询也必须走身份句柄。
	referee := models.User{
		ID:        uuid.NewString(),
		IAMSub:    "iam-sub-2620-referee",
		TenantID:  refereeTenant,
		OrgID:     refereeTenant,
		Name:      "Referee 2620",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&referee).Error)

	// 对照行：与 JWT tid 同租户的其他人——用于证明自动 scope 未被全局关闭。
	sameTenantOther := models.User{
		ID:        uuid.NewString(),
		IAMSub:    "iam-sub-2620-same-tenant",
		TenantID:  jwtTenant,
		OrgID:     jwtOrg,
		Name:      "Same Tenant 2620",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&sameTenantOther).Error)

	referral := models.Referral{
		ID:         uuid.NewString(),
		ReferrerID: self.ID,
		RefereeID:  referee.ID,
		RefCode:    "REF26200",
		Status:     "registered",
		CreatedAt:  time.Now(),
	}
	require.NoError(t, db.Create(&referral).Error)

	// 离线 IAM：Unbind 通过 PUT /api/v1/users/:id 更新（#2016 后 openid 权威在 IAM）。
	iamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/token":
			fmt.Fprint(w, `{"access_token":"mock-token","expires_in":3600,"token_type":"Bearer"}`)
		case r.Method == http.MethodPut && len(r.URL.Path) > len("/api/v1/users/"):
			fmt.Fprint(w, `{"code":20000,"data":{"id":"`+r.URL.Path[len("/api/v1/users/"):]+`"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer iamSrv.Close()
	services.SetIAMInternalURLForTesting(iamSrv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	wxStub := newWxAPIStub(t, false, nil)
	defer wxStub.Close()
	services.SetWxAPIBaseURLForTesting(wxStub.URL)
	t.Cleanup(func() { services.SetWxAPIBaseURLForTesting("") })
	t.Setenv("WX_APPID", "wx-test-appid")
	t.Setenv("WX_SECRET", "wx-test-secret")
	t.Setenv("IAM_NAMESPACE", "test-ns")

	actor := testutil.TestActor{
		TenantID: jwtTenant,
		OrgID:    jwtOrg,
		UserID:   self.IAMSub,
		Role:     "ADMIN",
	}

	t.Run("bind_gen", func(t *testing.T) {
		c, w := selfQueryActorContext(actor)
		NewWechatBindHandler().GenBindToken(c)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Token         string  `json:"token"`
				WxacodeBase64 *string `json:"wxacode_base64"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.Len(t, resp.Data.Token, 16)
		require.NotNil(t, resp.Data.WxacodeBase64)
		require.NotEmpty(t, *resp.Data.WxacodeBase64)
		t.Logf("SELF2620_BIND_GEN_HTTP=%d", w.Code)
	})

	t.Run("unbind", func(t *testing.T) {
		c, w := selfQueryActorContext(actor)
		NewWechatBindHandler().Unbind(c)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		var resp struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		t.Logf("SELF2620_UNBIND_HTTP=%d", w.Code)
	})

	t.Run("promo_qr", func(t *testing.T) {
		c, w := selfQueryActorContext(actor)
		NewReferralHandler().GetPromoQR(c)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		var resp struct {
			Code int `json:"code"`
			Data struct {
				RefCode string `json:"ref_code"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.Equal(t, self.IAMSub[:8], resp.Data.RefCode)
		t.Logf("SELF2620_PROMO_QR_HTTP=%d", w.Code)
	})

	t.Run("referrals", func(t *testing.T) {
		c, w := selfQueryActorContext(actor)
		NewReferralHandler().ListReferrals(c)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Total int `json:"total"`
				List  []struct {
					RefCode string `json:"ref_code"`
					Name    string `json:"name"`
				} `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.Equal(t, 1, resp.Data.Total)
		require.Len(t, resp.Data.List, 1, "referee 姓名不得因租户过滤被静默丢弃")
		require.Equal(t, referee.Name, resp.Data.List[0].Name)
		t.Logf("SELF2620_REFERRALS_TOTAL=%d", resp.Data.Total)
		t.Logf("SELF2620_REFEREE_NAME=1")
	})

	t.Run("cross_tenant_scoped", func(t *testing.T) {
		// 负例守卫：修复不得变成「全局关闭自动租户 scope」。
		ctx := actor.InjectContext(t.Context())
		scoped := database.GetDB().WithContext(ctx)

		var other models.User
		err := scoped.Where("iam_sub = ?", referee.IAMSub).First(&other).Error
		require.ErrorIs(t, err, gorm.ErrRecordNotFound, "他人用户行必须仍被租户过滤隐藏")

		var visible []models.User
		require.NoError(t, scoped.Model(&models.User{}).Find(&visible).Error)
		require.Len(t, visible, 1, "Find 只能返回 JWT tid 租户下的行")
		require.Equal(t, sameTenantOther.ID, visible[0].ID)
		t.Logf("SELF2620_CROSS_TENANT_HIDDEN=1")
	})
}
