package handlers

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

func setupMigrateBannerWebPTest(t *testing.T) (string, func()) {
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return "", func() {}
	}
	database.SetDB(db)

	// Clean banners table
	_ = db.Migrator().DropTable(&models.Banner{})
	require.NoError(t, db.Migrator().CreateTable(&models.Banner{}))

	// Create temp upload dir
	tmpDir := t.TempDir()
	uploadDir := filepath.Join(tmpDir, "uploads", "media")
	require.NoError(t, os.MkdirAll(uploadDir, 0755))

	origDir, err := os.Getwd()
	require.NoError(t, err)

	cleanup := func() {
		os.Chdir(origDir)
		db.Migrator().DropTable(&models.Banner{})
	}

	require.NoError(t, os.Chdir(tmpDir))
	return uploadDir, cleanup
}

func createTestJPEG(t *testing.T, path string) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 150))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
}

func createTestPNG(t *testing.T, path string) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 150))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
}

func insertBanner(t *testing.T, imageURL string) {
	db := database.GetDB()
	banner := models.Banner{
		ID:       uuid.New().String(),
		TenantID: uuid.New().String(),
		ImageURL: imageURL,
		Status:   "active",
	}
	require.NoError(t, db.Create(&banner).Error)
}

func TestMigrateBannerImagesToWebP_HappyPath(t *testing.T) {
	uploadDir, cleanup := setupMigrateBannerWebPTest(t)
	defer cleanup()

	key := "test_banner_happy.jpg"
	createTestJPEG(t, filepath.Join(uploadDir, key))
	insertBanner(t, "/uploads/media/"+key)

	count, err := MigrateBannerImagesToWebP(false)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify DB was updated
	db := database.GetDB()
	var banner models.Banner
	require.NoError(t, db.First(&banner).Error)
	assert.Equal(t, "/uploads/media/test_banner_happy.webp", banner.ImageURL)

	// Verify webp file exists
	_, err = os.Stat(filepath.Join(uploadDir, "test_banner_happy.webp"))
	assert.NoError(t, err, "webp file should exist")

	// Source .jpg should still exist (kept for safety)
	_, err = os.Stat(filepath.Join(uploadDir, "test_banner_happy.jpg"))
	assert.NoError(t, err, "source jpg should be preserved")
}

func TestMigrateBannerImagesToWebP_Idempotent(t *testing.T) {
	uploadDir, cleanup := setupMigrateBannerWebPTest(t)
	defer cleanup()

	key := "banner_already_webp.webp"
	createTestJPEG(t, filepath.Join(uploadDir, key))
	insertBanner(t, "/uploads/media/"+key)

	count, err := MigrateBannerImagesToWebP(false)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "already .webp banners should be skipped")
}

func TestMigrateBannerImagesToWebP_Mixed(t *testing.T) {
	uploadDir, cleanup := setupMigrateBannerWebPTest(t)
	defer cleanup()

	// One .jpg that needs conversion
	createTestJPEG(t, filepath.Join(uploadDir, "banner1.jpg"))
	insertBanner(t, "/uploads/media/banner1.jpg")

	// One already .webp
	createTestJPEG(t, filepath.Join(uploadDir, "banner2.webp"))
	insertBanner(t, "/uploads/media/banner2.webp")

	// One .png that needs conversion
	createTestPNG(t, filepath.Join(uploadDir, "banner3.png"))
	insertBanner(t, "/uploads/media/banner3.png")

	count, err := MigrateBannerImagesToWebP(false)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "jpg and png should be converted, webp skipped")

	// Verify both converted banners have .webp in DB
	db := database.GetDB()
	var banners []models.Banner
	require.NoError(t, db.Order("image_url asc").Find(&banners).Error)
	require.Len(t, banners, 3)

	assert.Equal(t, "/uploads/media/banner1.webp", banners[0].ImageURL)
	assert.Equal(t, "/uploads/media/banner2.webp", banners[1].ImageURL) // unchanged
	assert.Equal(t, "/uploads/media/banner3.webp", banners[2].ImageURL)
}

func TestMigrateBannerImagesToWebP_DryRun(t *testing.T) {
	uploadDir, cleanup := setupMigrateBannerWebPTest(t)
	defer cleanup()

	key := "banner_dryrun.jpg"
	createTestJPEG(t, filepath.Join(uploadDir, key))
	insertBanner(t, "/uploads/media/"+key)

	count, err := MigrateBannerImagesToWebP(true)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "dry run should report 1 would convert")

	// DB should NOT be updated
	db := database.GetDB()
	var banner models.Banner
	require.NoError(t, db.First(&banner).Error)
	assert.Equal(t, "/uploads/media/banner_dryrun.jpg", banner.ImageURL)

	// No webp file should be written
	_, err = os.Stat(filepath.Join(uploadDir, "banner_dryrun.webp"))
	assert.True(t, os.IsNotExist(err), "no webp file should be written in dry run")
}

func TestMigrateBannerImagesToWebP_SourceMissing(t *testing.T) {
	_, cleanup := setupMigrateBannerWebPTest(t)
	defer cleanup()

	// Banner points to a file that doesn't exist on disk
	insertBanner(t, "/uploads/media/nonexistent.jpg")

	count, err := MigrateBannerImagesToWebP(false)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "missing source should not be counted as converted")

	// DB should still have .jpg
	db := database.GetDB()
	var banner models.Banner
	require.NoError(t, db.First(&banner).Error)
	assert.Equal(t, "/uploads/media/nonexistent.jpg", banner.ImageURL)
}

func TestMigrateBannerImagesToWebP_ExternalURL(t *testing.T) {
	_, cleanup := setupMigrateBannerWebPTest(t)
	defer cleanup()

	insertBanner(t, "https://example.com/banner.jpg")

	count, err := MigrateBannerImagesToWebP(false)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "external URLs should be skipped")
}
