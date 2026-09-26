package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// GetMyRoles returns the current user's site_members roles and site bindings.
// (#1884) `sites` powers site-scoped UI gating (e.g. acceptance panel only for
// the instrument's site staff). `roles` is preserved for backward compatibility.
func GetMyRoles(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	// #2082: JWT 职能角色（fn_roles）并入角色并集——直属商户维修师傅（#1974 T1）
	// 没有 site_members 行，仅靠网点角色会让 roles 为空，前端师傅入口
	//（维修师工作台 / 租赁 Tab 互斥 #1884 / 维修流程门 #1883）被隐藏。
	// customer 为零权限顾客角色（B1），不属于员工角色集，排除。
	roles := []string{}
	for _, r := range middleware.GetFunctionalRoles(ctx) {
		if r == "customer" {
			continue
		}
		roles = append(roles, r)
	}

	// #2082（#2078/#2079 同族）：身份键查询免租户作用域——零租户自注册行
	// 在员工上下文会被 addTenantScope 过滤，导致 roles 意外为空。
	var localUser models.User
	if err := db.WithContext(database.IdentityCtx(ctx)).Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"roles": uniqueStrings(roles), "sites": []interface{}{}}})
		return
	}

	// #2034: roles 为多重角色全集的并集（roles 列优先，回退主 role）
	var members []models.SiteMember
	db.Where("user_id = ?", localUser.ID).Find(&members)
	for _, m := range members {
		roles = append(roles, m.EffectiveRoles()...)
	}

	type siteRole struct {
		SiteID   string   `json:"site_id"`
		Role     string   `json:"role"`
		Roles    []string `json:"roles"`     // #2034 多重角色
		SiteType string   `json:"site_type"` // #1937: 中转工作台入口按站点类型判断（transit）
	}
	var sites []siteRole
	db.Table("site_members sm").
		Select("sm.site_id, sm.role, sm.roles, s.type AS site_type").
		Joins("LEFT JOIN sites s ON s.id = sm.site_id").
		Where("sm.user_id = ?", localUser.ID).Find(&sites)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"roles": uniqueStrings(roles), "sites": sites}})
}

func uniqueStrings(s []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
