package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// #1982: 管理员加赠乐币 —— 建 manual 批次 + points_transactions 留痕。
func doGrant(t *testing.T, r *gin.Engine, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/admin/user-management/"+userID+"/points-grant", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGrantPoints_CreatesBatchAndLedger(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := "00000000-0000-4000-8000-0000000000a1"
	userID := "00000000-0000-4000-8000-0000000000a2"
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
		Username: "grantee", Status: "active",
	}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/admin/user-management/:id/points-grant", NewUserManagementHandler().GrantPoints)

	body, _ := json.Marshal(map[string]interface{}{"amount": 12.5, "reason": "客服补偿"})
	w := doGrant(t, r, userID, string(body))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			BatchID     string      `json:"batch_id"`
			AmountCents int64       `json:"amount_cents"`
			Balance     int64       `json:"balance"`
			ExpiresAt   interface{} `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, int64(1250), resp.Data.AmountCents, "12.5 元 = 1250 分")
	require.Equal(t, int64(1250), resp.Data.Balance)
	require.NotEmpty(t, resp.Data.BatchID)

	// 批次台账：source=manual，受有效期约束（归一化次月首日）。
	var b models.PointBatch
	require.NoError(t, db.Where("id = ?", resp.Data.BatchID).First(&b).Error)
	require.Equal(t, services.PointBatchSourceManual, b.SourceType)
	require.Equal(t, models.Cents(1250), b.AmountCents)
	require.Equal(t, models.Cents(1250), b.RemainingCents)
	require.NotNil(t, b.ExpiresAt)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	require.Equal(t, 1, b.ExpiresAt.In(loc).Day(), "归一化到次月首日（北京时间）")
	require.Equal(t, "客服补偿", b.SourceRef)

	// 台账留痕：type=manual_grant，描述含原因。
	var pt models.PointsTransaction
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, "manual_grant").First(&pt).Error)
	require.Equal(t, models.Cents(1250), pt.Amount)
	require.True(t, strings.Contains(pt.Description, "客服补偿"), "描述含加赠原因: %s", pt.Description)
	require.True(t, strings.Contains(pt.Description, "操作人"), "描述含操作人字段: %s", pt.Description)

	// 余额真源 = 批次 SUM。
	require.Equal(t, models.Cents(1250), pointsBalance(t, db, userID))
}

func TestGrantPoints_Validation(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := "00000000-0000-4000-8000-0000000000b1"
	userID := "00000000-0000-4000-8000-0000000000b2"
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
		Username: "grantee2", Status: "active",
	}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/admin/user-management/:id/points-grant", NewUserManagementHandler().GrantPoints)

	cases := []struct {
		name string
		body string
		user string
		want int
	}{
		{"amount zero", `{"amount":0,"reason":"x"}`, userID, http.StatusBadRequest},
		{"amount negative", `{"amount":-1,"reason":"x"}`, userID, http.StatusBadRequest},
		{"reason empty", `{"amount":1,"reason":"   "}`, userID, http.StatusBadRequest},
		{"user not found", `{"amount":1,"reason":"x"}`, "00000000-0000-4000-8000-00000000ffff", http.StatusNotFound},
	}
	for _, c := range cases {
		w := doGrant(t, r, c.user, c.body)
		require.Equal(t, c.want, w.Code, "%s: %s", c.name, w.Body.String())
	}

	// 越界与非法均不落批次。
	var count int64
	db.Model(&models.PointBatch{}).Where("user_id = ?", userID).Count(&count)
	require.Equal(t, int64(0), count, "无有效加赠不产生批次")
}

// #1982 audit fix: reason with multibyte text > 64 bytes must not crash
// (byte-slicing used to produce invalid UTF-8 → SQLSTATE 22021 → 500).
func TestGrantPoints_LongChineseReasonRuneSafe(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID := "00000000-0000-4000-8000-0000000000c1"
	userID := "00000000-0000-4000-8000-0000000000c2"
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
		Username: "grantee3", Status: "active",
	}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/admin/user-management/:id/points-grant", NewUserManagementHandler().GrantPoints)

	longReason := strings.Repeat("长", 70) // 70 runes = 210 bytes > 64 bytes
	body, _ := json.Marshal(map[string]interface{}{"amount": 1, "reason": longReason})
	w := doGrant(t, r, userID, string(body))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var b models.PointBatch
	require.NoError(t, db.Where("user_id = ?", userID).First(&b).Error)
	require.True(t, utf8.ValidString(b.SourceRef), "source_ref 必须是合法 UTF-8")
	require.Equal(t, 64, utf8.RuneCountInString(b.SourceRef), "按 rune 截断到 64 字符")

	// 完整原因保留在流水描述（varchar 500）中。
	var pt models.PointsTransaction
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, "manual_grant").First(&pt).Error)
	require.True(t, strings.Contains(pt.Description, longReason), "描述保留完整原因")
}
