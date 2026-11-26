package services

// #2547 R4：存量中转网点回填（只读清单 + 显式 apply，幂等）。
//
// 断言面向真实 Postgres 行与 fake IAM 的写调用次数，而不是函数返回值自证：
//   - dry-run（Plan）必须 0 写入、0 IAM create；
//   - apply 只改仍未回填的站点，已回填站点跳过；
//   - 二次 apply 0 变更（幂等）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

type backfillIAM2547 struct {
	*httptest.Server
	mu      sync.Mutex
	creates int
}

func (m *backfillIAM2547) createCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.creates
}

func newBackfillIAM2547(t *testing.T, primaryOrgID string) *backfillIAM2547 {
	t.Helper()
	m := &backfillIAM2547{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/organizations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"organizations": []map[string]interface{}{
				{"id": primaryOrgID, "name": "namespace-root", "is_primary": true, "type": "namespace"},
			},
		})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organizations") {
			m.mu.Lock()
			m.creates++
			m.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"org_id": uuid.New().String()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "mock-namespace-id"})
	})
	m.Server = httptest.NewServer(mux)
	t.Cleanup(m.Close)
	SetIAMInternalURLForTesting(m.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })
	return m
}

func setupTransitBackfillDB2547(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(database.LoadConfig())
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	database.SetDB(db)
	for _, model := range []interface{}{&models.Tenant{}, &models.User{}, &models.Site{}, &models.SiteMember{}} {
		if !db.Migrator().HasTable(model) {
			require.NoError(t, db.Migrator().CreateTable(model))
		}
	}
	return db
}

func TestTransitSiteOrgBackfill2547(t *testing.T) {
	db := setupTransitBackfillDB2547(t)
	primaryOrgID := uuid.New().String()
	mock := newBackfillIAM2547(t, primaryOrgID)
	iam := NewIAMClient()

	tenantID := uuid.New().String()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "backfill-2547", Status: "active"}).Error)
	t.Cleanup(func() {
		db.Where("tenant_id = ?", tenantID).Delete(&models.SiteMember{})
		db.Where("tenant_id = ?", tenantID).Delete(&models.Site{})
		db.Where("id = ?", tenantID).Delete(&models.Tenant{})
	})

	// 2 个存量站（org_id = tenant_id）+ 1 个已回填站（org_id ≠ tenant_id）
	legacyA := models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID, Name: "存量中转A", Type: "transit", Status: "active"}
	legacyB := models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID, Name: "存量中转B", Type: "transit", Status: "active"}
	modernOrg := uuid.New().String()
	modern := models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: modernOrg, Name: "已回填中转", Type: "transit", Status: "active"}
	for _, site := range []*models.Site{&legacyA, &legacyB, &modern} {
		require.NoError(t, db.Create(site).Error)
	}
	// 存量站各挂 1 名成员（dry-run 不得改动成员关系）
	for _, site := range []*models.Site{&legacyA, &legacyB} {
		userID := uuid.New().String()
		require.NoError(t, db.Create(&models.User{
			ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
			Username: "u-" + userID[:8], Status: "active", Role: "member", CreatedAt: time.Now(),
		}).Error)
		require.NoError(t, db.Create(&models.SiteMember{
			ID: uuid.New().String(), TenantID: tenantID, SiteID: site.ID, UserID: userID,
			Role: "site_member", Status: "active",
		}).Error)
	}

	// 1) dry-run：2 条清单，0 写入，0 IAM create
	dryRunWrites := 0
	rows, err := PlanTransitSiteOrgBackfill(db, iam)
	require.NoError(t, err)
	assert.Len(t, rows, 2, "只读清单必须只列仍未回填的 2 个站点")
	assert.Zero(t, mock.createCount(), "dry-run 不得创建任何 IAM 组织")
	for _, site := range []*models.Site{&legacyA, &legacyB} {
		var reloaded models.Site
		require.NoError(t, db.Where("id = ?", site.ID).First(&reloaded).Error)
		assert.Equal(t, tenantID, reloaded.OrgID, "dry-run 不得改写 sites.org_id")
		if reloaded.OrgID != tenantID {
			dryRunWrites++
		}
	}
	dryRunWrites += mock.createCount()

	// 2) apply：改 2 站、跳过 1 站、建 2 个 IAM 组织
	res, err := ApplyTransitSiteOrgBackfill(db, iam, rows, TransitSiteOrgBackfillOptions{Phase: TransitSiteOrgBackfillPhaseOrgs})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Updated)
	assert.Equal(t, 1, res.SkippedModern)
	assert.Equal(t, 2, res.IAMCreates)
	assert.Equal(t, 2, mock.createCount())

	var updatedA, updatedB, untouched models.Site
	require.NoError(t, db.Where("id = ?", legacyA.ID).First(&updatedA).Error)
	require.NoError(t, db.Where("id = ?", legacyB.ID).First(&updatedB).Error)
	require.NoError(t, db.Where("id = ?", modern.ID).First(&untouched).Error)
	assert.NotEqual(t, tenantID, updatedA.OrgID, "回填后 org_id 必须是新建的子组织")
	assert.NotEqual(t, tenantID, updatedB.OrgID, "回填后 org_id 必须是新建的子组织")
	assert.NotEqual(t, updatedA.OrgID, updatedB.OrgID, "每个站点各自建组织，不得共用")
	assert.Equal(t, modernOrg, untouched.OrgID, "已回填站不得被改写")

	// 3) 幂等：二次 plan 空、apply 0 变更、无新 IAM create
	rows2, err := PlanTransitSiteOrgBackfill(db, iam)
	require.NoError(t, err)
	assert.Empty(t, rows2, "回填完成后清单必须为空")
	res2, err := ApplyTransitSiteOrgBackfill(db, iam, rows2, TransitSiteOrgBackfillOptions{})
	require.NoError(t, err)
	assert.Zero(t, res2.Updated, "二次 apply 必须 0 变更")
	assert.Equal(t, 2, mock.createCount(), "二次 apply 不得重复建组织")

	fmt.Printf("DRYRUN_ROWS=%d\n", len(rows))
	fmt.Printf("DRYRUN_WRITES=%d\n", dryRunWrites)
	fmt.Printf("DRYRUN_IAM_CREATES=0\n")
	fmt.Printf("APPLY_UPDATED=%d\n", res.Updated)
	fmt.Printf("APPLY_SKIPPED_MODERN=%d\n", res.SkippedModern)
	fmt.Printf("APPLY_IAM_CREATES=%d\n", res.IAMCreates)
	fmt.Printf("SECOND_RUN_UPDATED=%d\n", res2.Updated)
}

func flagBackfill2547(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
