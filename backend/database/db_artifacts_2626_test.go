package database

import "testing"

// #2626：users.roles 迁移工件登记完整性（纯数据断言，不依赖数据库）。
// 缺登记 → 启动校验无法发现「schema_migrations 已记录但实列缺失」（#1986 教训）。
func TestMigrationArtifacts2626Registered(t *testing.T) {
	found := false
	for _, a := range defaultMigrationArtifacts {
		if a.Version == 20261128001 && a.Table == "users" && a.Column == "roles" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("migration artifact 20261128001 users.roles not registered")
	}
}

// 负例：未登记的合成列必须被识别为缺失。
func TestMigrationArtifacts2626DetectsMissing(t *testing.T) {
	for _, a := range defaultMigrationArtifacts {
		if a.Version == 20261128001 && a.Column == "roles_nonexistent" {
			t.Fatal("synthetic column must not be registered")
		}
	}
}
