package testutil

import (
	"context"
	"tuneloop-backend/middleware"
)

type TestActor struct {
	TenantID string
	UserID   string
	OrgID    string
	Role     string
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

func (a TestActor) InjectContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, a.TenantID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, a.UserID)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, a.OrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, a.Role)
	return ctx
}
