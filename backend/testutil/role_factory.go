package testutil

import (
	"context"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
)

type TestActor struct {
	TenantID string
	UserID   string
	OrgID    string
	SiteID   string
	Role     string
	SysPerm  int64
	CusPerm  int64
}

func MakeCustomer(tenantID, userID string) TestActor {
	return TestActor{
		TenantID: tenantID,
		UserID:   userID,
		OrgID:    "",
		Role:     "USER",
	}
}

func MakeSiteMember(tenantID, orgID, userID string) TestActor {
	return TestActor{
		TenantID: tenantID,
		UserID:   userID,
		OrgID:    orgID,
		Role:     "STAFF",
	}
}

func MakeSiteAdmin(tenantID, orgID, userID string) TestActor {
	return TestActor{
		TenantID: tenantID,
		UserID:   userID,
		OrgID:    orgID,
		Role:     "ADMIN",
		SysPerm:  -1,
		CusPerm:  -1,
	}
}

func MakeForwardingSiteMember(tenantID, orgID, userID string) TestActor {
	return TestActor{
		TenantID: tenantID,
		UserID:   userID,
		OrgID:    orgID,
		Role:     "STAFF",
	}
}

func (a TestActor) InjectContext(ctx context.Context) context.Context {
	// #2260 审计（Bug2）：生产中间件同时写 middleware 与 database 两套 key；
	// 测试上下文补齐 database 键，使 GORM 租户回调（addTenantScope）真实执行。
	ctx = database.SetTenantID(ctx, a.TenantID)
	ctx = database.SetOrgID(ctx, a.OrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, a.TenantID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, a.UserID)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, a.OrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, a.Role)
	ctx = context.WithValue(ctx, middleware.ContextKeyGid, a.TenantID)
	if a.SysPerm != 0 {
		ctx = context.WithValue(ctx, middleware.ContextKeySysPerm, a.SysPerm)
	}
	if a.CusPerm != 0 {
		ctx = context.WithValue(ctx, middleware.ContextKeyCusPerm, a.CusPerm)
	}
	return ctx
}
