package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2062: audit_logs.details(jsonb) 纯文本写入静默丢失 → 统一 writeAuditLog helper 修复。

func audit2062Inject(actor testutil.TestActor) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}
}

func audit2062Count(t *testing.T, action, resourceID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.GetDB().Model(&models.AuditLog{}).
		Where("action = ? AND resource_id = ?", action, resourceID).Count(&n).Error)
	return n
}

// helper：写入成功且 details 为合法 JSON
func TestWriteAuditLog_2062_ValidJSON(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	writeAuditLog(db, uuid.New().String(), uuid.New().String(), "t_2062", "", "r-1",
		map[string]interface{}{"event": "x", "n": 1})

	var row models.AuditLog
	require.NoError(t, db.Where("action = ?", "t_2062").First(&row).Error)
	require.NotNil(t, row.Details)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*row.Details), &m), "details 必须是合法 JSON（jsonb 列要求）")
	assert.Equal(t, "x", m["event"])
}

// 根因约束（重现）：纯文本 details 无法写入 jsonb → 静默丢失
func TestAuditLog_2062_PlainTextRejected(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	plain := "纯文本详情"
	err := db.Create(&models.AuditLog{
		ID: uuid.New().String(), TenantID: uuid.New().String(), UserID: uuid.New().String(),
		Action: "t_plain_2062", ResourceID: "r-2", Details: &plain,
	}).Error
	require.Error(t, err, "jsonb 列拒绝纯文本（SQLSTATE 22P02）—— 故 helper 必须 json.Marshal")
	var n int64
	require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ?", "t_plain_2062").Count(&n).Error)
	assert.EqualValues(t, 0, n, "写入失败即静默丢失（历史缺陷）")
}

// 流级：报废 → scrap_instrument 留痕
func TestScrapInstrument_2062_Audit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID := uuid.New().String(), uuid.New().String()
	userID := uuid.New().String()
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID, SN: "SN-2062", StockStatus: "available",
	}).Error)

	r := gin.New()
	r.Use(audit2062Inject(testutil.MakeCustomer(tenantID, userID)))
	r.POST("/instruments/:id/scrap", ScrapInstrument)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/instruments/"+instID+"/scrap", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, audit2062Count(t, "scrap_instrument", instID), "报废须留痕（修复前为 0）")
}

// 流级：归还 → return_order 留痕（details 含物流信息）
func TestReturnOrder_2062_Audit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID := uuid.New().String(), uuid.New().String()
	userID := uuid.New().String()
	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(), Status: models.OrderStatusInLease, LeaseTerm: 30,
	}).Error)

	r := gin.New()
	r.Use(audit2062Inject(testutil.MakeCustomer(tenantID, userID)))
	r.POST("/orders/:id/return", ReturnOrder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orders/"+orderID+"/return",
		bytes.NewReader([]byte(`{"courier_company":"SF","tracking_number":"SF123"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row models.AuditLog
	require.NoError(t, db.Where("action = ? AND resource_id = ?", "return_order", orderID).First(&row).Error)
	require.NotNil(t, row.Details)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*row.Details), &m))
	assert.Equal(t, "SF", m["courier"])
}

// 流级：SMTP 配置更新 → update_smtp_config 留痕
// actor TenantID="" + 非 USER 角色 → GetBusinessRole=system_admin（requireSystemAdmin 通过）
func TestUpdateSMTPConfig_2062_Audit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	actor := testutil.TestActor{TenantID: "", UserID: uuid.New().String(), Role: "ADMIN"}

	r := gin.New()
	r.Use(audit2062Inject(actor))
	r.PUT("/admin/smtp-config", UpdateSMTPConfig)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/smtp-config",
		bytes.NewReader([]byte(`{"host":"smtp.test","port":587,"from":"a@b.com","enabled":false}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row models.AuditLog
	require.NoError(t, db.Where("action = ?", "update_smtp_config").First(&row).Error)
	require.NotNil(t, row.Details)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*row.Details), &m))
	assert.Equal(t, "update", m["event"])
}
