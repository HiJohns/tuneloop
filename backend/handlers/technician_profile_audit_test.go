package handlers

import (
	"net/http"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2059：维修师傅档案 mutation 端点必须写审计留痕（resource_type=technician_profile），
// 保证建档/改档可追溯操作者（来源：预生产 2026-09-20 建档无 audit 痕迹）。
func TestTechnicianProfile_AuditTrail2059(t *testing.T) {
	f := setupTPFixture(t)
	admin := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.adminSub, Role: "ADMIN"}
	db := database.GetDB()

	auditCount := func(action, resourceID string) int64 {
		var n int64
		require.NoError(t, db.Model(&models.AuditLog{}).
			Where("action = ? AND resource_type = ? AND resource_id = ?", action, "technician_profile", resourceID).
			Count(&n).Error)
		return n
	}

	// Create → create_technician_profile
	code, resp := tpDo(t, f, http.MethodPost, "/technician-profiles", gin.H{
		"user_id": f.techID, "bio": "钢琴维修 12 年",
	}, admin)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	profileID := resp["data"].(map[string]interface{})["id"].(string)
	assert.GreaterOrEqual(t, auditCount("create_technician_profile", profileID), int64(1), "创建须留痕")
	// 留痕含操作者（JWT sub）且 Details 非空
	var created models.AuditLog
	require.NoError(t, db.Where("action = ? AND resource_id = ?", "create_technician_profile", profileID).
		First(&created).Error)
	assert.Equal(t, f.adminSub, created.UserID, "审计记录操作者")
	assert.NotNil(t, created.Details)
	assert.NotEmpty(t, *created.Details)

	// Update → update_technician_profile（Details 记变更字段）
	code, _ = tpDo(t, f, http.MethodPut, "/technician-profiles/"+profileID, gin.H{"bio": "更新后"}, admin)
	require.Equal(t, http.StatusOK, code)
	assert.GreaterOrEqual(t, auditCount("update_technician_profile", profileID), int64(1), "更新须留痕")
	var upd models.AuditLog
	require.NoError(t, db.Where("action = ? AND resource_id = ?", "update_technician_profile", profileID).
		First(&upd).Error)
	require.NotNil(t, upd.Details)
	assert.Contains(t, *upd.Details, "bio", "Details 记变更字段")

	// SetStatus → set_technician_profile_status
	code, _ = tpDo(t, f, http.MethodPut, "/technician-profiles/"+profileID+"/status", gin.H{"status": "inactive"}, admin)
	require.Equal(t, http.StatusOK, code)
	assert.GreaterOrEqual(t, auditCount("set_technician_profile_status", profileID), int64(1), "状态变更须留痕")

	// 只读端点不产生审计（List）
	code, _ = tpDo(t, f, http.MethodGet, "/technician-profiles", nil, admin)
	require.Equal(t, http.StatusOK, code)
	var total int64
	require.NoError(t, db.Model(&models.AuditLog{}).Where("resource_type = ?", "technician_profile").Count(&total).Error)
	assert.Equal(t, int64(3), total, "仅 3 次 mutation 留痕，List 不记")
}
