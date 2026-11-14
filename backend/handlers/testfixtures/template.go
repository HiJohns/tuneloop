package testfixtures

// #2341: 测试库生命周期——模板库 + fail-fast + dirty 自愈。
//
// 生产/测试一致的做法：
//   1. 模板库 tuneloop_test_template 由 AutoMigrate → migrations → seed 构建一次；
//   2. 每次套件运行 CREATE DATABASE <run> TEMPLATE <template>（秒级克隆）；
//   3. 套件结束删除 run 库，模板保留；
//   4. 模板用 meta 戳（模型清单 hash + seed 指纹 + 迁移链指纹/计数/版本）判定是否过期；
//   5. 克隆后校验所有 models.Cents 列必须为 bigint（numeric 会按元 ×100）。
//
// 本文件只被 handlers 测试使用；不进入生产路径。

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"tuneloop-backend/models"
)

const (
	// TemplateDBName 是共享模板库名（匹配 handlers 的 tuneloop_test_* 白名单）。
	TemplateDBName = "tuneloop_test_template"
	// templateLockKey 串行化多节点首建模板。
	templateLockKey = 7341001
	metaTable       = "_test_template_meta"
)

// TemplateOptions 描述模板库如何构建。
type TemplateOptions struct {
	Host, Port, User, Password, SSLMode string
	TemplateName                        string
	Models                              []interface{}
	// PostAutoMigrate 在 AutoMigrate 之后、迁移链重放之前执行（模型无法表达的列/表）。
	PostAutoMigrate func(*gorm.DB) error
	// PostMigrate 在迁移链重放之后执行（迁移依赖的补充种子/视图）。
	PostMigrate func(*gorm.DB) error
	// MigrationsDir 是迁移目录（#2342）；为空时自动探测。
	MigrationsDir string
}

func (o TemplateOptions) dsn(dbname string) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		o.Host, o.Port, o.User, o.Password, dbname, o.SSLMode)
}

// modelHash 对模型清单做稳定 hash，用于模板过期判定。
func modelHash(objs []interface{}) string {
	names := make([]string, 0, len(objs))
	for _, o := range objs {
		names = append(names, reflect.TypeOf(o).String())
	}
	sort.Strings(names)
	sum := sha1.Sum([]byte(strings.Join(names, "\n")))
	return hex.EncodeToString(sum[:])
}

// seedSQL 是模板必须包含的业务种子（来自 migration 20260703001）。
// #2342 起迁移链已整链重放（幂等化 shim）；种子仍显式固化，保证克隆库即便
// 迁移语义变化也一定有 system default pricing template。
const seedSQL = `INSERT INTO pricing_templates (id, code, name, config_schema, is_active, is_system_default, created_at)
	SELECT gen_random_uuid(), 'system_default', '系统默认定价',
	       '{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":180,"discount_percent":5},{"days_max":-1,"discount_percent":10}],"deposit_mode":"ratio","deposit_ratio":0.3,"overdue_multiplier":2.0}',
	       true, true, NOW()
	WHERE NOT EXISTS (SELECT 1 FROM pricing_templates WHERE is_system_default = true)`

// SeedTemplateBaseline 在每次 ResetTestDB 后恢复模板种子（system default pricing）。
func SeedTemplateBaseline(db *gorm.DB) {
	db.Exec(seedSQL)
}

func seedFingerprint() string {
	sum := sha1.Sum([]byte(seedSQL))
	return hex.EncodeToString(sum[:])
}

func dbExists(conn *sql.Conn, name string) (bool, error) {
	var exists bool
	err := conn.QueryRowContext(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists)
	return exists, err
}

func dropDatabase(conn *sql.Conn, name string) error {
	_, err := conn.ExecContext(context.Background(),
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoteIdent(name)))
	return err
}

func createDatabase(conn *sql.Conn, name, template string) error {
	stmt := fmt.Sprintf("CREATE DATABASE %s", quoteIdent(name))
	if template != "" {
		stmt += " TEMPLATE " + quoteIdent(template)
	}
	_, err := conn.ExecContext(context.Background(), stmt)
	return err
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// EnsureTemplate 确保模板库存在且未过期；返回是否发生了重建。
func EnsureTemplate(opts TemplateOptions) (bool, error) {
	admin, err := sql.Open("postgres", opts.dsn("postgres"))
	if err != nil {
		return false, fmt.Errorf("template: open maintenance db: %w", err)
	}
	defer admin.Close()
	conn, err := admin.Conn(context.Background())
	if err != nil {
		return false, fmt.Errorf("template: maintenance conn: %w", err)
	}
	defer conn.Close()

	// 多节点首建串行化。
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", templateLockKey); err != nil {
		return false, fmt.Errorf("template: advisory lock: %w", err)
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", templateLockKey)

	wantHash := modelHash(opts.Models)
	wantSeed := seedFingerprint()
	// #2342: 迁移链指纹进入过期判定；目录缺失/为空必须显式报错（不静默跳过迁移）。
	migrationsDir := opts.MigrationsDir
	if migrationsDir == "" {
		resolved, err := ResolveMigrationsDir()
		if err != nil {
			return false, fmt.Errorf("template: %w", err)
		}
		migrationsDir = resolved
	}
	wantMigrations, wantMigrationCount, err := MigrationSignature(migrationsDir)
	if err != nil {
		return false, fmt.Errorf("template: migrations signature: %w", err)
	}

	exists, err := dbExists(conn, opts.TemplateName)
	if err != nil {
		return false, fmt.Errorf("template: check exists: %w", err)
	}
	if exists {
		ok, metaErr := templateUpToDate(opts, wantHash, wantSeed, wantMigrations, wantMigrationCount)
		if metaErr == nil && ok {
			return false, nil
		}
		// 元数据缺失/过期/脏 → 重建（D2/D5：确定性优于修补）。
	}

	if err := rebuildTemplate(conn, opts, templateMeta{
		modelHash:      wantHash,
		seedHash:       wantSeed,
		migrationsHash: wantMigrations,
		migrationsDir:  migrationsDir,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// templateMeta 是写入模板库的戳（#2342 起含迁移链指纹/计数/版本）。
type templateMeta struct {
	modelHash      string
	seedHash       string
	migrationsHash string
	migrationsDir  string
}

func templateUpToDate(opts TemplateOptions, wantHash, wantSeed, wantMigrations string, wantCount int) (bool, error) {
	// meta 表在模板库内；必须连模板库查询（maintenance 连接看不到它）。
	db, err := sql.Open("postgres", opts.dsn(opts.TemplateName))
	if err != nil {
		return false, err
	}
	defer db.Close()
	var hash, seed, migrationsHash string
	var migrationsCount int
	err = db.QueryRow(fmt.Sprintf(
		"SELECT model_hash, seed_hash, migrations_hash, migrations_count FROM %s WHERE id = 1",
		metaTable)).Scan(&hash, &seed, &migrationsHash, &migrationsCount)
	if err != nil {
		return false, err
	}
	return hash == wantHash && seed == wantSeed &&
		migrationsHash == wantMigrations && migrationsCount == wantCount, nil
}

func rebuildTemplate(conn *sql.Conn, opts TemplateOptions, meta templateMeta) error {
	if err := dropDatabase(conn, opts.TemplateName); err != nil {
		return fmt.Errorf("template: drop: %w", err)
	}
	if err := createDatabase(conn, opts.TemplateName, ""); err != nil {
		return fmt.Errorf("template: create: %w", err)
	}
	// #2342 序：AutoMigrate → shimmed migrations → PostMigrate → seed。
	shimStats, version, err := initTemplateSchema(opts, meta.migrationsDir)
	if err != nil {
		return fmt.Errorf("template: build failed: %w", err)
	}
	fmt.Printf("[test-template] migrations replayed: %s version=%d\n", shimStats, version)
	// 写 meta 戳（模板内，随克隆复制；克隆后 run 库无需再比对）。
	db, err := gorm.Open(postgres.Open(opts.dsn(opts.TemplateName)), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("template: connect for meta: %w", err)
	}
	defer func() {
		if sqlDB, e := db.DB(); e == nil {
			sqlDB.Close()
		}
	}()
	ddl := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id int PRIMARY KEY,
		model_hash text NOT NULL,
		seed_hash text NOT NULL,
		migrations_hash text NOT NULL DEFAULT '',
		migrations_count int NOT NULL DEFAULT 0,
		migrations_version bigint NOT NULL DEFAULT 0,
		built_at timestamptz NOT NULL DEFAULT now()
	)`, metaTable)
	if err := db.Exec(ddl).Error; err != nil {
		return fmt.Errorf("template: meta table: %w", err)
	}
	if err := db.Exec(fmt.Sprintf(`DELETE FROM %s`, metaTable)).Error; err != nil {
		return fmt.Errorf("template: clear meta: %w", err)
	}
	if err := db.Exec(fmt.Sprintf(`INSERT INTO %s (id, model_hash, seed_hash, migrations_hash, migrations_count, migrations_version, built_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)`,
		metaTable), meta.modelHash, meta.seedHash, meta.migrationsHash, shimStats.UpFiles, version, time.Now()).Error; err != nil {
		return fmt.Errorf("template: write meta: %w", err)
	}
	return nil
}

// AutoMigrateMissingTables 只对「数据库中尚不存在」的模型建表。
// 已存在的表由迁移链负责（与生产一致），避免 GORM 在历史 schema 上做
// AlterColumn/DROP CONSTRAINT 造成的假失败（#2342 实测 coupons.uni_coupons_code）。
func AutoMigrateMissingTables(db *gorm.DB, objs []interface{}) error {
	missing := make([]interface{}, 0, len(objs))
	for _, obj := range objs {
		if !db.Migrator().HasTable(obj) {
			missing = append(missing, obj)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := db.AutoMigrate(missing...); err != nil {
		return fmt.Errorf("automigrate missing tables (%d): %w", len(missing), err)
	}
	return nil
}

func initTemplateSchema(opts TemplateOptions, migrationsDir string) (ShimStats, uint, error) {
	var stats ShimStats
	db, err := gorm.Open(postgres.Open(opts.dsn(opts.TemplateName)), &gorm.Config{})
	if err != nil {
		return stats, 0, fmt.Errorf("connect template: %w", err)
	}
	defer func() {
		if sqlDB, e := db.DB(); e == nil {
			sqlDB.Close()
		}
	}()
	// #2342 序：先重放真实迁移链（生产 fresh-install 的同一路径），
	// 再为「没有迁移的模型」补建表（生产不存在这种模型，测试 fixture 需要）。
	stats, version, err := RunShimmedMigrations(opts.dsn(opts.TemplateName), migrationsDir)
	if err != nil {
		return stats, 0, err
	}
	if err := AutoMigrateMissingTables(db, opts.Models); err != nil {
		return stats, version, fmt.Errorf("automigrate-missing: %w", err)
	}
	if opts.PostAutoMigrate != nil {
		if err := opts.PostAutoMigrate(db); err != nil {
			return stats, version, fmt.Errorf("post-automigrate: %w", err)
		}
	}
	if opts.PostMigrate != nil {
		if err := opts.PostMigrate(db); err != nil {
			return stats, version, fmt.Errorf("post-migrate: %w", err)
		}
	}
	if err := db.Exec(seedSQL).Error; err != nil {
		return stats, version, fmt.Errorf("seed: %w", err)
	}
	return stats, version, nil
}

// CloneTemplate 以模板库克隆出本次运行的库（会先删除同名旧库）。
func CloneTemplate(opts TemplateOptions, runDB string) error {
	admin, err := sql.Open("postgres", opts.dsn("postgres"))
	if err != nil {
		return fmt.Errorf("clone: open maintenance db: %w", err)
	}
	defer admin.Close()
	conn, err := admin.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("clone: maintenance conn: %w", err)
	}
	defer conn.Close()
	if err := dropDatabase(conn, runDB); err != nil {
		return fmt.Errorf("clone: drop run db: %w", err)
	}
	if err := createDatabase(conn, runDB, opts.TemplateName); err != nil {
		return fmt.Errorf("clone: create run db from template: %w", err)
	}
	return nil
}

// DropDatabase 删除指定数据库（套件结束后清理 run 库）。
func DropDatabase(opts TemplateOptions, name string) error {
	admin, err := sql.Open("postgres", opts.dsn("postgres"))
	if err != nil {
		return err
	}
	defer admin.Close()
	conn, err := admin.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	return dropDatabase(conn, name)
}

// AssertCentsSchema 克隆后校验模型内所有 Cents 列必须为 bigint（D4）。
// numeric/decimal 会被 models.Cents.Scan 当作「元」×100，必须 fail-fast。
func AssertCentsSchema(db *gorm.DB, objs []interface{}) error {
	type col struct{ table, column string }
	seen := map[col]bool{}
	var bad []string
	cache := &sync.Map{}
	for _, obj := range objs {
		s, err := schema.Parse(obj, cache, schema.NamingStrategy{})
		if err != nil {
			return fmt.Errorf("cents schema: parse %T: %w", obj, err)
		}
		for _, f := range s.Fields {
			if f.FieldType != reflect.TypeOf(models.Cents(0)) {
				continue
			}
			c := col{table: s.Table, column: f.DBName}
			if seen[c] {
				continue
			}
			seen[c] = true
			var dataType string
			if err := db.Raw(`SELECT data_type FROM information_schema.columns
				WHERE table_schema='public' AND table_name=? AND column_name=?`,
				c.table, c.column).Scan(&dataType).Error; err != nil {
				continue
			}
			if dataType != "" && dataType != "bigint" {
				bad = append(bad, fmt.Sprintf("%s.%s=%s", c.table, c.column, dataType))
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("cents schema drifted (expected bigint): %s; rebuild the template with FORCE_TEMPLATE_REBUILD=1",
			strings.Join(bad, ", "))
	}
	return nil
}
