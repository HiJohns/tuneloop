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
		log.Printf("[AccountMerge] local staff data migrated: %s (%s) -> %s (%s)",
			fromUser.ID, fromIAMSub, toUser.ID, toIAMSub)
		return nil
	})
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
