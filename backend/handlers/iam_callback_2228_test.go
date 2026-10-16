package handlers

// #2228 P1：IAM 事件回调接线——POST /api/iam/callback。
//   - 未注册路由 → 404（红）；注册后 HMAC 校验 + user.activated 完成邀请绑定（绿）
//   - user.activated：本地用户补建（如缺失）+ pending 定向邀请绑定（site_members +
//     角色模板 #2032）+ 标记 accepted

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postIAMCallback2228(t *testing.T, secret string, body map[string]interface{}) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/iam/callback", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-IAM-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-IAM-Event-ID", uuid.New().String())
	r := gin.New()
	r.POST("/api/iam/callback", HandleIAMCallback)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestIAMCallback_UserActivated_2228(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	// 记录角色模板赋值调用
	assigned := make(chan string, 8)
	srv := newMockIAMServer(nil, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/") {
			select {
			case assigned <- r.URL.Path:
			default:
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "message": "success"})
	})
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	require.NoError(t, os.Setenv("IAM_CALLBACK_SECRET", "test-callback-secret-2228"))
	defer os.Unsetenv("IAM_CALLBACK_SECRET")

	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.StaffInvite{}, &models.User{}, &models.SiteMember{}, &models.Site{}))

	// 预建账户（init 激活前形态：tuneloop 本地已建行）+ 网点 + pending 定向邀请
	sub := uuid.New().String()
	localID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: localID, IAMSub: sub, Phone: "13800002228", Name: "受邀人",
		TenantID: uuid.New().String(), OrgID: uuid.New().String(), Status: "active",
	}).Error)
	siteID := uuid.New().String()
	orgID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: uuid.New().String(), OrgID: orgID, Name: "S2228"}).Error)
	inv := models.StaffInvite{
		ID: uuid.New().String(), TenantID: uuid.New().String(), OrgID: orgID,
		SiteID: &siteID, Role: "repair_technician", Code: uuid.New().String()[:16],
		Status: "pending", ExpiresAt: time.Now().Add(time.Hour),
		InviteeIdentifier: "13800002228",
	}
	require.NoError(t, db.Create(&inv).Error)

	// 1) 错误签名 → 401（不处理）
	code, _ := postIAMCallback2228(t, "wrong-secret", map[string]interface{}{
		"action": "user.activated", "payload": map[string]interface{}{"user_id": sub},
	})
	assert.Equal(t, http.StatusUnauthorized, code)

	// 2) 正确签名 → 激活绑定成功
	code, body := postIAMCallback2228(t, "test-callback-secret-2228", map[string]interface{}{
		"action":    "user.activated",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"payload":   map[string]interface{}{"user_id": sub},
	})
	require.Equal(t, http.StatusOK, code, "body=%s", body)

	// 邀请已接受 + 认领
	var stored models.StaffInvite
	require.NoError(t, db.Where("id = ?", inv.ID).First(&stored).Error)
	assert.Equal(t, "accepted", stored.Status)
	require.NotNil(t, stored.InviteeUserID)
	assert.Equal(t, localID, *stored.InviteeUserID)

	// site_members 已建立
	var n int64
	require.NoError(t, db.Model(&models.SiteMember{}).
		Where("site_id = ? AND user_id = ?", siteID, localID).Count(&n).Error)
	assert.Equal(t, int64(1), n, "激活回调应建立网点成员关系")

	// 角色模板已赋值（repair_technician 在 mock 模板列表中 → 应调用 users roles 端点）
	select {
	case p := <-assigned:
		assert.Contains(t, p, "/users/", "应调用角色模板赋值端点，实际=%s", p)
	case <-time.After(2 * time.Second):
		t.Fatal("未观察到角色模板赋值调用（#2032 修复未生效）")
	}

	// 3) 幂等重放（同一事件重发）→ 仍 200；邀请不重复处理
	code2, _ := postIAMCallback2228(t, "test-callback-secret-2228", map[string]interface{}{
		"action":  "user.activated",
		"payload": map[string]interface{}{"user_id": sub},
	})
	assert.Equal(t, http.StatusOK, code2)
	require.NoError(t, db.Model(&models.SiteMember{}).
		Where("site_id = ? AND user_id = ?", siteID, localID).Count(&n).Error)
	assert.Equal(t, int64(1), n, "重放不得重复建成员")
}

func TestIAMCallback_Activated_NoInvite_2228(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	srv := newMockIAMServer(nil, nil)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	require.NoError(t, os.Setenv("IAM_CALLBACK_SECRET", "test-callback-secret-2228b"))
	defer os.Unsetenv("IAM_CALLBACK_SECRET")

	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.StaffInvite{}, &models.User{}, &models.SiteMember{}))

	// 无本地用户 + 无邀请 → 补建用户并 200（no-op 绑定）
	sub := uuid.New().String()
	code, body := postIAMCallback2228(t, "test-callback-secret-2228b", map[string]interface{}{
		"action":  "user.activated",
		"payload": map[string]interface{}{"user_id": sub, "phone": "13800003228", "name": "无邀请"},
	})
	require.Equal(t, http.StatusOK, code, "body=%s", body)

	var u models.User
	require.NoError(t, db.Where("iam_sub = ?", sub).First(&u).Error)
	assert.Equal(t, "13800003228", u.Phone)
	assert.Equal(t, "active", u.Status)
}
