package handlers

// #2403：InviteToMerchant 商户查找不得用 JWT tenant 过滤（merchants.tenant_id = 命名空间 org，
// JWT tid = 商户 org）；改为按 id 查 + 可见组织授权。
//   R1 商户管理员邀请成功（修复前 404）
//   R2 跨商户 → 404（不泄露存在性）
//   R3 去重（409）与 kind/role 映射
import (
	"net/http"
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

type invite2403Fixture struct {
	r           *gin.Engine
	merchantOrg string // JWT tid/oid = 商户 org
	namespace   string // merchants.tenant_id = 命名空间 org（≠ JWT tid）
	merchantID  string
	adminSub    string
}

func setupInvite2403(t *testing.T) *invite2403Fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	f := &invite2403Fixture{
		merchantOrg: uuid.NewString(),
		namespace:   uuid.NewString(),
		merchantID:  uuid.NewString(),
		adminSub:    uuid.NewString(),
	}

	// site_members/staff_invites 的 tenant 列在测试库带 FK → 保留 tenants 行
	require.NoError(t, db.Create(&models.Tenant{
		ID: f.merchantOrg, Name: "T2403-" + f.merchantOrg[:8], Status: "active",
	}).Error)

	// 现场数据形态：merchants.org_id=商户 org，merchants.tenant_id=命名空间 org
	require.NoError(t, db.Create(&models.Merchant{
		ID: f.merchantID, TenantID: f.namespace, OrgID: f.merchantOrg,
		Name: "M2403", Code: "M2403-" + f.merchantOrg[:8],
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active",
		AdminUID: uuid.NewString(),
	}).Error)

	// 操作者：商户管理员（tid=oid=商户 org）
	require.NoError(t, db.Create(&models.User{
		ID: uuid.NewString(), IAMSub: f.adminSub, TenantID: f.merchantOrg, OrgID: f.merchantOrg,
		Username: "admin2403", Name: "商户管理员", Role: "OWNER", Status: "active",
	}).Error)

	f.r = gin.New()
	f.r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: f.merchantOrg, OrgID: f.merchantOrg, UserID: f.adminSub, Role: "OWNER"}.
				InjectContext(c.Request.Context()))
		c.Next()
	})
	f.r.POST("/api/admin/merchants/:id/invites", InviteToMerchant)
	return f
}

// R1：修复前用 tenant 过滤恒 404；修复后按 id 查 + 可见组织 → 201，且 invite 落库。
func TestInviteToMerchantMerchantScopeFixed_2403(t *testing.T) {
	f := setupInvite2403(t)
	db := database.GetDB()

	code, resp := doJSON(t, f.r, http.MethodPost, "/api/admin/merchants/"+f.merchantID+"/invites",
		`{"identifier":"invitee2403@example.com","kind":"merchant_staff"}`)
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)

	var inv models.StaffInvite
	require.NoError(t, db.Where("org_id = ? AND invitee_identifier = ?", f.merchantOrg, "invitee2403@example.com").
		First(&inv).Error)
	assert.Equal(t, f.merchantOrg, inv.TenantID, "invite tenant 仍为商户 org（不扩大到命名空间）")
	assert.Equal(t, f.merchantOrg, inv.OrgID)
	assert.Equal(t, "merchant_staff", inv.Kind)
	assert.Equal(t, "merchant_staff", inv.Role)
	assert.Equal(t, "pending", inv.Status)
}

// R2：商户 B 的 id + 商户 A 管理员 → 404，且不落库（不泄露存在性）。
func TestInviteToMerchantCrossMerchant404_2403(t *testing.T) {
	f := setupInvite2403(t)
	db := database.GetDB()

	otherOrg := uuid.NewString()
	otherID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: otherID, TenantID: uuid.NewString(), OrgID: otherOrg,
		Name: "M2403B", Code: "M2403B-" + otherOrg[:8],
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active",
		AdminUID: uuid.NewString(),
	}).Error)

	code, resp := doJSON(t, f.r, http.MethodPost, "/api/admin/merchants/"+otherID+"/invites",
		`{"identifier":"other2403@example.com"}`)
	require.Equal(t, http.StatusNotFound, code, "resp=%v", resp)
	assert.EqualValues(t, 40400, resp["code"])

	var n int64
	require.NoError(t, db.Model(&models.StaffInvite{}).Where("org_id = ?", otherOrg).Count(&n).Error)
	assert.EqualValues(t, 0, n, "跨商户请求不得落库")
}

// R3：同 identifier 重复邀请 → 409；kind 缺省映射 merchant_member/member。
func TestInviteToMerchantDedupAndKind_2403(t *testing.T) {
	f := setupInvite2403(t)
	db := database.GetDB()

	path := "/api/admin/merchants/" + f.merchantID + "/invites"
	code1, resp1 := doJSON(t, f.r, http.MethodPost, path, `{"identifier":"dup2403@example.com","kind":"merchant_staff"}`)
	require.Equal(t, http.StatusCreated, code1, "resp=%v", resp1)

	code2, resp2 := doJSON(t, f.r, http.MethodPost, path, `{"identifier":"dup2403@example.com","kind":"merchant_staff"}`)
	require.Equal(t, http.StatusConflict, code2, "resp=%v", resp2)
	assert.EqualValues(t, 40900, resp2["code"])

	var n int64
	require.NoError(t, db.Model(&models.StaffInvite{}).
		Where("org_id = ? AND invitee_identifier = ?", f.merchantOrg, "dup2403@example.com").Count(&n).Error)
	assert.EqualValues(t, 1, n, "重复邀请不得产生第二行")

	// kind 缺省 → merchant_member / role=member
	code3, resp3 := doJSON(t, f.r, http.MethodPost, path, `{"identifier":"default2403@example.com"}`)
	require.Equal(t, http.StatusCreated, code3, "resp=%v", resp3)
	data, _ := resp3["data"].(map[string]interface{})
	assert.Equal(t, "merchant_member", data["kind"])
	assert.Equal(t, "member", data["role"])
}
