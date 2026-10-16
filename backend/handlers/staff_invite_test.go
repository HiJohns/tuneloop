package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2031 邀请制自助加入守卫：未登录 401 / 无效邀请码 404 / 过期 410 / 已用 409。
// 邀请码使用随机值以兼容共享测试库（避免跨运行唯一索引冲突）。
func TestAcceptInvite_Guards(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.StaffInvite{}))

	userID := uuid.New().String()
	// #2052: JWT sub ≠ 本地 users.id —— 必须能解析（iam_sub 反查）
	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: userID,
		TenantID: uuid.New().String(), OrgID: uuid.New().String(), Status: "active",
	}).Error)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/accept-invite", AcceptInvite)

	post := func(code string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"code": code})
		req := httptest.NewRequest("POST", "/api/user/accept-invite", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	randCode := func(prefix string) string {
		return prefix + uuid.New().String()[:12]
	}

	t.Run("未登录 → 401", func(t *testing.T) {
		e := gin.New()
		e.POST("/api/user/accept-invite", AcceptInvite)
		body, _ := json.Marshal(map[string]string{"code": "x"})
		req := httptest.NewRequest("POST", "/api/user/accept-invite", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown code → 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, post("deadbeefdeadbeef").Code)
	})

	t.Run("expired invite → 410", func(t *testing.T) {
		code := randCode("exp")
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: strPtr2114(uuid.New().String()), Role: "site_member", Code: code,
			Status: "pending", ExpiresAt: time.Now().Add(-time.Hour),
		}
		require.NoError(t, db.Create(&inv).Error)
		assert.Equal(t, http.StatusGone, post(code).Code)
	})

	t.Run("already accepted → 409", func(t *testing.T) {
		code := randCode("used")
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: strPtr2114(uuid.New().String()), Role: "site_member", Code: code,
			Status: "accepted", ExpiresAt: time.Now().Add(time.Hour),
		}
		require.NoError(t, db.Create(&inv).Error)
		assert.Equal(t, http.StatusConflict, post(code).Code)
	})

	// 回归（审计发现的真实 Bug）：created_by/accepted_by 未设置时必须写 NULL，
	// 而非空串——UUID 列拒绝 ''（SQLSTATE 22P02），否则签发邀请码会 500。
	t.Run("unset created_by/accepted_by → NULL, not empty uuid", func(t *testing.T) {
		code := randCode("nil")
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: strPtr2114(uuid.New().String()), Role: "site_member", Code: code,
			Status: "pending", ExpiresAt: time.Now().Add(time.Hour),
		}
		require.NoError(t, db.Create(&inv).Error)
		var stored models.StaffInvite
		require.NoError(t, db.Where("code = ?", code).First(&stored).Error)
		assert.Nil(t, stored.CreatedBy)
		assert.Nil(t, stored.AcceptedBy)
	})
}

// #2052 邀请通知守卫：非被邀请人 403 / 已处理 409 / 过期 410 / 拒绝落 status=rejected。
func TestInvitation_Guards(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.StaffInvite{}))

	// #2052: 主动构造 sub ≠ 本地 users.id 的形态（防止 fixture 同值掩盖 sub/key 混用）
	inviteeSub, inviteeLocal := uuid.New().String(), uuid.New().String()
	otherSub, otherLocal := uuid.New().String(), uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: inviteeLocal, IAMSub: inviteeSub,
		TenantID: uuid.New().String(), OrgID: uuid.New().String(), Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: otherLocal, IAMSub: otherSub,
		TenantID: uuid.New().String(), OrgID: uuid.New().String(), Status: "active",
	}).Error)

	newRouter := func(uid string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, uid)
			ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, uuid.New().String())
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/api/user/invitations/:id/accept", AcceptInvitation)
		r.POST("/api/user/invitations/:id/reject", RejectInvitation)
		return r
	}

	mkInvite := func(status string, ttl time.Duration) models.StaffInvite {
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: strPtr2114(uuid.New().String()), Role: "site_member", Code: uuid.New().String()[:16],
			Status: status, ExpiresAt: time.Now().Add(ttl), InviteeUserID: &inviteeLocal,
		}
		require.NoError(t, db.Create(&inv).Error)
		return inv
	}

	post := func(r *gin.Engine, id, action string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/user/invitations/"+id+"/"+action, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("非被邀请人（其他本地用户）→ 403", func(t *testing.T) {
		inv := mkInvite("pending", time.Hour)
		assert.Equal(t, http.StatusForbidden, post(newRouter(otherSub), inv.ID, "accept").Code)
	})

	t.Run("已处理 → 409", func(t *testing.T) {
		inv := mkInvite("accepted", time.Hour)
		assert.Equal(t, http.StatusConflict, post(newRouter(inviteeSub), inv.ID, "accept").Code)
	})

	t.Run("过期 → 410", func(t *testing.T) {
		inv := mkInvite("pending", -time.Hour)
		assert.Equal(t, http.StatusGone, post(newRouter(inviteeSub), inv.ID, "accept").Code)
	})

	t.Run("sub 无本地用户 → 404", func(t *testing.T) {
		inv := mkInvite("pending", time.Hour)
		assert.Equal(t, http.StatusNotFound, post(newRouter(uuid.New().String()), inv.ID, "accept").Code)
	})

	t.Run("拒绝（sub≠users.id 解析成功）→ 200 且 status=rejected", func(t *testing.T) {
		inv := mkInvite("pending", time.Hour)
		w := post(newRouter(inviteeSub), inv.ID, "reject")
		assert.Equal(t, http.StatusOK, w.Code)
		var stored models.StaffInvite
		require.NoError(t, db.Where("id = ?", inv.ID).First(&stored).Error)
		assert.Equal(t, "rejected", stored.Status)
	})
}

func strPtr2114(s string) *string { return &s }

// #2216: 「我的待接受定向邀请」查询——按 user_id 或（未认领时）identifier ∈ {手机,邮箱} 匹配；
// 他人/已处理/已过期一律不可见。
func TestListMyPendingInvitations(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.StaffInvite{}, &models.User{}, &models.Site{}, &models.Merchant{}))

	mySub, myLocal := uuid.New().String(), uuid.New().String()
	otherLocal := uuid.New().String()
	// 随机标识：共享测试库跨运行不清空，固定值会与历次运行的 identifier 邀请冲突。
	myPhone := "138" + uuid.New().String()[:8]
	myEmail := uuid.New().String()[:8] + "@example.com"
	require.NoError(t, db.Create(&models.User{
		ID: myLocal, IAMSub: mySub, Phone: myPhone, Email: myEmail,
		TenantID: uuid.New().String(), OrgID: uuid.New().String(), Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: otherLocal, IAMSub: uuid.New().String(), Phone: "139" + uuid.New().String()[:8], Email: uuid.New().String()[:8] + "@example.com",
		TenantID: uuid.New().String(), OrgID: uuid.New().String(), Status: "active",
	}).Error)

	mk := func(status string, ttl time.Duration, userID *string, identifier string) models.StaffInvite {
		inv := models.StaffInvite{
			ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
			SiteID: strPtr2114(uuid.New().String()), Role: "site_member", Code: uuid.New().String()[:16],
			Status: status, ExpiresAt: time.Now().Add(ttl),
			InviteeUserID: userID, InviteeIdentifier: identifier,
		}
		require.NoError(t, db.Create(&inv).Error)
		return inv
	}

	byUser := mk("pending", time.Hour, &myLocal, "")  // 可见
	byPhone := mk("pending", time.Hour, nil, myPhone) // 可见（identifier=我的手机）
	byEmail := mk("pending", time.Hour, nil, myEmail) // 可见（identifier=我的邮箱）
	mk("pending", time.Hour, &otherLocal, "")         // 他人 → 不可见
	mk("pending", time.Hour, nil, "13999999999")      // 他人 identifier → 不可见
	mk("accepted", time.Hour, &myLocal, "")           // 已处理 → 不可见
	mk("pending", -time.Hour, &myLocal, "")           // 已过期 → 不可见

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, mySub)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/api/user/invitations/pending", ListMyPendingInvitations)

	req := httptest.NewRequest("GET", "/api/user/invitations/pending", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 20000, resp.Code)
	require.Equal(t, 3, len(resp.Data.List))
	got := map[string]bool{}
	for _, it := range resp.Data.List {
		got[it["id"].(string)] = true
		assert.NotContains(t, it, "code", "邀请码不得下发")
	}
	assert.True(t, got[byUser.ID])
	assert.True(t, got[byPhone.ID])
	assert.True(t, got[byEmail.ID])
}

// #2216: identifier 定向邀请（未认领）可由 identifier 本人接受，并在接受时认领 invitee_user_id。
func TestAcceptInvitation_IdentifierClaim(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	// #2228 P1：商户级邀请接受**现在会绑定组织关系 + 补授角色模板**（#2032/#2228 修复），
	// 需挂 mock IAM（bind + namespaces/role-templates + users 端点）。
	srv := newMockIAMServer(nil, nil)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.StaffInvite{}, &models.User{}, &models.SiteMember{}))

	mySub, myLocal := uuid.New().String(), uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: myLocal, IAMSub: mySub, Phone: "13800000011",
		TenantID: uuid.New().String(), OrgID: uuid.New().String(), Status: "active",
	}).Error)

	// SiteID=nil（#2114 商户/平台级邀请）；#2228 P1 修复后接受即绑定 org（mock IAM 应答），
	// 本用例聚焦：identifier 匹配通过 loader + 接受时认领 invitee_user_id。
	inv := models.StaffInvite{
		ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: uuid.New().String(),
		SiteID: nil, Role: "merchant_member", Code: uuid.New().String()[:16],
		Status: "pending", ExpiresAt: time.Now().Add(time.Hour), InviteeIdentifier: "13800000011",
	}
	require.NoError(t, db.Create(&inv).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, mySub)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/api/user/invitations/:id/accept", AcceptInvitation)

	req := httptest.NewRequest("POST", "/api/user/invitations/"+inv.ID+"/accept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var stored models.StaffInvite
	require.NoError(t, db.Where("id = ?", inv.ID).First(&stored).Error)
	require.NotNil(t, stored.InviteeUserID)
	assert.Equal(t, myLocal, *stored.InviteeUserID, "接受时应认领 invitee_user_id")
	assert.Equal(t, "accepted", stored.Status)
}
