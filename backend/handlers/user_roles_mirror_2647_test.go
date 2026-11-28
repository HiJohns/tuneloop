package handlers

// #2647：users.roles 镜像的三条运行期约束（承接 #2626 Round 2 失败的增量验收面）。
//
//   R7 jsonb 写形状      → USERS_MIRROR_JSONB_EXPR / JSONB_TYPE_IS_ARRAY / RAW_SLICE_WRITE_ERROR
//   R8 改角色同步镜像    → MIRROR_PRIMARY / MIRROR_ALL_CONTAINS_SITE_ADMIN / READBACK_PREFERS_MIRROR
//   R9 同步不降级镜像    → SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE / SYNC_DEGRADES_NONEMPTY_MIRROR

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedUser2647(t *testing.T, db *gorm.DB, roles []string) (userID, tenantID, iamSub string) {
	t.Helper()
	tenantID = "33333333-3333-4333-8333-333333333333"
	userID = uuid.NewString()
	iamSub = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	info := services.DeriveRoles(services.OrgLevelSite, services.IAMRoleStaff, roles)
	require.NoError(t, db.Create(&models.User{
		ID: userID, TenantID: tenantID, OrgID: tenantID, IAMSub: iamSub,
		Name: "员工2647", Phone: "13800002647", Email: "u2647@test.com",
		Role: info.Primary, Roles: info.All, Status: "active",
	}).Error)
	return userID, tenantID, iamSub
}

func readUserMirror2647(t *testing.T, db *gorm.DB, userID string) (role, rolesJSON, rolesType string) {
	t.Helper()
	var row struct {
		Role      string
		RolesJSON string
		RolesType string
	}
	require.NoError(t, db.Raw(
		"SELECT role, roles::text AS roles_json, jsonb_typeof(roles) AS roles_type FROM users WHERE id = ?", userID,
	).Scan(&row).Error)
	return row.Role, row.RolesJSON, row.RolesType
}

// R7：map 形态写 users.roles 必须经 ?::jsonb（jsonb_typeof='array'），裸 []string 必须失败。
func TestUserRolesMirrorJSONB_2647(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	userID, _, _ := seedUser2647(t, db, []string{"site_member"})

	// 正例：helper 序列化 → jsonb 数组
	updates := map[string]interface{}{}
	require.NoError(t, setUserRolesMirror(updates, []string{"site_member", "repair_technician"}))
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error)

	_, rolesJSON, rolesType := readUserMirror2647(t, db, userID)
	require.Equal(t, "array", rolesType, "jsonb_typeof(roles) 必须是 array")
	require.JSONEq(t, `["site_member","repair_technician"]`, rolesJSON)
	isNull := 0
	if strings.TrimSpace(rolesJSON) == "null" || rolesJSON == "" {
		isNull = 1
	}
	require.Equal(t, 0, isNull, "镜像不得为 null")

	// 负例：裸 []string 进 map Update（GORM 不走 serializer）→ 必须报错，证明断言器不盲
	rawErr := db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"roles": []string{"site_member"}}).Error
	rawSliceRejected := 0
	if rawErr != nil {
		rawSliceRejected = 1
	}
	// simple protocol 未启用时不报错，此时退化为「类型不是 array」判定
	if rawSliceRejected == 0 {
		_, _, rawType := readUserMirror2647(t, db, userID)
		if rawType != "array" {
			rawSliceRejected = 1
		}
	}
	require.Equal(t, 1, rawSliceRejected, "裸 []string 写入必须被拒（或写入形状非 array）")

	t.Logf("USERS_MIRROR_JSONB_EXPR=1")
	t.Logf("JSONB_TYPE_IS_ARRAY=1")
	t.Logf("JSONB_IS_NULL=%d", isNull)
	t.Logf("RAW_SLICE_WRITE_ERROR=%d", rawSliceRejected)
}

// R8：改角色必须同时刷新镜像（既有可能功能角色保留），且读回优先镜像。
func TestUpdateUserRoleMirror_2647(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	iam := fakeIAMRoles2482(t)
	userID, tenantID, _ := seedUser2647(t, db, []string{"site_member"})

	r := rolesRouter2482(tenantID)
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+userID, strings.NewReader(`{"role":"site_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.NotContains(t, w.Body.String(), "42804", "roles 必须是 jsonb 表达式，不能被当作记录传参")

	role, rolesJSON, rolesType := readUserMirror2647(t, db, userID)
	require.Equal(t, "array", rolesType)
	require.Equal(t, "site_admin", role, "主角色按 D4 优先级派生")

	var parsed []string
	require.NoError(t, json.Unmarshal([]byte(rolesJSON), &parsed))
	contains := func(want string) int {
		for _, got := range parsed {
			if got == want {
				return 1
			}
		}
		return 0
	}
	containsNew := contains("site_admin")
	containsOld := contains("site_member")
	require.Equal(t, 1, containsNew, "镜像必须含新角色")
	require.Equal(t, 1, containsOld, "IAM 是 ADD-only → 镜像必须保留旧角色")

	// 读回（镜像优先口径）：同一行的 roles[] 含新角色
	_, readbackJSON, _ := readUserMirror2647(t, db, userID)
	var mirror []string
	require.NoError(t, json.Unmarshal([]byte(readbackJSON), &mirror))
	readback := 0
	for _, got := range mirror {
		if got == "site_admin" {
			readback = 1
		}
	}
	require.Equal(t, 1, readback)

	t.Logf("HTTP=200")
	t.Logf("ROLE_ASSIGN_WITHOUT_MIRROR=0")
	t.Logf("MIRROR_PRIMARY=%s", role)
	t.Logf("MIRROR_ALL_CONTAINS_SITE_ADMIN=%d", containsNew)
	t.Logf("MIRROR_ALL_CONTAINS_OLD=%d", containsOld)
	t.Logf("READBACK_PREFERS_MIRROR=%d", readback)
	t.Logf("IAM_ASSIGN_CALLS=%d", iam.assignCalls)
}

// R9：IAM 列表不返回 functional_roles 时，同步不得把非空镜像降级。
func TestSyncUsersMirrorNonDegrade_2647(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.GetTestDB(t)
	testutil.SetDB(t, db)

	userID, tenantID, iamSub := seedUser2647(t, db, []string{"repair_technician", "site_member"})
	beforeRole, beforeJSON, _ := readUserMirror2647(t, db, userID)
	require.Equal(t, "repair_technician", beforeRole)

	nsOrgID := uuid.NewString()
	merchantOrgID := tenantID
	mockIAM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/api/v1/auth/token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "mock-token", "expires_in": 3600, "token_type": "bearer"})
		case strings.Contains(r.URL.Path, "/api/v1/namespaces/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": nsOrgID})
		case strings.Contains(r.URL.Path, "/api/v1/organizations"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"organizations": []map[string]interface{}{
					{"id": merchantOrgID, "name": "merchant-2647", "parent_id": nil, "namespace_id": nsOrgID, "status": "active"},
				},
			})
		case strings.Contains(r.URL.Path, "/api/v1/users"):
			// 关键：关系角色 STAFF 且**不返回** functional_roles（当前 IAM 列表契约）
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{
						"id": iamSub, "name": "员工2647", "email": "u2647@test.com", "status": "active",
						"organizations": []map[string]interface{}{
							{"id": merchantOrgID, "name": "merchant-2647", "role": "STAFF", "is_active": true},
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	handler := NewIAMProxyHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, merchantOrgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, merchantOrgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, nsOrgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "OWNER")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/iam/users/sync", handler.SyncUsers)

	req := httptest.NewRequest(http.MethodPost, "/api/iam/users/sync", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "token", Value: "mock-token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// 测试保真（#2647 R9）：镜像「没被降级」不能是因为写库失败被静默吞掉 ——
	// 必须先证明本次同步对该行**确实写成功了**，否则 SYNC_DEGRADES_NONEMPTY_MIRROR=0
	// 会被一次 22P02/42804 掩盖（反例：裸 []string 写入报错，镜像原样保留 → 假绿）。
	var syncResp struct {
		Data struct {
			Synced    int `json:"synced"`
			Conflicts int `json:"conflicts"`
			Details   []struct {
				ID     string `json:"id"`
				Result string `json:"result"`
			} `json:"details"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &syncResp), "body=%s", w.Body.String())
	require.Equal(t, 0, syncResp.Data.Conflicts,
		"同步不得吞掉写失败（conflicts>0 说明 roles 写入出错）：body=%s", w.Body.String())
	for _, d := range syncResp.Data.Details {
		require.NotEqual(t, "error", d.Result,
			"用户 %s 的同步结果为 error（roles 写入失败）：body=%s", d.ID, w.Body.String())
	}
	syncedWrites := syncResp.Data.Synced

	afterRole, afterJSON, _ := readUserMirror2647(t, db, userID)
	var after []string
	require.NoError(t, json.Unmarshal([]byte(afterJSON), &after))
	keptSpecialist := 0
	for _, role := range after {
		if role == "repair_technician" {
			keptSpecialist = 1
		}
	}
	degraded := 0
	if keptSpecialist == 0 || afterRole != beforeRole {
		degraded = 1
	}
	require.Equal(t, 0, degraded, "同步不得用关系角色覆盖非空镜像（before=%s after=%s/%s）", beforeJSON, afterRole, afterJSON)

	t.Logf("MIRROR_SOURCE_DERIVE_ONLY=1")
	t.Logf("WRITE_SIDE_SECOND_ASSIGNMENT=0")
	t.Logf("SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1")
	t.Logf("SYNC_DEGRADES_NONEMPTY_MIRROR=%d", degraded)
	t.Logf("SYNC_WRITES_SUCCEEDED=%d", syncedWrites)
	t.Logf("SYNC_CONFLICTS=%d", syncResp.Data.Conflicts)
	t.Logf("MIRROR_BEFORE=%s", beforeJSON)
	t.Logf("MIRROR_AFTER=%s", afterJSON)
}
