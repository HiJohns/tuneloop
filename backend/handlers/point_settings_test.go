package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// #1945 Sub-B: 全局「乐币规则」参数（GET/PUT /admin/point-settings）。
// 覆盖：默认值 / 持久化 / 越界拒绝 / 即时应用到运行时 / pay_ratio_max 影响 per-level 校验。
func TestPointSettingsDefaults(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	_ = db
	router := giftPolicyRouter(makeAdminActor())

	req := httptest.NewRequest("GET", "/api/admin/point-settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			PayRatioMax              float64 `json:"pay_ratio_max"`
			PointBatchValidityMonths float64 `json:"point_batch_validity_months"`
			PointExpiryReminderDays  float64 `json:"point_expiry_reminder_days"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, 1.0, resp.Data.PayRatioMax, "默认抵扣上限 100%")
	require.Equal(t, 24.0, resp.Data.PointBatchValidityMonths, "默认有效期 24 月")
	require.Equal(t, 30.0, resp.Data.PointExpiryReminderDays, "默认提醒提前量 30 天")
}

func TestPointSettingsUpdateAndApply(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	_ = db
	router := giftPolicyRouter(makeAdminActor())

	// 保存原运行时值，测试结束恢复（包级全局，避免污染其他用例）。
	origValidity, origLead := services.PointBatchValidity, services.PointBatchReminderLead
	t.Cleanup(func() {
		services.PointBatchValidity = origValidity
		services.PointBatchReminderLead = origLead
	})

	body, _ := json.Marshal(map[string]interface{}{
		"pay_ratio_max":               0.5,
		"point_batch_validity_months": 12,
		"point_expiry_reminder_days":  7,
	})
	req := httptest.NewRequest("PUT", "/api/admin/point-settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 持久化（全局租户 nil UUID）
	var rows []models.SystemSetting
	require.NoError(t, db.Where("setting_key IN ?", []string{
		services.SettingPayRatioMax,
		services.SettingPointBatchValidityMonths,
		services.SettingPointExpiryReminderDays,
	}).Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, r := range rows {
		require.Equal(t, "00000000-0000-0000-0000-000000000000", r.TenantID)
	}

	// 即时应用到运行时（无需重启）
	require.Equal(t, time.Duration(12)*30*24*time.Hour, services.PointBatchValidity)
	require.Equal(t, time.Duration(7)*24*time.Hour, services.PointBatchReminderLead)

	// 回读
	req2 := httptest.NewRequest("GET", "/api/admin/point-settings", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	var got struct {
		Data struct {
			PayRatioMax              float64 `json:"pay_ratio_max"`
			PointBatchValidityMonths float64 `json:"point_batch_validity_months"`
			PointExpiryReminderDays  float64 `json:"point_expiry_reminder_days"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &got))
	require.Equal(t, 0.5, got.Data.PayRatioMax)
	require.Equal(t, 12.0, got.Data.PointBatchValidityMonths)
	require.Equal(t, 7.0, got.Data.PointExpiryReminderDays)
}

func TestPointSettingsBoundsRejected(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	_ = db
	router := giftPolicyRouter(makeAdminActor())

	cases := []map[string]interface{}{
		{"pay_ratio_max": 1.01},
		{"pay_ratio_max": -0.01},
		{"point_batch_validity_months": 0},
		{"point_batch_validity_months": 121},
		{"point_expiry_reminder_days": -1},
		{"point_expiry_reminder_days": 366},
	}
	for _, c := range cases {
		body, _ := json.Marshal(c)
		req := httptest.NewRequest("PUT", "/api/admin/point-settings", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, "body=%v resp=%s", c, w.Body.String())
	}
}

// pay_ratio_max 生效：配置上限 0.5 后，level pay_ratio=0.6 被拒、=0.5 通过。
func TestPointSettingsPayRatioMaxAffectsGuardrail(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	_ = db
	router := giftPolicyRouter(makeAdminActor())

	body, _ := json.Marshal(map[string]interface{}{"pay_ratio_max": 0.5})
	req := httptest.NewRequest("PUT", "/api/admin/point-settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 0.6 → 拒绝
	over, _ := json.Marshal(map[string]interface{}{"level_id": 1, "pay_ratio": 0.6})
	reqOver := httptest.NewRequest("PUT", "/api/admin/gift-policies", bytes.NewBuffer(over))
	reqOver.Header.Set("Content-Type", "application/json")
	wOver := httptest.NewRecorder()
	router.ServeHTTP(wOver, reqOver)
	require.Equal(t, http.StatusBadRequest, wOver.Code, "0.6 > max 0.5 应拒绝: %s", wOver.Body.String())

	// 0.5 → 通过
	ok, _ := json.Marshal(map[string]interface{}{"level_id": 1, "pay_ratio": 0.5})
	reqOK := httptest.NewRequest("PUT", "/api/admin/gift-policies", bytes.NewBuffer(ok))
	reqOK.Header.Set("Content-Type", "application/json")
	wOK := httptest.NewRecorder()
	router.ServeHTTP(wOK, reqOK)
	require.Equal(t, http.StatusOK, wOK.Code, "0.5 = max 应通过: %s", wOK.Body.String())
}

// pay_ratio_max 可配置后应能放宽/收紧，但仍硬上限 1.0（超 1 的值被校验拒绝，无法写入）。
func TestPointSettingsPayRatioMaxHardCap(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	_ = db
	router := giftPolicyRouter(makeAdminActor())

	body, _ := json.Marshal(map[string]interface{}{"pay_ratio_max": 1.2})
	req := httptest.NewRequest("PUT", "/api/admin/point-settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, "pay_ratio_max=1.2 应拒绝: %s", w.Body.String())
}
