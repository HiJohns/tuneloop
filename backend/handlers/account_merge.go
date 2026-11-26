package handlers

// #2445: local tuneloop cache migration for an IAM account merge (X→Y).
//
// IAM is authoritative and already moved user_org_relations (including
// functional roles) and marked X obsolete. The local staff tables below are
// keyed by the LOCAL users.id (not IAM sub), so they must follow:
//   - site_members.user_id
//   - merchant_members.user_id
//   - technician_profiles.user_id
// Business data (orders/points/notifications) intentionally stays with X for
// audit; only the staff identity moves to Y.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"tuneloop-backend/models"
)

const zeroUUID = "00000000-0000-0000-0000-000000000000"

// findLocalUserByIAMSub resolves the local cache row for an IAM subject.
// Legacy rows may store the IAM sub directly in users.id (without iam_sub).
func findLocalUserByIAMSub(tx *gorm.DB, iamSub string) (*models.User, error) {
	var u models.User
	if err := tx.Where("iam_sub = ?", iamSub).First(&u).Error; err == nil {
		return &u, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Legacy rows may store the IAM sub in users.id. Only attempt this when
	// the subject is a UUID — Postgres rejects a cast otherwise (22P02).
	if _, parseErr := uuid.Parse(iamSub); parseErr == nil {
		if err := tx.Where("id = ?", iamSub).First(&u).Error; err == nil {
			return &u, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// migrateMergedAccountLocal moves staff rows from X's local user to Y's local
// user. Idempotent: a second run finds no rows for X and returns nil.
func migrateMergedAccountLocal(db *gorm.DB, fromIAMSub, toIAMSub string) error {
	fromIAMSub = strings.TrimSpace(fromIAMSub)
	toIAMSub = strings.TrimSpace(toIAMSub)
	if fromIAMSub == "" || toIAMSub == "" {
		return fmt.Errorf("migrateMergedAccountLocal: empty iam_sub (from=%q to=%q)", fromIAMSub, toIAMSub)
	}
	if fromIAMSub == toIAMSub {
		return fmt.Errorf("migrateMergedAccountLocal: cannot merge %s into itself", fromIAMSub)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		fromUser, err := findLocalUserByIAMSub(tx, fromIAMSub)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Nothing cached locally for X — still ensure Y exists? No local
			// staff rows can point at X if X has no local row.
			log.Printf("[AccountMerge] local X not found (iam_sub=%s); nothing to migrate", fromIAMSub)
			return nil
		}
		if err != nil {
			return err
		}

		toUser, err := findLocalUserByIAMSub(tx, toIAMSub)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			toUser = &models.User{
				ID:       uuid.NewString(),
				IAMSub:   toIAMSub,
				TenantID: fromUser.TenantID,
				OrgID:    fromUser.OrgID,
				Name:     fromUser.Name,
				Nickname: fromUser.Nickname,
				Phone:    fromUser.Phone,
				Email:    fromUser.Email,
				Role:     "USER",
				Status:   "active",
				IsShadow: true,
			}
			if toUser.TenantID == "" {
				toUser.TenantID = zeroUUID
			}
			if toUser.OrgID == "" {
				toUser.OrgID = zeroUUID
			}
			if err := tx.Create(toUser).Error; err != nil {
				return fmt.Errorf("create local user for %s: %w", toIAMSub, err)
			}
			log.Printf("[AccountMerge] created local shadow user %s for iam_sub=%s", toUser.ID, toIAMSub)
		} else if err != nil {
			return err
		}

		if fromUser.ID == toUser.ID {
			return nil
		}

		// #2567 Rev.3（Round 1 Audit Bug 1）：X 的 iam_sub 必须按**合并前**的本地值留痕。
		// Legacy 行的 iam_sub 为空，下面对它的回填只是为了让后续反查可用；审计若读回填后的
		// 值就丢失了「这是一条 legacy 行」这一事实（反查仍靠 from_local）。
		legacyFromIAMSub := fromUser.IAMSub

		// Ensure both rows carry their IAM sub for future reverse lookups.
		if fromUser.IAMSub == "" {
			if err := tx.Model(&models.User{}).Where("id = ?", fromUser.ID).Update("iam_sub", fromIAMSub).Error; err != nil {
				return err
			}
		}
		if toUser.IAMSub == "" {
			if err := tx.Model(&models.User{}).Where("id = ?", toUser.ID).Update("iam_sub", toIAMSub).Error; err != nil {
				return err
			}
		}

		if err := migrateSiteMembers(tx, fromUser.ID, toUser.ID); err != nil {
			return err
		}
		if err := migrateMerchantMembers(tx, fromUser.ID, toUser.ID); err != nil {
			return err
		}
		if err := migrateTechnicianProfiles(tx, fromUser.ID, toUser.ID); err != nil {
			return err
		}

		// Local cache mirrors IAM authority: X obsolete, Y active.
		if err := tx.Model(&models.User{}).Where("id = ?", fromUser.ID).
			Updates(map[string]interface{}{"status": "obsolete", "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.User{}).Where("id = ?", toUser.ID).
			Updates(map[string]interface{}{"status": "active", "updated_at": time.Now()}).Error; err != nil {
			return err
		}

		// #2567 D2: the three cache columns are recomputed from the membership
		// rows we just moved — never written inline. Y inherits the org identity
		// (visible in 人员管理), X falls back to the customer state.
		if err := reconcileUserCache(tx, toUser.ID); err != nil {
			return err
		}
		if err := reconcileUserCache(tx, fromUser.ID); err != nil {
			return err
		}

		// #2567 D7（Rev.2）：X is obsolete, so its *local* pending work must be
		// closed instead of being re-homed or left dangling. IAM's
		// user_task_queues is ops-side (tuneloop must not touch the IAM DB).
		invites, sessions, err := closeMergedAccountLocalTodo(tx, fromUser.ID)
		if err != nil {
			return err
		}

		// #2567 Rev.3（Round 1 Audit Bug 1）：审计归属必须取**合并后已落库**的行。
		// reconcileUserCache 只用 SQL 写回，内存里的 fromUser/toUser 仍是合并前的值
		// （A 为纯顾客 → nil/nil/USER），直接拿它们写审计会把 org_id 写成 nil UUID、
		// to_iam_sub 写成空串，org 维度反查就断了。因此在这里重新读回两行。
		var fromAfter, toAfter models.User
		if err := tx.Where("id = ?", fromUser.ID).First(&fromAfter).Error; err != nil {
			return fmt.Errorf("reload merged-away user %s: %w", fromUser.ID, err)
		}
		if err := tx.Where("id = ?", toUser.ID).First(&toAfter).Error; err != nil {
			return fmt.Errorf("reload surviving user %s: %w", toUser.ID, err)
		}

		if err := writeAccountMergeAudit(tx, &fromAfter, &toAfter, legacyFromIAMSub, invites, sessions); err != nil {
			return err
		}

		log.Printf("[AccountMerge] local staff data migrated: %s (%s) -> %s (%s); closed invites=%d confirm_sessions=%d",
			fromUser.ID, fromIAMSub, toUser.ID, toIAMSub, invites, sessions)
		return nil
	})
}

// closeMergedAccountLocalTodo closes X's *local* pending work when X becomes
// obsolete (#2567 D7 / Rev.2). Semantic is "close", not "re-home": the source
// account is retired, so its pending invites/confirmations must not continue
// under the surviving identity.
//
// Only local tables are touched — IAM's user_task_queues is ops-side and
// tuneloop is forbidden to connect to the IAM database.
func closeMergedAccountLocalTodo(tx *gorm.DB, fromLocalID string) (int64, int64, error) {
	var invites, sessions int64
	res := tx.Model(&models.StaffInvite{}).
		Where("invitee_user_id = ? AND status = ?", fromLocalID, "pending").
		Updates(map[string]interface{}{
			"status":        "rejected",
			"reject_reason": "account_merged",
			"updated_at":    time.Now(),
		})
	if res.Error != nil {
		return 0, 0, fmt.Errorf("close staff_invites for %s: %w", fromLocalID, res.Error)
	}
	invites = res.RowsAffected

	if !tx.Migrator().HasTable(&models.ConfirmationSession{}) {
		// #2543 created this table; older databases may not have it yet.
		log.Printf("[AccountMerge] confirmation_sessions table missing; skip local todo close for %s", fromLocalID)
		return invites, 0, nil
	}
	res = tx.Model(&models.ConfirmationSession{}).
		Where("user_id = ? AND status = ?", fromLocalID, "waiting").
		Updates(map[string]interface{}{"status": "cancelled", "updated_at": time.Now()})
	if res.Error != nil {
		return invites, 0, fmt.Errorf("close confirmation_sessions for %s: %w", fromLocalID, res.Error)
	}
	sessions = res.RowsAffected
	return invites, sessions, nil
}

// writeAccountMergeAudit records ACCOUNT_MERGE inside the merge transaction.
//
// Unlike the fire-and-forget writeAuditLog helper, a failure here fails the
// merge: "who absorbed which staff account" must never be lost (#2567 D5).
//
// fromUser/toUser must be the **post-merge persisted** rows: the cache triple is
// written by reconcileUserCache via SQL, so callers reload before calling
// (Rev.3 / Round 1 Audit Bug 1). legacyFromIAMSub keeps X's pre-merge local
// iam_sub (empty for legacy rows) so the audit records history instead of the
// backfilled value.
func writeAccountMergeAudit(tx *gorm.DB, fromUser, toUser *models.User, legacyFromIAMSub string, invites, sessions int64) error {
	// X's iam_sub as it was before the merge backfill: empty for legacy rows is
	// the historical truth and must not be replaced by the backfilled value
	// (traceability is preserved through from_local, which is always recorded).
	fromSub := legacyFromIAMSub
	details := map[string]interface{}{
		"from_local":   fromUser.ID,
		"from_iam_sub": fromSub,
		"to_local":     toUser.ID,
		"to_iam_sub":   toUser.IAMSub,
		"org_id":       toUser.OrgID,
		"source":       "qr_bind",
		// 组织归属来自成员关系重算（reconcileUserCache），不是合并前的缓存列。
		"reconcile_source":        "membership",
		"invites_closed":          invites,
		"confirm_sessions_closed": sessions,
	}
	b, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("account merge audit details: %w", err)
	}
	d := string(b)
	tenant := toUser.TenantID
	if tenant == "" || tenant == zeroUUID {
		tenant = fromUser.TenantID
	}
	if tenant == "" {
		tenant = zeroUUID
	}
	row := models.AuditLog{
		ID:           uuid.NewString(),
		TenantID:     tenant,
		UserID:       toUser.ID,
		ActorName:    toUser.Name,
		Action:       "ACCOUNT_MERGE",
		ResourceType: "user",
		ResourceID:   fromUser.ID,
		Details:      &d,
		CreatedAt:    time.Now(),
	}
	if toUser.OrgID != "" && toUser.OrgID != zeroUUID {
		org := toUser.OrgID
		row.OrgID = &org
	}
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("account merge audit write: %w", err)
	}
	return nil
}

func migrateSiteMembers(tx *gorm.DB, fromLocalID, toLocalID string) error {
	var rows []models.SiteMember
	if err := tx.Where("user_id = ?", fromLocalID).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		var existing models.SiteMember
		err := tx.Where("tenant_id = ? AND site_id = ? AND user_id = ?", row.TenantID, row.SiteID, toLocalID).
			First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Model(&models.SiteMember{}).Where("id = ?", row.ID).
				Update("user_id", toLocalID).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			// Y already belongs to this site: keep one row, union roles, and
			// prefer the active/primary state.
			existing.Roles = unionStrings(existing.Roles, row.Roles, existing.Role, row.Role)
			if existing.Role == "" {
				existing.Role = row.Role
			}
			if existing.Status != "active" && row.Status == "active" {
				existing.Status = "active"
			}
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", row.ID).Delete(&models.SiteMember{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func migrateMerchantMembers(tx *gorm.DB, fromLocalID, toLocalID string) error {
	var rows []models.MerchantMember
	if err := tx.Where("user_id = ?", fromLocalID).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		var existing models.MerchantMember
		// merchant_members has a global unique index on user_id.
		err := tx.Where("user_id = ?", toLocalID).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Model(&models.MerchantMember{}).Where("id = ?", row.ID).
				Update("user_id", toLocalID).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			existing.CusPermCodes = pq.StringArray(unionStrings([]string(existing.CusPermCodes), []string(row.CusPermCodes)))
			if existing.Role == "" {
				existing.Role = row.Role
			}
			if existing.Status != "active" && row.Status == "active" {
				existing.Status = "active"
			}
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", row.ID).Delete(&models.MerchantMember{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func migrateTechnicianProfiles(tx *gorm.DB, fromLocalID, toLocalID string) error {
	var rows []models.TechnicianProfile
	if err := tx.Where("user_id = ?", fromLocalID).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		var existing models.TechnicianProfile
		err := tx.Where("user_id = ? AND tenant_id = ?", toLocalID, row.TenantID).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Model(&models.TechnicianProfile{}).Where("id = ?", row.ID).
				Update("user_id", toLocalID).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			// Keep Y's profile; fill empty descriptive fields from X.
			if existing.Photo == "" {
				existing.Photo = row.Photo
			}
			if existing.Bio == "" {
				existing.Bio = row.Bio
			}
			if (existing.Experience == "" || existing.Experience == "[]") && row.Experience != "" {
				existing.Experience = row.Experience
			}
			if existing.Status != "active" && row.Status == "active" {
				existing.Status = "active"
			}
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", row.ID).Delete(&models.TechnicianProfile{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// unionStrings dedupes non-empty values while preserving first-seen order.
func unionStrings(groups ...interface{}) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, g := range groups {
		switch v := g.(type) {
		case string:
			add(v)
		case []string:
			for _, s := range v {
				add(s)
			}
		case pq.StringArray:
			for _, s := range v {
				add(s)
			}
		}
	}
	return out
}
