package services

import (
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

func CheckAndUpgradeLevel(userID string, db *gorm.DB) error {
	if db == nil {
		db = database.GetDB()
	}
	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	var levels []models.MembershipLevel
	if err := db.Order("id ASC").Find(&levels).Error; err != nil {
		return err
	}
	newLevelID := 1
	for _, l := range levels {
		if user.TotalSpending >= l.MinAmount {
			newLevelID = l.ID
		}
	}
	if user.MembershipLevelID == nil || *user.MembershipLevelID < newLevelID {
		if err := db.Model(&user).Update("membership_level_id", newLevelID).Error; err != nil {
			return err
		}
	}
	return nil
}
