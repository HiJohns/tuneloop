package handlers

// #2546：商户所有人（唯一 OWNER）+ 转让所有权（原 OWNER 降 ADMIN）。
//
// 本地所有权权威 = merchants.admin_uid（D1）；IAM 是关系角色权威，转让只有一个
// 出口（POST /organizations/:id/transfer-ownership，D2/D5）。本文件覆盖：
//   R1 转让成功（IAM 契约 + 本地同步）
//   R2 IAM 失败映射（403/404/409/400/5xx）+ 本地不变
//   R3 前置拒绝（不调 IAM）
//   R4 添加/改角色只 bind ADMIN，OWNER 请求被拒
//   R5 OWNER 移除/降级保护 + 非 owner 管理员回归
//   R6 编辑商户改所有人走转让（含置空拒绝）
//   R7 列表 owner 标记 + include=all 候选

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

type ownershipIAMMock2546 struct {
	mu sync.Mutex

	transferCalls int
	bindCalls     int
	roleCalls     int
	unbindCalls   int

	transferStatus int // 0 == 200
	bindStatus     int
	roleStatus     int

	lastMethod       string
	lastTransferPath string
	lastTransferBody string
	bindRoles        []string
}

func transferMessage2546(status int) string {
	switch status {
	case http.StatusForbidden:
		return "only the current owner can transfer ownership"
	case http.StatusNotFound:
		return "target user is not an active member of this organization"
	case http.StatusConflict:
		return "organization has no active owner"
	case http.StatusBadRequest:
		return "user is already the owner"
	default:
		return "failed to commit ownership transfer"
	}
}

func setupOwnershipIAMMock2546(t *testing.T, m *ownershipIAMMock2546) {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	statusOf := func(code int) int {
		if code == 0 {
			return http.StatusOK
		}
		return code
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "svc-token-2546", "expires_in": 3600})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/transfer-ownership"):
			m.mu.Lock()
			m.transferCalls++
			m.lastMethod = r.Method
			m.lastTransferPath = r.URL.Path
			m.lastTransferBody = string(raw)
			status := statusOf(m.transferStatus)
			m.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				writeJSON(w, map[string]string{"error": transferMessage2546(status)})
				return
			}
			writeJSON(w, map[string]interface{}{
				"org_id":            "org",
				"previous_owner_id": "prev",
				"new_owner_id":      "next",
			})
		case strings.HasSuffix(r.URL.Path, "/role"):
			m.mu.Lock()
			m.roleCalls++
			status := statusOf(m.roleStatus)
			m.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				writeJSON(w, map[string]string{"error": "role endpoint unavailable"})
				return
			}
			writeJSON(w, map[string]interface{}{"code": 20000})
		default:
			var req struct {
				Action string `json:"action"`
				Role   string `json:"role"`
			}
			_ = json.Unmarshal(raw, &req)
			m.mu.Lock()
			if req.Action == "unbind" {
				m.unbindCalls++
			} else {
				m.bindCalls++
				m.bindRoles = append(m.bindRoles, req.Role)
			}
			status := statusOf(m.bindStatus)
			m.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				writeJSON(w, map[string]string{"error": "bind endpoint unavailable"})
				return
			}
			writeJSON(w, map[string]interface{}{"code": 20000})
		}
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/role-templates") {
			writeJSON(w, []map[string]string{
				{"id": "tmpl-merchant_admin", "code": "merchant_admin"},
				{"id": "tmpl-site_member", "code": "site_member"},
				{"id": "tmpl-merchant_staff", "code": "merchant_staff"},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"id": "ns-2546"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
}

type ownershipFixture2546 struct {
	tenantID   string
	merchantID string
	orgID      string

	ownerID  string
	adminID  string
	staffID  string
	ownerSub string
	adminSub string
	staffSub string
}

// seedOwnershipFixture2546 builds: merchant M (tenant=shared, org=anchor,
// admin_uid=owner A) + members A (merchant_admin), B (merchant_admin),
// E (merchant_staff). The request actor is the merchant root org (tid == oid).
func seedOwnershipFixture2546(t *testing.T) ownershipFixture2546 {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	f := ownershipFixture2546{
		orgID: uuid.NewString(),
	}
	// 商户根组织即其 tenant（merchant_admin 判定 tid == oid）；共享 tenant 形态由
	// #2250/#2274 的读锚用例覆盖，不在本单范围。
	f.tenantID = f.orgID
	testfixtures.SeedTenantFK(t, db, f.tenantID)

	seedUser := func() (string, string) {
		id := uuid.NewString()
		sub := uuid.NewString()
		require.NoError(t, db.Create(&models.User{
			ID: id, IAMSub: sub, TenantID: f.orgID, OrgID: f.orgID,
			Username: "u-" + uuid.NewString()[:8], Name: "member-" + id[:8],
			Email: id[:8] + "@example.test", Role: "merchant_admin", Status: "active",
		}).Error)
		return id, sub
	}

	f.ownerID, f.ownerSub = seedUser()
	f.adminID, f.adminSub = seedUser()
	f.staffID, f.staffSub = seedUser()

	f.merchantID = uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: f.merchantID, TenantID: f.tenantID, OrgID: f.orgID, Name: "M2546",
		AdminUID: f.ownerID, AdminPending: false, Status: "active", MerchantType: "full",
	}).Error)

	for _, row := range []struct {
		userID string
		role   string
	}{
		{f.ownerID, "merchant_admin"},
		{f.adminID, "merchant_admin"},
		{f.staffID, "merchant_staff"},
	} {
		require.NoError(t, db.Create(&models.MerchantMember{
			TenantID: f.orgID, MerchantID: f.merchantID, UserID: row.userID,
			Role: row.role, Status: "active",
		}).Error)
	}

	return f
}

func ownershipRouter2546(t *testing.T, tenantID, orgID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: uuid.NewString(), Role: "OWNER"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	members := &MerchantMemberHandler{}
	merchants := &MerchantHandler{}
	r.POST("/merchants/:id/transfer-ownership", members.TransferOwnership)
	r.PUT("/merchants/:id", merchants.UpdateMerchant)
	r.GET("/admin/merchants/:id/members", members.ListMembers)
	r.POST("/admin/merchants/:id/members", members.AddMember)
	r.PUT("/admin/merchants/:id/members/:uid", members.UpdateMemberRole)
	r.DELETE("/admin/merchants/:id/members/:uid", members.RemoveMember)
	return r
}

func doOwnershipRequest2546(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-2546")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type ownershipResponse2546 struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		MerchantID      string `json:"merchant_id"`
		PreviousOwnerID string `json:"previous_owner_id"`
		NewOwnerID      string `json:"new_owner_id"`
		OwnerUID        string `json:"owner_uid"`
		Total           int64  `json:"total"`
		List            []struct {
			UserID  string `json:"user_id"`
			Role    string `json:"role"`
			Status  string `json:"status"`
			IsOwner bool   `json:"is_owner"`
		} `json:"list"`
		DirectlyAdded []map[string]interface{} `json:"directly_added"`
	} `json:"data"`
}

func decodeOwnership2546(t *testing.T, w *httptest.ResponseRecorder) ownershipResponse2546 {
	t.Helper()
	var resp ownershipResponse2546
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	return resp
}

// R1：转让成功 —— IAM 只调 POST /transfer-ownership，本地指针与业务角色同步。
func TestTransferOwnershipSuccess_2546(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+f.adminID+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	resp := decodeOwnership2546(t, w)
	assert.Equal(t, 20000, resp.Code)
	assert.Equal(t, f.adminID, resp.Data.NewOwnerID, "NEW_OWNER_ID")
	assert.Equal(t, f.ownerID, resp.Data.PreviousOwnerID, "PREVIOUS_OWNER_ID")
	ownerFieldPresent := strings.Contains(w.Body.String(), `"new_owner_id"`) &&
		strings.Contains(w.Body.String(), `"previous_owner_id"`)
	assert.True(t, ownerFieldPresent, "OWNER_FIELD_PRESENT")

	mock.mu.Lock()
	transferCalls := mock.transferCalls
	method := mock.lastMethod
	path := mock.lastTransferPath
	body := mock.lastTransferBody
	bindCalls := mock.bindCalls
	unbindCalls := mock.unbindCalls
	mock.mu.Unlock()

	assert.Equal(t, 1, transferCalls, "IAM_TRANSFER_CALLS=1")
	assert.Equal(t, http.MethodPost, method, "IAM_METHOD=POST")
	assert.True(t, strings.HasSuffix(path, "/transfer-ownership"), "IAM_PATH_SUFFIX=%s", path)
	assert.Equal(t, 0, bindCalls, "OWNER_BINDS=0（不得再走 bind 写 OWNER）")
	assert.Equal(t, 0, unbindCalls, "IAM_UNBIND_CALLS=0")

	var iamBody struct {
		UserID string `json:"user_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &iamBody))
	assert.Equal(t, f.adminSub, iamBody.UserID, "IAM_BODY_USER_ID（IAM 侧 sub，非本地 id）")

	var merchant models.Merchant
	require.NoError(t, database.GetDB().Where("id = ?", f.merchantID).First(&merchant).Error)
	assert.Equal(t, f.adminID, merchant.AdminUID, "LOCAL_OWNER_UPDATED=1")
	assert.False(t, merchant.AdminPending, "ADMIN_PENDING_CLEARED=1")

	var oldOwnerRow, newOwnerRow models.MerchantMember
	require.NoError(t, database.GetDB().Where("merchant_id = ? AND user_id = ?", f.merchantID, f.ownerID).First(&oldOwnerRow).Error)
	require.NoError(t, database.GetDB().Where("merchant_id = ? AND user_id = ?", f.merchantID, f.adminID).First(&newOwnerRow).Error)
	assert.Equal(t, "merchant_admin", oldOwnerRow.Role, "OLD_OWNER_LOCAL_ROLE=merchant_admin（业务角色不变）")
	assert.Equal(t, "merchant_admin", newOwnerRow.Role, "TARGET_LOCAL_ROLE=merchant_admin")

	t.Logf("TRANSFER_STATUS=%d IAM_METHOD=%s IAM_PATH_SUFFIX=%s IAM_BODY_USER_ID=1 LOCAL_OWNER_UPDATED=1 OLD_OWNER_LOCAL_ROLE=%s TARGET_LOCAL_ROLE=%s ADMIN_PENDING_CLEARED=1 OWNER_FIELD_PRESENT=1",
		w.Code, method, "/transfer-ownership", oldOwnerRow.Role, newOwnerRow.Role)
}

// R2：IAM 失败映射 —— 403/404/409/400/5xx 一律显式返回，且本地指针不变。
func TestTransferOwnershipIAMErrorMapping_2546(t *testing.T) {
	type codePair struct {
		status int
		code   int
	}
	cases := []codePair{
		{http.StatusForbidden, 40303},
		{http.StatusNotFound, 40400},
		{http.StatusConflict, 40900},
		{http.StatusBadRequest, 40002},
		{http.StatusInternalServerError, 50200},
	}

	results := make(map[int]codePair, len(cases))
	unchanged := true

	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			f := seedOwnershipFixture2546(t)
			mock := &ownershipIAMMock2546{transferStatus: tc.status}
			setupOwnershipIAMMock2546(t, mock)

			r := ownershipRouter2546(t, f.orgID, f.orgID)
			w := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
				`{"user_id":"`+f.adminID+`"}`)

			// 5xx → 50200 (Bad Gateway) ; 4xx keeps its own status.
			expectedHTTP := tc.status
			if tc.status >= 500 {
				expectedHTTP = http.StatusBadGateway
			}
			require.Equal(t, expectedHTTP, w.Code, "IAM %d 必须显式失败：%s", tc.status, w.Body.String())
			resp := decodeOwnership2546(t, w)
			assert.Equal(t, tc.code, resp.Code, "IAM %d → code %d", tc.status, tc.code)

			var merchant models.Merchant
			require.NoError(t, database.GetDB().Where("id = ?", f.merchantID).First(&merchant).Error)
			if merchant.AdminUID != f.ownerID {
				unchanged = false
			}
			assert.Equal(t, f.ownerID, merchant.AdminUID, "LOCAL_OWNER_UNCHANGED=1（IAM 失败不得改本地）")

			mock.mu.Lock()
			assert.Equal(t, 1, mock.transferCalls, "IAM_TRANSFER_CALLS=1")
			mock.mu.Unlock()

			results[tc.status] = codePair{status: w.Code, code: resp.Code}
			t.Logf("IAM%d_STATUS=%d IAM%d_CODE=%d", tc.status, w.Code, tc.status, resp.Code)
		})
	}

	assert.True(t, unchanged, "LOCAL_OWNER_UNCHANGED=1")
	t.Logf("IAM403_STATUS=%d IAM403_CODE=%d IAM404_STATUS=%d IAM404_CODE=%d IAM409_STATUS=%d IAM409_CODE=%d IAM400_CODE=%d IAM500_STATUS=%d IAM500_CODE=%d LOCAL_OWNER_UNCHANGED=1",
		results[http.StatusForbidden].status, results[http.StatusForbidden].code,
		results[http.StatusNotFound].status, results[http.StatusNotFound].code,
		results[http.StatusConflict].status, results[http.StatusConflict].code,
		results[http.StatusBadRequest].code,
		results[http.StatusInternalServerError].status, results[http.StatusInternalServerError].code)
}

// R3：前置拒绝 —— 非本商户成员 / 目标已是 owner / user_id 空，均不调 IAM。
func TestTransferOwnershipPreflightNegatives_2546(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	r := ownershipRouter2546(t, f.orgID, f.orgID)

	// 非本商户成员（平台用户但无 merchant_members 行）
	outsider := uuid.NewString()
	require.NoError(t, database.GetDB().Create(&models.User{
		ID: outsider, IAMSub: uuid.NewString(), TenantID: f.orgID, OrgID: f.orgID,
		Username: "u-" + outsider[:8], Role: "site_member", Status: "active",
	}).Error)
	wNonMember := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+outsider+`"}`)
	nonMember := decodeOwnership2546(t, wNonMember)

	mock.mu.Lock()
	callsAfterNonMember := mock.transferCalls
	mock.mu.Unlock()

	// 目标已是当前 owner
	wSame := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+f.ownerID+`"}`)
	sameOwner := decodeOwnership2546(t, wSame)

	mock.mu.Lock()
	callsAfterSame := mock.transferCalls
	mock.mu.Unlock()

	// user_id 空
	wEmpty := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership", `{}`)
	emptyUID := decodeOwnership2546(t, wEmpty)

	mock.mu.Lock()
	callsAfterEmpty := mock.transferCalls
	mock.mu.Unlock()

	require.Equal(t, http.StatusNotFound, wNonMember.Code, wNonMember.Body.String())
	assert.Equal(t, 40400, nonMember.Code)
	assert.Equal(t, 0, callsAfterNonMember, "NONMEMBER_IAM_CALLS=0")
	assert.Equal(t, http.StatusBadRequest, wSame.Code, wSame.Body.String())
	assert.Equal(t, 40002, sameOwner.Code)
	assert.Equal(t, 0, callsAfterSame, "SAME_OWNER_IAM_CALLS=0")
	assert.Equal(t, http.StatusBadRequest, wEmpty.Code, wEmpty.Body.String())
	assert.Equal(t, 40001, emptyUID.Code)
	assert.Equal(t, 0, callsAfterEmpty, "EMPTY_UID_IAM_CALLS=0")

	var merchant models.Merchant
	require.NoError(t, database.GetDB().Where("id = ?", f.merchantID).First(&merchant).Error)
	assert.Equal(t, f.ownerID, merchant.AdminUID, "LOCAL_OWNER_UNCHANGED=1")

	t.Logf("NONMEMBER_STATUS=%d NONMEMBER_CODE=%d NONMEMBER_IAM_CALLS=%d SAME_OWNER_STATUS=%d SAME_OWNER_CODE=%d SAME_OWNER_IAM_CALLS=%d EMPTY_UID_STATUS=%d EMPTY_UID_CODE=%d",
		wNonMember.Code, nonMember.Code, callsAfterNonMember,
		wSame.Code, sameOwner.Code, callsAfterSame,
		wEmpty.Code, emptyUID.Code)
}

// R4：添加成员 / 改角色只 bind ADMIN；显式 OWNER 请求被拒且不写 IAM。
func TestMerchantMemberRoleBindsAdmin_2546(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	r := ownershipRouter2546(t, f.orgID, f.orgID)

	// 添加一个新管理员（非 owner）
	newAdminID := uuid.NewString()
	require.NoError(t, database.GetDB().Create(&models.User{
		ID: newAdminID, IAMSub: uuid.NewString(), TenantID: f.orgID, OrgID: f.orgID,
		Username: "u-" + newAdminID[:8], Role: "site_member", Status: "active",
	}).Error)
	wAdd := doOwnershipRequest2546(t, r, http.MethodPost, "/admin/merchants/"+f.merchantID+"/members",
		`{"user_id":"`+newAdminID+`","role":"merchant_admin"}`)
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, wAdd.Code, wAdd.Body.String())

	// 改角色：直属员工 → merchant_admin
	wUpdate := doOwnershipRequest2546(t, r, http.MethodPut, "/admin/merchants/"+f.merchantID+"/members/"+f.staffID,
		`{"role":"merchant_admin"}`)
	require.Equal(t, http.StatusOK, wUpdate.Code, wUpdate.Body.String())

	mock.mu.Lock()
	roles := append([]string(nil), mock.bindRoles...)
	bindCallsBeforeOwnerRequest := mock.bindCalls
	mock.mu.Unlock()

	// 显式 OWNER 请求必须 400 且不写 IAM
	wOwner := doOwnershipRequest2546(t, r, http.MethodPost, "/admin/merchants/"+f.merchantID+"/members",
		`{"user_id":"`+newAdminID+`","role":"OWNER"}`)
	ownerResp := decodeOwnership2546(t, wOwner)

	mock.mu.Lock()
	bindCallsAfterOwnerRequest := mock.bindCalls
	mock.mu.Unlock()

	require.Equal(t, http.StatusBadRequest, wOwner.Code, wOwner.Body.String())
	assert.Equal(t, 40002, ownerResp.Code)
	assert.Equal(t, bindCallsBeforeOwnerRequest, bindCallsAfterOwnerRequest, "OWNER 请求不得调 IAM bind")

	ownerBinds := 0
	for _, role := range roles {
		if strings.EqualFold(role, "OWNER") {
			ownerBinds++
		}
	}
	require.Len(t, roles, 2, "ADD/UPDATE 各一次 bind，实际 %v", roles)
	assert.Equal(t, "ADMIN", roles[0], "ADD_BIND_ROLE=ADMIN")
	assert.Equal(t, "ADMIN", roles[1], "UPDATE_BIND_ROLE=ADMIN")
	assert.Equal(t, 0, ownerBinds, "OWNER_BINDS=0")

	t.Logf("ADD_BIND_ROLE=%s UPDATE_BIND_ROLE=%s OWNER_BINDS=%d OWNER_REQUEST_STATUS=%d OWNER_REQUEST_CODE=%d",
		roles[0], roles[1], ownerBinds, wOwner.Code, ownerResp.Code)
}

// R5：OWNER 不能移除/降级（不调 IAM）；非 owner 管理员沿用既有移除路径。
func TestMerchantOwnerProtection_2546(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	r := ownershipRouter2546(t, f.orgID, f.orgID)

	wRemoveOwner := doOwnershipRequest2546(t, r, http.MethodDelete,
		"/admin/merchants/"+f.merchantID+"/members/"+f.ownerID, "")
	removeOwner := decodeOwnership2546(t, wRemoveOwner)

	mock.mu.Lock()
	callsAfterRemoveOwner := mock.unbindCalls + mock.bindCalls + mock.roleCalls + mock.transferCalls
	mock.mu.Unlock()

	wDemoteOwner := doOwnershipRequest2546(t, r, http.MethodPut,
		"/admin/merchants/"+f.merchantID+"/members/"+f.ownerID, `{"role":"merchant_staff"}`)
	demoteOwner := decodeOwnership2546(t, wDemoteOwner)

	var ownerRow models.MerchantMember
	require.NoError(t, database.GetDB().Where("merchant_id = ? AND user_id = ?", f.merchantID, f.ownerID).First(&ownerRow).Error)

	wRemoveAdmin := doOwnershipRequest2546(t, r, http.MethodDelete,
		"/admin/merchants/"+f.merchantID+"/members/"+f.adminID, "")

	require.Equal(t, http.StatusForbidden, wRemoveOwner.Code, wRemoveOwner.Body.String())
	assert.Equal(t, 40303, removeOwner.Code, "REMOVE_OWNER_CODE=40303")
	assert.Equal(t, 0, callsAfterRemoveOwner, "REMOVE_OWNER_IAM_CALLS=0")
	assert.NotEmpty(t, ownerRow.ID, "OWNER_ROW_KEPT=1")
	assert.Equal(t, "merchant_admin", ownerRow.Role)
	assert.Equal(t, http.StatusBadRequest, wDemoteOwner.Code, wDemoteOwner.Body.String())
	assert.Equal(t, 40002, demoteOwner.Code, "DEMOTE_OWNER_CODE=40002")
	assert.Equal(t, http.StatusOK, wRemoveAdmin.Code, wRemoveAdmin.Body.String())

	t.Logf("REMOVE_OWNER_STATUS=%d REMOVE_OWNER_CODE=%d REMOVE_OWNER_IAM_CALLS=%d OWNER_ROW_KEPT=1 DEMOTE_OWNER_STATUS=%d DEMOTE_OWNER_CODE=%d REMOVE_ADMIN_STATUS=%d",
		wRemoveOwner.Code, removeOwner.Code, callsAfterRemoveOwner,
		wDemoteOwner.Code, demoteOwner.Code, wRemoveAdmin.Code)
}

// R6：编辑商户改 admin_uid 走同一个转让端点（翻转 #2461 的 bind+demote 断言）。
func TestUpdateMerchantOwnershipTransfer_2546(t *testing.T) {
	t.Run("transfer", func(t *testing.T) {
		f := seedOwnershipFixture2546(t)
		mock := &ownershipIAMMock2546{}
		setupOwnershipIAMMock2546(t, mock)
		r := ownershipRouter2546(t, f.orgID, f.orgID)

		w := doOwnershipRequest2546(t, r, http.MethodPut, "/merchants/"+f.merchantID,
			`{"admin_uid":"`+f.adminID+`"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		mock.mu.Lock()
		transferCalls := mock.transferCalls
		path := mock.lastTransferPath
		bindCalls := mock.bindCalls
		unbindCalls := mock.unbindCalls
		mock.mu.Unlock()
		assert.Equal(t, 1, transferCalls, "UPD_ADMIN_IAM_TRANSFER_CALLS=1")
		assert.True(t, strings.HasSuffix(path, "/transfer-ownership"), "UPD_ADMIN_IAM_PATH_SUFFIX=%s", path)
		assert.Equal(t, 0, bindCalls, "UPD_ADMIN_BIND_CALLS=0")
		assert.Equal(t, 0, unbindCalls, "UPD_ADMIN_UNBIND_CALLS=0")

		var merchant models.Merchant
		require.NoError(t, database.GetDB().Where("id = ?", f.merchantID).First(&merchant).Error)
		assert.Equal(t, f.adminID, merchant.AdminUID, "UPD_ADMIN_LOCAL=1")

		wList := doOwnershipRequest2546(t, r, http.MethodGet, "/admin/merchants/"+f.merchantID+"/members?include=all", "")
		require.Equal(t, http.StatusOK, wList.Code, wList.Body.String())
		list := decodeOwnership2546(t, wList)
		flip := false
		for _, row := range list.Data.List {
			if row.UserID == f.adminID && row.IsOwner {
				flip = true
			}
			if row.UserID == f.ownerID && row.IsOwner {
				flip = false
			}
		}
		assert.True(t, flip, "UPD_ADMIN_IS_OWNER_FLIP=1")

		t.Logf("UPD_ADMIN_STATUS=%d UPD_ADMIN_IAM_PATH_SUFFIX=%s UPD_ADMIN_LOCAL=1 UPD_ADMIN_IS_OWNER_FLIP=1",
			w.Code, "/transfer-ownership")
	})

	t.Run("clear-rejected", func(t *testing.T) {
		f := seedOwnershipFixture2546(t)
		mock := &ownershipIAMMock2546{}
		setupOwnershipIAMMock2546(t, mock)
		r := ownershipRouter2546(t, f.orgID, f.orgID)

		w := doOwnershipRequest2546(t, r, http.MethodPut, "/merchants/"+f.merchantID, `{"admin_uid":""}`)
		resp := decodeOwnership2546(t, w)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Equal(t, 40002, resp.Code, "CLEAR_OWNER_CODE=40002")

		mock.mu.Lock()
		transferCalls := mock.transferCalls
		mock.mu.Unlock()
		assert.Equal(t, 0, transferCalls, "CLEAR_OWNER_IAM_CALLS=0")

		var merchant models.Merchant
		require.NoError(t, database.GetDB().Where("id = ?", f.merchantID).First(&merchant).Error)
		assert.Equal(t, f.ownerID, merchant.AdminUID, "CLEAR_OWNER_LOCAL_UNCHANGED=1")

		t.Logf("CLEAR_OWNER_STATUS=%d CLEAR_OWNER_CODE=%d", w.Code, resp.Code)
	})
}

// R7：列表 owner 标记 + include=all 候选（默认口径不变：仅 merchant_admin）。
func TestListMembersOwnerFlag_2546(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	r := ownershipRouter2546(t, f.orgID, f.orgID)

	wDefault := doOwnershipRequest2546(t, r, http.MethodGet, "/admin/merchants/"+f.merchantID+"/members", "")
	require.Equal(t, http.StatusOK, wDefault.Code, wDefault.Body.String())
	defaultList := decodeOwnership2546(t, wDefault)

	wAll := doOwnershipRequest2546(t, r, http.MethodGet, "/admin/merchants/"+f.merchantID+"/members?include=all", "")
	require.Equal(t, http.StatusOK, wAll.Code, wAll.Body.String())
	allList := decodeOwnership2546(t, wAll)

	ownerHits := 0
	for _, row := range defaultList.Data.List {
		if row.IsOwner {
			ownerHits++
		}
	}
	assert.Equal(t, 1, ownerHits, "OWNER_FLAG_HITS=1")
	assert.Equal(t, f.ownerID, defaultList.Data.OwnerUID, "OWNER_UID_MATCH=1")
	assert.Equal(t, int64(2), defaultList.Data.Total, "DEFAULT_ROWS=2")
	assert.Equal(t, int64(3), allList.Data.Total, "ALL_ROWS=3")
	ownerFieldPresent := strings.Contains(wDefault.Body.String(), `"is_owner"`) &&
		strings.Contains(wDefault.Body.String(), `"owner_uid"`)
	assert.True(t, ownerFieldPresent, "OWNER_FIELD_PRESENT=1")

	t.Logf("OWNER_FLAG_HITS=%d OWNER_UID_MATCH=1 DEFAULT_ROWS=%d ALL_ROWS=%d OWNER_FIELD_PRESENT=1",
		ownerHits, defaultList.Data.Total, allList.Data.Total)
}
