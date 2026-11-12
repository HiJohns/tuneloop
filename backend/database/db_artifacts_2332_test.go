package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #2327 S3b: package / credit_path 迁移工件登记完整性（纯数据断言，不依赖数据库）。
func TestMigrationArtifacts2332Registered(t *testing.T) {
	want := map[uint]map[string]bool{
		20261112002: {"credit_package": false, "credit_package_expire_at": false},
		20261112003: {"deposit_credit_path": false},
	}
	for _, a := range defaultMigrationArtifacts {
		cols, ok := want[a.Version]
		if !ok || a.Table != "orders" {
			continue
		}
		if _, known := cols[a.Column]; known {
			cols[a.Column] = true
		}
	}
	for version, cols := range want {
		for col, registered := range cols {
			if !registered {
				t.Fatalf("migration artifact %d orders.%s not registered", version, col)
			}
		}
	}
}

// 迁移文件必须 up/down 成对存在（#1913 迁移完整性纪律）。
func TestMigrationFiles2332Paired(t *testing.T) {
	dir := "migrations"
	for _, base := range []string{
		"20261112002_payscore_package",
		"20261112003_payscore_credit_path",
	} {
		for _, suffix := range []string{".up.sql", ".down.sql"} {
			path := filepath.Join(dir, base+suffix)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing migration file %s: %v", path, err)
			}
			if !strings.Contains(strings.ToLower(string(data)), "orders") {
				t.Fatalf("%s must target the orders table", path)
			}
		}
	}
}
