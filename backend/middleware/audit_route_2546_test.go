package middleware

// #2546 R9：转让所有权必须落审计 —— auditRouteMap 表项驱动中间件自动写 audit_logs，
// handler 不手写审计（D6）。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditRouteTransferOwnership_2546(t *testing.T) {
	info, ok := auditRouteMap["POST /api/merchants/:id/transfer-ownership"]
	require.True(t, ok, "AUDIT_ROUTE_FOUND=1：转让端点缺少审计表项 → 转让无审计")
	assert.Equal(t, "TRANSFER_OWNERSHIP", info.Action, "AUDIT_ACTION")
	assert.Equal(t, "merchant", info.ResourceType, "AUDIT_RESOURCE")
	assert.Equal(t, "CRITICAL", info.Priority, "AUDIT_PRIORITY")

	t.Logf("AUDIT_ROUTE_FOUND=1 AUDIT_ACTION=%s AUDIT_RESOURCE=%s AUDIT_PRIORITY=%s",
		info.Action, info.ResourceType, info.Priority)
}
