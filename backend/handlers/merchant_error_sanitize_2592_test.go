package handlers

// #2592：商户/成员链路的 5xx 文案必须收敛为固定字面量，诊断细节只进日志。
//
// 覆盖 Issue 表里 9 个运行时可达站点（另 2 处 merchant.go:632 / merchant_member.go:960
// 在当前类型流下防御性不可达，由 scripts/verify-2592.sh 的 R2/R3 以 static 方式断言）：
//
//	SITE=merchant.go:621                  PUT /merchants/:id（owner 解析失败）
//	SITE=merchant_member.go:55            GET  member list（owner 解析失败）
//	SITE=merchant_member.go:62            GET  member list（viewer 解析失败）
//	SITE=merchant_member.go:497           PUT  member role（owner 解析失败）
//	SITE=merchant_member.go:614           DELETE member（owner 解析失败）
//	SITE=merchant_member.go:811           POST transfer-ownership（owner 解析失败）
//	SITE=merchant_member.go:854           POST transfer-ownership（目标成员查询失败）
//	SITE=merchant_member.go:879           POST transfer-ownership（IAM 不可达，非类型化错误）
//	SITE=merchant_member.go:916           POST transfer-ownership（本地同步失败）
//
// 每个分支三条断言：响应只有固定文案、响应体不含诊断/内部 ID、日志仍保留原始 error。

import (
	"bytes"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

const (
	// sanitizeSentinel2592 is injected as the "original" DB error so both the
	// response body and the log can be checked for what happened to it.
	sanitizeSentinel2592 = "sentinel2592: injected db failure"

	litOwner2592    = "读取商户所有人信息失败，请稍后重试"
	litViewer2592   = "读取当前登录账号失败，请稍后重试"
	litTarget2592   = "查询目标成员失败，请稍后重试"
	litTransfer2592 = "转让所有权失败（IAM），请稍后重试"
	litSync2592     = "IAM 已转让，但本地同步失败，请联系平台人工核对"
)

// captureLog2592 redirects the standard logger for the duration of one test so
// the "diagnostics still reach the log" half of the contract can be asserted.
func captureLog2592(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// failQueryOn2592 injects a deterministic query failure for one table.
func failQueryOn2592(t *testing.T, table string) {
	t.Helper()
	db := database.GetDB()
	cbName := "test:inject_" + table + "_query_failure_2592_" + uuid.NewString()[:8]
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == table {
			_ = tx.AddError(errors.New(sanitizeSentinel2592))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(cbName) })
}

// failUpdateOn2592 injects a deterministic update failure for one table.
func failUpdateOn2592(t *testing.T, table, message string) {
	t.Helper()
	db := database.GetDB()
	cbName := "test:inject_" + table + "_update_failure_2592_" + uuid.NewString()[:8]
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == table {
			_ = tx.AddError(errors.New(message))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(cbName) })
}

// assertSanitized2592 asserts the user-facing response carries only the fixed
// literal (no diagnostics, no internal identifiers) while the log keeps the
// original error.
func assertSanitized2592(t *testing.T, w *httptest.ResponseRecorder, logBuf *bytes.Buffer, wantStatus, wantCode int, wantMessage, logNeedle string, forbiddenIDs ...string) {
	t.Helper()
	require.Equal(t, wantStatus, w.Code, w.Body.String())
	resp := decodeOwnership2546(t, w)
	assert.Equal(t, wantCode, resp.Code, "RESPONSE_CODE")
	assert.Equal(t, wantMessage, resp.Message, "FIXED_MESSAGE")

	body := w.Body.String()
	assert.NotContains(t, body, sanitizeSentinel2592, "LEAK_SENTINEL=0")
	assert.NotContains(t, body, "pq:", "LEAK_PQ=0")
	assert.NotContains(t, body, "connection refused", "LEAK_DIAL=0")
	assert.NotContains(t, body, "request failed", "LEAK_REQUEST_FAILED=0")
	for _, id := range forbiddenIDs {
		if id == "" {
			continue
		}
		assert.NotContains(t, body, id, "LEAK_INTERNAL_ID=0")
	}

	require.NotNil(t, logBuf, "LOG_CAPTURE_MISSING")
	assert.Contains(t, logBuf.String(), logNeedle, "LOG_DETAIL=1")
}

// SITE=merchant.go:621 —— PUT /merchants/:id 的 owner 解析失败。
func TestUpdateMerchantOwnerResolutionSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	failQueryOn2592(t, "users")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodPut, "/merchants/"+f.merchantID,
		`{"admin_uid":"`+f.adminID+`"}`)

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litOwner2592,
		sanitizeSentinel2592, f.ownerID, f.adminID)
	t.Logf("SITE=merchant.go:621 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}

// SITE=merchant_member.go:55 —— 成员列表的 owner 解析失败。
func TestListMembersOwnerResolutionSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	failQueryOn2592(t, "users")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodGet, "/admin/merchants/"+f.merchantID+"/members", "")

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litOwner2592,
		sanitizeSentinel2592, f.ownerID, f.adminID)
	t.Logf("SITE=merchant_member.go:55 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}

// SITE=merchant_member.go:62 —— 成员列表的当前登录账号解析失败。
// 商户无 admin_uid 时 owner 解析提前返回，故障点只落在 viewerLocalUserID。
func TestListMembersViewerResolutionSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	// admin_uid 是 uuid 列：清空必须走 NULL（空串会被 PG 拒绝）。
	require.NoError(t, database.GetDB().
		Exec("UPDATE merchants SET admin_uid = NULL, admin_pending = true WHERE id = ?", f.merchantID).Error)
	failQueryOn2592(t, "users")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodGet, "/admin/merchants/"+f.merchantID+"/members", "")

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litViewer2592,
		sanitizeSentinel2592, f.ownerID, f.adminID)
	t.Logf("SITE=merchant_member.go:62 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}

// SITE=merchant_member.go:497 —— 改角色前的 owner 解析失败。
func TestUpdateMemberRoleOwnerResolutionSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	failQueryOn2592(t, "users")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodPut,
		"/admin/merchants/"+f.merchantID+"/members/"+f.staffID, `{"role":"site_member"}`)

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litOwner2592,
		sanitizeSentinel2592, f.ownerID, f.adminID)
	t.Logf("SITE=merchant_member.go:497 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}

// SITE=merchant_member.go:614 —— 移除成员前的 owner 解析失败。
func TestRemoveMemberOwnerResolutionSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	failQueryOn2592(t, "users")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodDelete,
		"/admin/merchants/"+f.merchantID+"/members/"+f.staffID, "")

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litOwner2592,
		sanitizeSentinel2592, f.ownerID, f.adminID)
	t.Logf("SITE=merchant_member.go:614 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}

// SITE=merchant_member.go:811 —— transferMerchantOwnership 内的 owner 解析失败。
func TestTransferOwnershipOwnerResolutionSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	failQueryOn2592(t, "users")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+f.adminID+`"}`)

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litOwner2592,
		sanitizeSentinel2592, f.ownerID, f.adminID)
	mock.mu.Lock()
	transferCalls := mock.transferCalls
	mock.mu.Unlock()
	assert.Equal(t, 0, transferCalls, "OWNER_RESOLUTION_FAIL_IAM_CALLS=0")
	t.Logf("SITE=merchant_member.go:811 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}

// SITE=merchant_member.go:854 —— 目标成员查询失败（非 record-not-found）。
func TestTransferOwnershipTargetLookupSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	failQueryOn2592(t, "merchant_members")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+f.adminID+`"}`)

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litTarget2592,
		sanitizeSentinel2592, f.ownerID, f.adminID)
	mock.mu.Lock()
	transferCalls := mock.transferCalls
	mock.mu.Unlock()
	assert.Equal(t, 0, transferCalls, "TARGET_LOOKUP_FAIL_IAM_CALLS=0")
	t.Logf("SITE=merchant_member.go:854 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}

// SITE=merchant_member.go:879 —— beaconiam 不可达（非 IAMTransferOwnershipError）。
func TestTransferOwnershipIAMUnreachableSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	services.SetIAMInternalURLForTesting("http://" + addr)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	w := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+f.adminID+`"}`)

	assertSanitized2592(t, w, logBuf, http.StatusBadGateway, 50200, litTransfer2592,
		"IAM transfer failed", f.ownerID, f.adminID)
	assert.NotContains(t, w.Body.String(), addr, "LEAK_IAM_ADDR=0")

	var merchant models.Merchant
	require.NoError(t, database.GetDB().Where("id = ?", f.merchantID).First(&merchant).Error)
	assert.Equal(t, f.ownerID, merchant.AdminUID, "LOCAL_OWNER_UNCHANGED=1")
	t.Logf("SITE=merchant_member.go:879 STATUS=%d CODE=%d LEAK=0 LOG_DETAIL=1", w.Code, 50200)
}

// SITE=merchant_member.go:916 —— IAM 已转让但本地事务回滚：固定文案，且不回显
// 旧/新所有人 UUID 与原始 error。
func TestTransferOwnershipLocalSyncSanitized_2592(t *testing.T) {
	f := seedOwnershipFixture2546(t)
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	failUpdateOn2592(t, "merchant_members", "injected merchant_members update failure")
	logBuf := captureLog2592(t)

	r := ownershipRouter2546(t, f.orgID, f.orgID)
	// 目标是直属员工：提升失败才可观测（role 变化）。
	w := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+f.staffID+`"}`)

	assertSanitized2592(t, w, logBuf, http.StatusInternalServerError, 50000, litSync2592,
		"injected merchant_members update failure", f.ownerID, f.staffID)
	assert.NotContains(t, w.Body.String(), "rows=", "LEAK_ROWS_DIAG=0")

	var merchant models.Merchant
	require.NoError(t, database.GetDB().Where("id = ?", f.merchantID).First(&merchant).Error)
	assert.Equal(t, f.ownerID, merchant.AdminUID, "ADMIN_UID_UNCHANGED=1")
	t.Logf("SITE=merchant_member.go:916 STATUS=%d CODE=%d LEAK=0 UUID_LEAK=0 LOG_DETAIL=1", w.Code, 50000)
}
