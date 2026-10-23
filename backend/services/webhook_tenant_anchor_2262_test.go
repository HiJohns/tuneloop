package services

// #2262 B5：warning 的商户引用必须解析为商户 org（配置键），而非共享 tenant。
// 本测试只建 2 张表（服务层单测，避免 handlers/testfixtures 的 import cycle）。

import (
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerchantTenantID_ReturnsMerchantOrg_2262(t *testing.T) {
	db, err := database.InitDB(database.LoadConfig())
	if err != nil {
		t.Skip("test database not available")
	}
	database.SetDB(db)
	require.NoError(t, db.AutoMigrate(&models.Merchant{}, &models.SystemSetting{}))

	shared := uuid.NewString()
	org := uuid.NewString()
	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: shared, OrgID: org, Name: "WH-" + org[:8],
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	assert.Equal(t, org, merchantTenantID(db, merchantID), "必须返回商户 org（配置键）")
	assert.Equal(t, org, merchantTenantID(db, org), "按 org 引用同样返回 org")
	assert.Equal(t, "", merchantTenantID(db, uuid.NewString()), "未知商户返回空")
}
