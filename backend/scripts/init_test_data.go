//go:build ignore

package main

import (
	"fmt"
	"log"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

func main() {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	tenantID := "00000000-0000-0000-0000-000000000000"

	if err := createTestData(db, tenantID); err != nil {
		log.Fatalf("Failed to create test data: %v", err)
	}

	fmt.Println("✅ Test data created successfully!")
	fmt.Println("📋 Summary:")
	fmt.Printf("   - 1 Service Site\n")
	fmt.Printf("   - 1 Technician\n")
	fmt.Printf("   - 3 Instruments\n")
}

func createTestData(db *gorm.DB, tenantID string) error {
	fmt.Println("🔄 Creating test data...")

	site := models.Site{
		TenantID:      tenantID,
		Name:          "中央维修中心",
		Address:       "北京市朝阳区音乐街88号",
		Phone:         "010-12345678",
		BusinessHours: "09:00-18:00",
		Latitude:      39.9042,
		Longitude:     116.4074,
		Status:        "active",
	}

	if err := db.Create(&site).Error; err != nil {
		return fmt.Errorf("failed to create site: %w", err)
	}
	fmt.Printf("✓ Created site: %s\n", site.Name)

	technician := models.Technician{
		TenantID: tenantID,
		SiteID:   site.ID,
		Name:     "张师傅",
		Phone:    "13800138001",
	}

	if err := db.Create(&technician).Error; err != nil {
		return fmt.Errorf("failed to create technician: %w", err)
	}
	fmt.Printf("✓ Created technician: %s\n", technician.Name)

	instruments := []models.Instrument{
		{
			// models.Instrument no longer has Name/Brand: the display name is
			// folded into Description and brand/model live in the dynamic
			// property system (instrument_properties), which this seed skips.
			TenantID:    tenantID,
			SN:          "SEED-PIANO-U1",
			Level:       "professional",
			LevelName:   "专业级",
			Description: "雅马哈立式钢琴 U1：日本原装进口，音色纯净，适合进阶学习者",
			StockStatus: "available",
			Pricing:     `{"3month": 800, "6month": 750, "12month": 700}`,
		},
		{
			TenantID:    tenantID,
			SN:          "SEED-GUITAR-214CE",
			Level:       "professional",
			LevelName:   "专业级",
			Description: "泰勒民谣吉他 214ce：美国品牌，云杉面板，玫瑰木背侧板",
			StockStatus: "available",
			Pricing:     `{"3month": 600, "6month": 550, "12month": 500}`,
		},
		{
			TenantID:    tenantID,
			SN:          "SEED-VIOLIN-STRAD",
			Level:       "master",
			LevelName:   "大师级",
			Description: "斯特拉迪瓦里小提琴 4/4：意大利手工制作，音质卓越",
			StockStatus: "available",
			Pricing:     `{"3month": 1200, "6month": 1100, "12month": 1000}`,
		},
	}

	for i := range instruments {
		if err := db.Create(&instruments[i]).Error; err != nil {
			return fmt.Errorf("failed to create instrument: %w", err)
		}
		fmt.Printf("✓ Created instrument: %s\n", instruments[i].SN)
	}

	return nil
}
