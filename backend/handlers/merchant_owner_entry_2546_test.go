package handlers

// #2546 R15：actor 授权矩阵（D13 = 方案 a）。
//
// 商户管理员（JWT businessRole=merchant_admin，sys_perm=1073740800 = bit 10-29）：
//   - 可读 4 条路由：GET /merchants、GET /merchants/:id、GET /admin/merchants/:id/members、
//     POST /merchants/:id/transfer-ownership（数据面收窄到本商户）；
//   - 平台专属写路由仍是 403：创建/编辑/删除商户、添加成员。
//
// 路由表按 backend/main.go 的注册顺序与中间件逐条复刻（main 包不可 import）。

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// merchantAdminSysPerm2546 是 IAM merchant_admin 模板位图（bit 10-29），不含 tenant 位。
const merchantAdminSysPerm2546 = int64(1073740800)

func ownerEntryRouter2546(t *testing.T, actor testutil.TestActor) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	members := &MerchantMemberHandler{}
	merchants := &MerchantHandler{}

	// 4 条放开路由（与 main.go 同中间件链）。
	r.GET("/merchants", middleware.ExplicitTenantScope(),
		middleware.RequireSysPermOrRole(middleware.SysPermTenantList, middleware.BusinessRoleMerchantAdmin), merchants.ListMerchants)
	r.GET("/merchants/:id", middleware.ExplicitTenantScope(),
		middleware.RequireSysPermOrRole(middleware.SysPermTenantView, middleware.BusinessRoleMerchantAdmin), merchants.GetMerchant)
	r.GET("/admin/merchants/:id/members", middleware.ExplicitTenantScope(),
		middleware.RequireSysPermOrRole(middleware.SysPermTenantView, middleware.BusinessRoleMerchantAdmin), members.ListMembers)
	r.POST("/merchants/:id/transfer-ownership", middleware.ExplicitTenantScope(),
		middleware.RequireSysPermOrRole(middleware.SysPermTenantUpdate, middleware.BusinessRoleMerchantAdmin), members.TransferOwnership)

	// 平台专属写路由（保持 RequireSysPerm，不得对商户管理员放行）。
	r.POST("/merchants", middleware.ExplicitTenantScope(),
		middleware.RequireSysPerm(middleware.SysPermTenantCreate), merchants.CreateMerchant)
	r.PUT("/merchants/:id", middleware.ExplicitTenantScope(),
		middleware.RequireSysPerm(middleware.SysPermTenantUpdate), merchants.UpdateMerchant)
	r.DELETE("/merchants/:id", middleware.ExplicitTenantScope(),
		middleware.RequireSysPerm(middleware.SysPermTenantDelete), merchants.DeleteMerchant)
	r.POST("/admin/merchants/:id/members", middleware.ExplicitTenantScope(),
		middleware.RequireSysPerm(middleware.SysPermTenantUpdate), members.AddMember)
	return r
}

func TestMerchantAdminActorAuthorization_2546(t *testing.T) {
	f := seedOwnershipFixture2546(t)

	// 第二个商户 M2（同一平台，不同 tenant/org）：用于验证跨商户不可见。
	otherOrg := uuid.NewString()
	testfixtures.SeedTenantFK(t, database.GetDB(), otherOrg)
	otherMerchantID := uuid.NewString()
	require.NoError(t, database.GetDB().Create(&models.Merchant{
		ID: otherMerchantID, TenantID: otherOrg, OrgID: otherOrg, Name: "M2-2546",
		AdminUID: nilUUID2546, Status: "active", MerchantType: "full",
	}).Error)

	actor := testutil.TestActor{
		TenantID: f.orgID,
		OrgID:    f.orgID,
		UserID:   f.ownerSub, // JWT sub（IAM 用户 id）；本地 users.id 由服务端解析
		Role:     "OWNER",
		SysPerm:  merchantAdminSysPerm2546,
	}
	r := ownerEntryRouter2546(t, actor)

	// 1) 列表：只看到本商户（数据面收窄，不是所有商户）。
	wList := doOwnershipRequest2546(t, r, http.MethodGet, "/merchants", "")
	require.Equal(t, http.StatusOK, wList.Code, wList.Body.String())
	var listResp struct {
		Data struct {
			Total int64 `json:"total"`
			List  []struct {
				ID string `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &listResp), wList.Body.String())
	assert.Equal(t, int64(1), listResp.Data.Total, "MA_LIST_ROWS=1")
	require.Len(t, listResp.Data.List, 1)
	assert.Equal(t, f.merchantID, listResp.Data.List[0].ID, "MA_LIST_SCOPED=1")

	// 2) 详情：本商户 200，另一商户 404（不泄漏）。
	wOwn := doOwnershipRequest2546(t, r, http.MethodGet, "/merchants/"+f.merchantID, "")
	assert.Equal(t, http.StatusOK, wOwn.Code, "MA_GET_OWN_STATUS=200")
	wOther := doOwnershipRequest2546(t, r, http.MethodGet, "/merchants/"+otherMerchantID, "")
	assert.Equal(t, http.StatusNotFound, wOther.Code, "MA_GET_OTHER_STATUS=404")

	// 3) 成员列表可读。
	wMembers := doOwnershipRequest2546(t, r, http.MethodGet, "/admin/merchants/"+f.merchantID+"/members", "")
	assert.Equal(t, http.StatusOK, wMembers.Code, "MA_MEMBERS_STATUS=200")

	// 4) 转让可执行（商户管理员 = 当前所有人）。
	mock := &ownershipIAMMock2546{}
	setupOwnershipIAMMock2546(t, mock)
	wTransfer := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants/"+f.merchantID+"/transfer-ownership",
		`{"user_id":"`+f.adminID+`"}`)
	assert.Equal(t, http.StatusOK, wTransfer.Code, "MA_TRANSFER_STATUS=200")

	// 5) 平台专属写路由仍 403（越权面不得扩大）。
	wCreate := doOwnershipRequest2546(t, r, http.MethodPost, "/merchants", `{"name":"x"}`)
	assert.Equal(t, http.StatusForbidden, wCreate.Code, "MA_CREATE_MERCHANT_STATUS=403")
	wUpdate := doOwnershipRequest2546(t, r, http.MethodPut, "/merchants/"+f.merchantID, `{"name":"x"}`)
	assert.Equal(t, http.StatusForbidden, wUpdate.Code, "MA_UPDATE_MERCHANT_STATUS=403")
	wDelete := doOwnershipRequest2546(t, r, http.MethodDelete, "/merchants/"+f.merchantID, "")
	assert.Equal(t, http.StatusForbidden, wDelete.Code, "MA_DELETE_MERCHANT_STATUS=403")
	wAdd := doOwnershipRequest2546(t, r, http.MethodPost, "/admin/merchants/"+f.merchantID+"/members", `{"user_id":"`+f.staffID+`"}`)
	assert.Equal(t, http.StatusForbidden, wAdd.Code, "MA_ADD_MEMBER_STATUS=403")

	t.Logf("MA_LIST_STATUS=%d MA_LIST_ROWS=%d MA_GET_OWN_STATUS=%d MA_GET_OTHER_STATUS=%d MA_MEMBERS_STATUS=%d "+
		"MA_TRANSFER_STATUS=%d MA_CREATE_MERCHANT_STATUS=%d MA_UPDATE_MERCHANT_STATUS=%d MA_DELETE_MERCHANT_STATUS=%d MA_ADD_MEMBER_STATUS=%d",
		wList.Code, listResp.Data.Total, wOwn.Code, wOther.Code, wMembers.Code,
		wTransfer.Code, wCreate.Code, wUpdate.Code, wDelete.Code, wAdd.Code)
}
