package handlers

// #2242：实名审核历史留存 + 姓名搜索（审批快照写入 audit_logs；历史端点）。
// 红绿口径：父提交无 /history 端点（404）且 details 无实名快照 → 本测试失败。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const frhFullIDCard = "110101199001011234"

type frhEnv struct {
	router *gin.Engine
	batchA string
	batchR string
	userA  string
	userR  string
}

func newFrhEnv(t *testing.T) (frhEnv, func() *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	operator := uuid.NewString()
	actor := testutil.TestActor{TenantID: "", OrgID: "", UserID: operator, Role: "NAMESPACE_ADMIN"}

	// A：待审核（注册类，用户未采录实名 → approve 时由员工填写 5 项）
	userA := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userA, IAMSub: uuid.NewString(), Name: "历史甲", Phone: "13900004401", Status: "active",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
	}).Error)
	batchA := uuid.NewString()
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: batchA, UserID: userA, Status: "pending", Kind: "registration", SubmittedAt: time.Now(),
	}).Error)

	// R：待审核（驳回场景）
	userR := uuid.NewString()
	realNameR := "历史乙"
	idNoR := "110101198802021234"
	require.NoError(t, db.Create(&models.User{
		ID: userR, IAMSub: uuid.NewString(), Name: "历史乙", Phone: "13900004402", Status: "active",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		RealName: &realNameR, IdCardNo: &idNoR,
	}).Error)
	batchR := uuid.NewString()
	require.NoError(t, db.Create(&models.FaceCaptureBatch{
		ID: batchR, UserID: userR, Status: "pending", Kind: "registration", SubmittedAt: time.Now(),
	}).Error)

	h := &FaceReviewHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/admin/face-review/:batchId", h.Review)
	r.GET("/admin/face-review/history", h.History)

	return frhEnv{router: r, batchA: batchA, batchR: batchR, userA: userA, userR: userR}, nil
}

func frhCall(t *testing.T, r *gin.Engine, method, path string, body interface{}) (int, string) {
	t.Helper()
	var raw *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		raw = bytes.NewReader(b)
	} else {
		raw = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, raw)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestFaceReviewHistory_ApproveSnapshotAndSearch_2242(t *testing.T) {
	env, _ := newFrhEnv(t)

	// approve：用户未采录 → 员工填写 5 项
	code, body := frhCall(t, env.router, http.MethodPost, "/admin/face-review/"+env.batchA, map[string]interface{}{
		"action": "approve", "real_name": "张三",
		"id_card_no": frhFullIDCard, "id_card_expire": "2030-01-01",
		"id_card_authority": "北京市公安局", "id_card_address": "北京市朝阳区",
	})
	require.Equal(t, http.StatusOK, code, body)

	// 快照落 audit_logs（值级断言 + 掩码负例）
	conn := getTestDB(t)
	var log models.AuditLog
	require.NoError(t, conn.Where("action = ? AND resource_id = ?", "face_review_approve", env.userA).First(&log).Error)
	require.NotNil(t, log.Details)
	assert.Contains(t, *log.Details, "张三")
	assert.Contains(t, *log.Details, "****1234")
	assert.NotContains(t, *log.Details, frhFullIDCard, "快照不得含证件号明文")
	assert.Contains(t, *log.Details, "北京市公安局")

	// 历史列表
	code, body = frhCall(t, env.router, http.MethodGet, "/admin/face-review/history", nil)
	require.Equal(t, http.StatusOK, code, body)
	var resp struct {
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Equal(t, 1, resp.Data.Total)
	item := resp.Data.List[0]
	assert.Equal(t, env.batchA, item["batch_id"])
	assert.Equal(t, "approve", item["result"])
	assert.Equal(t, "张三", item["real_name"])
	assert.Equal(t, "****1234", item["id_card_no_masked"])
	assert.NotContains(t, body, frhFullIDCard, "历史响应不得返回证件号明文")

	// 姓名搜索：命中 / 未命中
	code, body = frhCall(t, env.router, http.MethodGet, "/admin/face-review/history?real_name=%E5%BC%A0", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Equal(t, 1, resp.Data.Total, "按姓名『张』应命中")

	code, body = frhCall(t, env.router, http.MethodGet, "/admin/face-review/history?real_name=%E4%B8%8D%E5%AD%98%E5%9C%A8", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Equal(t, 0, resp.Data.Total, "未命中姓名应为 0")
}

func TestFaceReviewHistory_RejectAndActionFilter_2242(t *testing.T) {
	env, _ := newFrhEnv(t)

	code, body := frhCall(t, env.router, http.MethodPost, "/admin/face-review/"+env.batchR, map[string]interface{}{
		"action": "reject", "reason": "证件模糊",
	})
	require.Equal(t, http.StatusOK, code, body)

	conn := getTestDB(t)
	var log models.AuditLog
	require.NoError(t, conn.Where("action = ? AND resource_id = ?", "face_review_reject", env.userR).First(&log).Error)
	require.NotNil(t, log.Details)
	assert.Contains(t, *log.Details, "证件模糊")
	assert.Contains(t, *log.Details, "****1234")
	assert.NotContains(t, *log.Details, "110101198802021234", "驳回快照同样不得含明文")

	var resp struct {
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	code, body = frhCall(t, env.router, http.MethodGet, "/admin/face-review/history?action=reject", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Equal(t, 1, resp.Data.Total)
	assert.Equal(t, "reject", resp.Data.List[0]["result"])
	assert.Equal(t, "证件模糊", resp.Data.List[0]["reject_reason"])
	assert.Equal(t, "历史乙", resp.Data.List[0]["real_name"])
}

func getTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return database.GetDB()
}
