package services

// #2552 R4：中转网点组织回填工具的 dry-run / apply / 幂等 / members 阶段。
//
// 关键安全语义（D4）：默认 dry-run 必须 **0 写入 0 IAM 建组织**；
// `--apply` 用条件更新保证幂等（二次 apply 不得再更新）；
// `--phase=members` 才动 IAM 成员关系。
//
// 注：`services` 包不能 import `handlers/testfixtures`（后者 import services，
// 会形成测试期 import cycle），故本文件按 `services/*_test.go` 既有做法
// 自建所需表（见 point_batches_test.go）。
//
// Token 行用 fmt.Printf，供 scripts/verify-2552.sh r4 grep。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// setupBackfillDB 在**独立测试库 + 独立 schema** 中建表，绝不触碰开发库
// （沿用 services/settlement_executor_2361_test.go 的隔离口径：
// `TUNELOOP_TEST_DB`，默认 `tuneloop_test`；用完 DROP SCHEMA）。
func setupBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := backfillEnv("TUNELOOP_TEST_DB", "tuneloop_test")
	admin, err := gorm.Open(gormPostgres.Open(backfillDSN("", "postgres")), &gorm.Config{})
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	var exists int64
	require.NoError(t, admin.Raw(`SELECT COUNT(*) FROM pg_database WHERE datname = ?`, dbName).Scan(&exists).Error)
	if exists == 0 {
		require.NoError(t, admin.Exec(`CREATE DATABASE "`+strings.ReplaceAll(dbName, `"`, `""`)+`"`).Error,
			"create test database %s", dbName)
	}
	schema := "transit2552_" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	base, err := gorm.Open(gormPostgres.Open(backfillDSN("", dbName)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, base.Exec("CREATE SCHEMA IF NOT EXISTS "+schema).Error)

	db, err := gorm.Open(gormPostgres.Open(backfillDSN(schema, dbName)), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = base.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error
		if sqlDB, err := base.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	database.SetDB(db)

	for _, m := range []interface{}{&models.Tenant{}, &models.User{}, &models.Site{}, &models.SiteMember{}} {
		require.NoError(t, db.Migrator().CreateTable(m))
	}
	// `models.User.IAMSub` 带 `-:migration`，CreateTable 不建该列；生产由 PostAutoMigrate 补，
	// 这里显式补上（否则成员重绑无法读到 iam_sub）。
	require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''`).Error)
	return db
}

func backfillEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func backfillDSN(schema, dbName string) string {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable default_query_exec_mode=simple_protocol",
		backfillEnv("POSTGRES_HOST", "localhost"),
		backfillEnv("POSTGRES_PORT", "5432"),
		backfillEnv("POSTGRES_USER", "tuneloop"),
		os.Getenv("POSTGRES_PASSWORD"),
		dbName,
	)
	if schema != "" {
		dsn += " search_path=" + schema
	}
	return dsn
}

type backfillIAMRecorder struct {
	mu        sync.Mutex
	creates   int
	deletes   int
	binds     []string
	bindUsers []string
	unbinds   []string
	primary   string
	createOK  bool
}

func (f *backfillIAMRecorder) snapshot() (creates, deletes int, binds, unbinds []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates, f.deletes, append([]string(nil), f.binds...), append([]string(nil), f.unbinds...)
}

func (f *backfillIAMRecorder) boundUsers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.bindUsers...)
}

func readBody(r *http.Request) string {
	data, _ := io.ReadAll(r.Body)
	return string(data)
}

func (f *backfillIAMRecorder) newServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/organizations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"organizations": []map[string]interface{}{
				{"id": f.primary, "name": "平台根组织", "is_primary": true},
			},
		})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/")
		parts := strings.Split(trimmed, "/")
		body := readBody(r)
		if strings.HasSuffix(r.URL.Path, "/bind") && len(parts) >= 4 {
			f.mu.Lock()
			f.bindUsers = append(f.bindUsers, parts[2]) // /{org}/users/{user}/bind
			if strings.Contains(body, `"action":"unbind"`) {
				f.unbinds = append(f.unbinds, parts[0])
			} else {
				f.binds = append(f.binds, parts[0])
			}
			f.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "message": "success"})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/organizations/") && r.Method == http.MethodDelete:
			f.mu.Lock()
			f.deletes++
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/organizations"):
			f.mu.Lock()
			f.creates++
			ok := f.createOK
			f.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"iam down"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"org_id": uuid.New().String(), "admin_id": uuid.New().String(),
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.New().String()})
		}
	})
	srv := httptest.NewServer(mux)
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return srv
}

// seedBackfillFixture 构造 A/B（org=tenant，需回填）、C（已回填）、D（已回填且有 1 成员）。
func seedBackfillFixture(t *testing.T) (*gorm.DB, models.Site, models.Site, models.Site, models.Site, string) {
	t.Helper()
	db := setupBackfillDB(t)

	tenant := uuid.New().String()
	require.NoError(t, db.Create(&models.Tenant{ID: tenant, Name: "平台租户2552", Status: "active"}).Error)

	mk := func(name, orgID string) models.Site {
		site := models.Site{
			ID: uuid.New().String(), TenantID: tenant, OrgID: orgID,
			Name: name, Type: "transit", Status: "active",
		}
		require.NoError(t, db.Create(&site).Error)
		return site
	}
	siteA := mk("回填站A", tenant)               // org == tenant → 需回填
	siteB := mk("回填站B", tenant)               // org == tenant → 需回填
	siteC := mk("已回填站C", uuid.New().String()) // 已回填
	siteD := mk("成员站D", uuid.New().String())  // 已回填，用于 members 阶段

	memberUserID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: memberUserID, IAMSub: memberUserID, TenantID: tenant, OrgID: tenant,
		Name: "成员D", Email: "memberd2552@example.com", Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenant, SiteID: siteD.ID, UserID: memberUserID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)
	return db, siteA, siteB, siteC, siteD, memberUserID
}

func TestTransitBackfill2552_DryRunApplyIdempotentMembers(t *testing.T) {
	db, siteA, siteB, siteC, siteD, memberUserID := seedBackfillFixture(t)

	rec := &backfillIAMRecorder{primary: uuid.New().String(), createOK: true}
	rec.newServer(t)
	iam := NewIAMClient()

	// 1) dry-run：0 写入、0 IAM 建组织、站点行不变。
	before := map[string]string{}
	for _, s := range []models.Site{siteA, siteB, siteC, siteD} {
		var got models.Site
		require.NoError(t, db.Where("id = ?", s.ID).First(&got).Error)
		before[s.ID] = got.OrgID
	}

	dry, err := ApplyTransitSiteOrgBackfill(db, iam, TransitSiteBackfillOptions{Apply: false, Phase: TransitBackfillPhaseOrgs})
	require.NoError(t, err)
	creates, _, _, _ := rec.snapshot()
	unchanged := 0
	for id, org := range before {
		var got models.Site
		require.NoError(t, db.Where("id = ?", id).First(&got).Error)
		if got.OrgID == org {
			unchanged++
		}
	}
	fmt.Printf("DRYRUN_WRITES=%d\n", dry.Updated+dry.Created)
	fmt.Printf("DRYRUN_IAM_CREATES=%d\n", creates)
	fmt.Printf("DRYRUN_UNCHANGED_SITES=%d\n", unchanged)
	require.Zero(t, dry.Created)
	require.Zero(t, dry.Updated)
	require.Zero(t, creates, "dry-run 不得调用 IAM 建组织")
	require.Equal(t, 4, unchanged, "dry-run 不得修改任何站点行")

	// 2) apply（orgs）：A/B 各建 1 个组织 + 更新；C/D 跳过。
	applied, err := ApplyTransitSiteOrgBackfill(db, iam, TransitSiteBackfillOptions{Apply: true, Phase: TransitBackfillPhaseOrgs})
	require.NoError(t, err)
	fmt.Printf("APPLY_CREATED=%d\n", applied.Created)
	fmt.Printf("APPLY_UPDATED=%d\n", applied.Updated)
	fmt.Printf("APPLY_SKIPPED=%d\n", applied.Skipped)
	require.Equal(t, 2, applied.Created)
	require.Equal(t, 2, applied.Updated)
	require.Equal(t, 2, applied.Skipped)

	for _, s := range []models.Site{siteA, siteB} {
		var got models.Site
		require.NoError(t, db.Where("id = ?", s.ID).First(&got).Error)
		require.NotEqual(t, got.TenantID, got.OrgID, "回填后 org_id 不得再等于 tenant_id")
		require.NotEmpty(t, got.OrgID)
	}

	// 3) 二次 apply：幂等（0 更新）。
	again, err := ApplyTransitSiteOrgBackfill(db, iam, TransitSiteBackfillOptions{Apply: true, Phase: TransitBackfillPhaseOrgs})
	require.NoError(t, err)
	fmt.Printf("SECOND_APPLY_UPDATED=%d\n", again.Updated)
	fmt.Printf("SECOND_APPLY_CREATED=%d\n", again.Created)
	require.Zero(t, again.Updated, "条件更新必须保证幂等")
	require.Zero(t, again.Created)

	// 4) members 阶段：D 的 1 个成员重绑到 D 自己的组织。
	_, _, bindsBefore, _ := rec.snapshot()
	members, err := ApplyTransitSiteOrgBackfill(db, iam, TransitSiteBackfillOptions{
		Apply: true, Phase: TransitBackfillPhaseMembers, OperatorID: "ops-2552",
	})
	require.NoError(t, err)
	_, _, binds, unbinds := rec.snapshot()
	fmt.Printf("MEMBERS_REBIND=%d\n", members.MembersRebind)
	fmt.Printf("MEMBERS_BIND_CALLS=%d\n", len(binds)-len(bindsBefore))
	require.Equal(t, 1, members.MembersRebind, "D 站 1 个成员应被重绑")
	require.NotEmpty(t, binds)
	require.Equal(t, siteD.OrgID, binds[len(binds)-1], "成员必须绑到站点自己的组织")
	require.Contains(t, unbinds, siteD.TenantID, "旧组织（tenant）应被 best-effort 解绑")
	require.Contains(t, rec.boundUsers(), memberUserID, "被重绑的必须是 D 站的成员用户")
}

// R4（负例）：建组织失败必须显式返回错误，且不留下「半回填」的站点行。
func TestTransitBackfill2552_IAMFailureExplicit(t *testing.T) {
	db, siteA, _, _, _, _ := seedBackfillFixture(t)

	rec := &backfillIAMRecorder{primary: uuid.New().String(), createOK: false}
	rec.newServer(t)
	iam := NewIAMClient()

	var before models.Site
	require.NoError(t, db.Where("id = ?", siteA.ID).First(&before).Error)

	_, err := ApplyTransitSiteOrgBackfill(db, iam, TransitSiteBackfillOptions{Apply: true, Phase: TransitBackfillPhaseOrgs})
	require.Error(t, err, "IAM 建组织失败必须显式返回（禁止静默跳过）")

	var after models.Site
	require.NoError(t, db.Where("id = ?", siteA.ID).First(&after).Error)
	untouched := 0
	if after.OrgID == before.OrgID {
		untouched = 1
	}
	fmt.Printf("IAM_FAILURE_EXPLICIT=%d\n", 1)
	fmt.Printf("IAM_FAILURE_UNTOUCHED=%d\n", untouched)
	require.Equal(t, 1, untouched, "失败时不得写入 org_id")
}
