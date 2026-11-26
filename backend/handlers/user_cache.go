package handlers

// #2567 D2: single source of truth for the local `users` cache mirror.
//
// `users.role` / `users.tenant_id` / `users.org_id` are a CACHE of the
// authoritative membership tables (merchant_members / site_members). Before
// this file every path wrote those three columns inline (or forgot to), which
// is how a merged account ended up invisible in 人员管理 while the obsolete
// source account stayed visible as "禁用".
//
// Rule: any path that changes a user's memberships (account merge, unbind,
// member removal) calls reconcileUserCache inside its transaction instead of
// writing the three columns by hand.

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

// visibleUserStatuses is the allow-list used by every staff/personnel list
// (#2567 D3). Retired accounts (`obsolete`, `deleted`, `disabled`) must never
// appear — that is how the merged-away staff account surfaced as "禁用" next to
// an invisible surviving account. `pending` stays visible on purpose: the PC
// staff list renders it as 「待确认」 and the activation flow depends on it.
var visibleUserStatuses = []string{"active", "pending"}

// userCacheState is the recomputed cache triple for one user.
type userCacheState struct {
	Role     string
	TenantID string
	OrgID    string
}

// resolveUserCache derives the cache triple from membership authority:
//
//  1. merchant_members active row → tenant_id/org_id = that row's tenant_id
//     (a merchant org is its own tenant), role = that row's role;
//  2. otherwise site_members active row → tenant_id/org_id from the joined
//     sites row, role = site_members.roles[0] (fallback primary role, then
//     "site_member");
//  3. otherwise (pure customer) → USER / zero UUID / zero UUID.
func resolveUserCache(tx *gorm.DB, userID string) (userCacheState, error) {
	var mm models.MerchantMember
	err := tx.Where("user_id = ? AND status = ?", userID, "active").
		Order("created_at ASC").First(&mm).Error
	switch {
	case err == nil:
		role := mm.Role
		if role == "" {
			role = "site_member"
		}
		return userCacheState{Role: role, TenantID: mm.TenantID, OrgID: mm.TenantID}, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return userCacheState{}, fmt.Errorf("merchant_members lookup for %s: %w", userID, err)
	}

	var sm models.SiteMember
	err = tx.Where("user_id = ? AND status = ?", userID, "active").
		Order("created_at ASC").First(&sm).Error
	switch {
	case err == nil:
		var site models.Site
		if err := tx.Where("id = ?", sm.SiteID).First(&site).Error; err != nil {
			return userCacheState{}, fmt.Errorf("site lookup %s for user %s: %w", sm.SiteID, userID, err)
		}
		role := sm.Role
		if len(sm.Roles) > 0 && sm.Roles[0] != "" {
			role = sm.Roles[0]
		}
		if role == "" {
			role = "site_member"
		}
		tenant := site.TenantID
		if tenant == "" {
			tenant = sm.TenantID
		}
		org := site.OrgID
		if org == "" {
			org = zeroUUID
		}
		if tenant == "" {
			tenant = zeroUUID
		}
		return userCacheState{Role: role, TenantID: tenant, OrgID: org}, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return userCacheState{}, fmt.Errorf("site_members lookup for %s: %w", userID, err)
	}

	return userCacheState{Role: "USER", TenantID: zeroUUID, OrgID: zeroUUID}, nil
}

// reconcileUserCache recomputes and persists the cache triple for one user.
// Idempotent: running it twice yields the same row. Must be called inside the
// caller's transaction so the mirror commits with the membership change.
func reconcileUserCache(tx *gorm.DB, userID string) error {
	if userID == "" {
		return fmt.Errorf("reconcileUserCache: empty user id")
	}
	state, err := resolveUserCache(tx, userID)
	if err != nil {
		return err
	}
	if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"role":       state.Role,
		"tenant_id":  state.TenantID,
		"org_id":     state.OrgID,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("reconcileUserCache(%s): %w", userID, err)
	}
	log.Printf("[UserCache] reconciled user=%s role=%s tenant=%s org=%s",
		userID, state.Role, state.TenantID, state.OrgID)
	return nil
}
