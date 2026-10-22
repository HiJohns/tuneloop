package database

// #2261（P3 B3 批次1）：显式租户范围开关 + 观察期守卫（no-op 语义验证）。

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ts2261Item struct {
	ID       string
	TenantID string `gorm:"column:tenant_id"`
}

func (ts2261Item) TableName() string { return "ts2261_items" }

func TestExplicitTenantScopeOnly_2261(t *testing.T) {
	db, err := InitDB(LoadConfig())
	if err != nil {
		t.Skip("test database not available")
	}
	SetDB(db)

	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS ts2261_items`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE ts2261_items (id uuid primary key, tenant_id uuid, name text)`).Error)
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS ts2261_items`).Error })
	require.NoError(t, db.Exec(`INSERT INTO ts2261_items (id, tenant_id, name) VALUES (?, ?, 'a'), (?, ?, 'b')`,
		uuid.NewString(), tenantA, uuid.NewString(), tenantB).Error)

	// 1) 默认（未打显式标记）：自动注入 tenant 过滤（现行为不变）
	ctxA := SetTenantID(context.Background(), tenantA)
	var n int64
	require.NoError(t, db.WithContext(ctxA).Model(&ts2261Item{}).Count(&n).Error)
	assert.Equal(t, int64(1), n, "默认路径必须仍自动注入 tenant_id 过滤（守卫式 no-op）")

	// 2) 显式模式：跳过自动注入（逐域迁移后的目标形态）
	ctxExplicit := ExplicitTenantScopeOnly(ctxA)
	require.NoError(t, db.WithContext(ctxExplicit).Model(&ts2261Item{}).Count(&n).Error)
	assert.Equal(t, int64(2), n, "显式模式不得自动注入（调用方自行过滤）")

	// 3) 无 tenant 键：不过滤（既有语义）
	require.NoError(t, db.WithContext(context.Background()).Model(&ts2261Item{}).Count(&n).Error)
	assert.Equal(t, int64(2), n)
}
