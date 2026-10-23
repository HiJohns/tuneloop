package testutil

// #2264：自包含测试夹具助手——用例不再依赖前序数据残留。

import (
	"testing"

	"tuneloop-backend/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// SeedVerifiedCustomer 建一条已核身（双面证件照 + face_verified）的顾客，
// ID/IAMSub 与中间件 userID 一致，供下单/发货等身份门控用例使用。
func SeedVerifiedCustomer(t *testing.T, db *gorm.DB, tenantID, orgID, userID string) models.User {
	t.Helper()
	front := "/uploads/media/id-front-test.jpg"
	back := "/uploads/media/id-back-test.jpg"
	u := models.User{
		ID:           userID,
		IAMSub:       userID,
		TenantID:     tenantID,
		OrgID:        orgID,
		Name:         "Verified Customer",
		Phone:        "13800000000",
		Email:        "verified@example.test",
		IsShadow:     false,
		Status:       "active",
		Role:         "USER",
		FaceVerified: true,
		IdPhotoFront: &front,
		IdPhotoBack:  &back,
	}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// SeedSiteMember 建/复用 users 行并写入 site_members（站点 tenant 由 sites 行推导）。
func SeedSiteMember(t *testing.T, db *gorm.DB, siteID, userID, role string) models.SiteMember {
	t.Helper()
	var site models.Site
	require.NoError(t, db.Where("id = ?", siteID).First(&site).Error, "seed site %s", siteID)

	orgID := site.OrgID
	if orgID == "" {
		orgID = site.TenantID
	}
	var u models.User
	if err := db.Where("id = ?", userID).First(&u).Error; err != nil {
		u = models.User{
			ID:       userID,
			IAMSub:   userID,
			TenantID: site.TenantID,
			OrgID:    orgID,
			Name:     "Seed Site Member",
			IsShadow: false,
			Status:   "active",
			Role:     "STAFF",
		}
		require.NoError(t, db.Create(&u).Error)
	}
	m := models.SiteMember{
		TenantID: site.TenantID,
		SiteID:   siteID,
		UserID:   userID,
		Role:     role,
		Roles:    []string{role},
		Status:   "active",
	}
	require.NoError(t, db.Create(&m).Error)
	return m
}
