package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"gorm.io/gorm"
)

func setupPropertyResolutionTest(t *testing.T) (*gorm.DB, string) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return nil, ""
	}
	database.SetDB(db)

	db.Exec("DELETE FROM instrument_properties")
	db.Exec("DELETE FROM property_options")
	db.Exec("DELETE FROM instruments")
	db.Exec("DELETE FROM categories")

	tenantID := uuid.New().String()
	now := time.Now()

	db.Exec("INSERT INTO properties (id, name, tenant_id, property_type, caption, scope_type, status, created_at, updated_at) VALUES (?, ?, ?, 'text', ?, ?, 'active', ?, ?)",
		uuid.New().String(), "产地", tenantID, "产地", "global", now, now)
	propBrandID := uuid.New().String()
	db.Exec("INSERT INTO properties (id, name, tenant_id, property_type, caption, scope_type, status, created_at, updated_at) VALUES (?, ?, ?, 'text', ?, ?, 'active', ?, ?)",
		propBrandID, "品牌", tenantID, "品牌", "category", now, now)
	catID := uuid.New().String()
	db.Exec("INSERT INTO categories (id, name, tenant_id, created_at) VALUES (?, ?, ?, ?)",
		catID, "钢琴", tenantID, now)

	// Set related_category_id on brand property
	db.Exec("UPDATE properties SET related_category_id = ? WHERE id = ?", catID, propBrandID)

	return db, tenantID
}

func TestBuildPropertyResolutions_Confirmed(t *testing.T) {
	db, tenantID := setupPropertyResolutionTest(t)
	if db == nil {
		return
	}
	defer db.Exec("DELETE FROM instrument_properties")
	defer db.Exec("DELETE FROM property_options")
	defer db.Exec("DELETE FROM instruments")
	defer db.Exec("DELETE FROM categories")
	defer db.Exec("DELETE FROM properties WHERE tenant_id = ?", tenantID)

	// Create a confirmed option for global-scoped property "产地" value "广东"
	db.Exec("INSERT INTO property_options (id, property_name, value, status, tenant_id, created_at) VALUES (?, ?, '广东', 'confirmed', ?, ?)",
		uuid.New().String(), "产地", tenantID, time.Now())

	var properties []models.Property
	db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&properties)

	validations := []RowValidation{
		{
			Row: 2, SN: "SN001", Valid: true,
			Fields: map[string]interface{}{"prop_产地": "广东"},
		},
	}

	resolutions := buildPropertyResolutions(validations, properties, db, tenantID)
	require.Len(t, resolutions, 1)
	assert.Equal(t, "confirmed", resolutions[0].Status)
	assert.Equal(t, "产地", resolutions[0].PropertyName)
	assert.Equal(t, "广东", resolutions[0].Value)
}

func TestBuildPropertyResolutions_Alias(t *testing.T) {
	db, tenantID := setupPropertyResolutionTest(t)
	if db == nil {
		return
	}
	defer db.Exec("DELETE FROM instrument_properties")
	defer db.Exec("DELETE FROM property_options")
	defer db.Exec("DELETE FROM instruments")
	defer db.Exec("DELETE FROM categories")
	defer db.Exec("DELETE FROM properties WHERE tenant_id = ?", tenantID)

	// Create target confirmed option
	targetID := uuid.New().String()
	db.Exec("INSERT INTO property_options (id, property_name, value, status, tenant_id, created_at) VALUES (?, ?, '雅马哈', 'confirmed', ?, ?)",
		targetID, "品牌", tenantID, time.Now())
	// Create alias option: "yamaha" -> alias to target
	db.Exec("INSERT INTO property_options (id, property_name, value, status, alias, tenant_id, created_at) VALUES (?, ?, 'yamaha', 'confirmed', ?, ?, ?)",
		uuid.New().String(), "品牌", targetID, tenantID, time.Now())

	var properties []models.Property
	db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&properties)

	validations := []RowValidation{
		{
			Row: 2, SN: "SN001", Valid: true,
			Fields: map[string]interface{}{"prop_品牌": "yamaha"},
		},
	}

	resolutions := buildPropertyResolutions(validations, properties, db, tenantID)
	require.Len(t, resolutions, 1)
	assert.Equal(t, "alias", resolutions[0].Status)
	assert.Contains(t, resolutions[0].ResolvedValue, "雅马哈")
}

func TestBuildPropertyResolutions_New(t *testing.T) {
	db, tenantID := setupPropertyResolutionTest(t)
	if db == nil {
		return
	}
	defer db.Exec("DELETE FROM instrument_properties")
	defer db.Exec("DELETE FROM property_options")
	defer db.Exec("DELETE FROM instruments")
	defer db.Exec("DELETE FROM categories")
	defer db.Exec("DELETE FROM properties WHERE tenant_id = ?", tenantID)

	var properties []models.Property
	db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&properties)

	validations := []RowValidation{
		{
			Row: 2, SN: "SN001", Valid: true,
			Fields: map[string]interface{}{
				"prop_产地": "广东",
				"prop_品牌": "珠江",
			},
		},
	}

	resolutions := buildPropertyResolutions(validations, properties, db, tenantID)
	assert.Equal(t, 2, len(resolutions))
	for _, r := range resolutions {
		assert.Equal(t, "new", r.Status)
	}
}

func TestBuildPropertyResolutions_Pending(t *testing.T) {
	db, tenantID := setupPropertyResolutionTest(t)
	if db == nil {
		return
	}
	defer db.Exec("DELETE FROM instrument_properties")
	defer db.Exec("DELETE FROM property_options")
	defer db.Exec("DELETE FROM instruments")
	defer db.Exec("DELETE FROM categories")
	defer db.Exec("DELETE FROM properties WHERE tenant_id = ?", tenantID)

	db.Exec("INSERT INTO property_options (id, property_name, value, status, tenant_id, created_at) VALUES (?, ?, '广东', 'pending', ?, ?)",
		uuid.New().String(), "产地", tenantID, time.Now())

	var properties []models.Property
	db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&properties)

	validations := []RowValidation{
		{
			Row: 2, SN: "SN001", Valid: true,
			Fields: map[string]interface{}{"prop_产地": "广东"},
		},
	}

	resolutions := buildPropertyResolutions(validations, properties, db, tenantID)
	require.Len(t, resolutions, 1)
	assert.Equal(t, "pending", resolutions[0].Status)
}

func TestBuildPropertyResolutions_CategoryScoped_Independent(t *testing.T) {
	db, tenantID := setupPropertyResolutionTest(t)
	if db == nil {
		return
	}
	defer db.Exec("DELETE FROM instrument_properties")
	defer db.Exec("DELETE FROM property_options")
	defer db.Exec("DELETE FROM instruments")
	defer db.Exec("DELETE FROM categories")
	defer db.Exec("DELETE FROM properties WHERE tenant_id = ?", tenantID)

	catPianoID := uuid.New().String()
	catViolinID := uuid.New().String()
	db.Exec("INSERT INTO categories (id, name, tenant_id, created_at) VALUES (?, ?, ?, ?)", catPianoID, "钢琴", tenantID, time.Now())
	db.Exec("INSERT INTO categories (id, name, tenant_id, created_at) VALUES (?, ?, ?, ?)", catViolinID, "小提琴", tenantID, time.Now())

	// Create a category-scoped property "品牌" with a non-nil RelatedCategoryID to enable scoping
	catScopedPropID := uuid.New().String()
	db.Exec("INSERT INTO properties (id, name, tenant_id, property_type, caption, scope_type, related_category_id, status, created_at, updated_at) VALUES (?, ?, ?, 'text', ?, 'category', ?, 'active', ?, ?)",
		catScopedPropID, "品牌", tenantID, "品牌", catPianoID, time.Now(), time.Now())

	var properties []models.Property
	db.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&properties)

	// Two rows with same property value "雅马哈" but different categories
	validations := []RowValidation{
		{
			Row: 2, SN: "SN001", Valid: true,
			Fields: map[string]interface{}{
				"prop_品牌":      "雅马哈",
				"category_id": catPianoID,
			},
		},
		{
			Row: 3, SN: "SN002", Valid: true,
			Fields: map[string]interface{}{
				"prop_品牌":      "雅马哈",
				"category_id": catViolinID,
			},
		},
	}

	res := buildPropertyResolutions(validations, properties, db, tenantID)

	// Same value under different categories should be separate resolutions
	require.Len(t, res, 2, "Two rows with same value but different categories should produce 2 separate resolutions")

	var pianoRes, violinRes *PropertyResolution
	for i := range res {
		if res[i].ScopeCategoryID == catPianoID {
			pianoRes = &res[i]
		} else if res[i].ScopeCategoryID == catViolinID {
			violinRes = &res[i]
		}
	}
	require.NotNil(t, pianoRes, "Should have a resolution for piano category")
	require.NotNil(t, violinRes, "Should have a resolution for violin category")
	assert.Equal(t, "new", pianoRes.Status)
	assert.Equal(t, "new", violinRes.Status)
	assert.Equal(t, "钢琴", pianoRes.ScopeCategoryName)
	assert.Equal(t, "小提琴", violinRes.ScopeCategoryName)
	assert.NotEqual(t, pianoRes.ScopeCategoryID, violinRes.ScopeCategoryID, "Different category IDs")
}
