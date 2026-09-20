package database

import (
	"testing"
)

// #1986: 迁移工件断言——版本 ≥ 工件版本但表/列缺失 → 报错。
func TestVerifyArtifacts_MissingColumnFails(t *testing.T) {
	cfg := LoadConfig()
	db, err := InitDB(cfg)
	if err != nil {
		t.Skip("dev db not available")
		return
	}
	SetDB(db)

	arts := []migrationArtifact{
		{Version: 1, Table: "orders", Column: "nonexistent_column_xyz", Desc: "synthetic"},
	}
	if err := verifyArtifacts(db, 999999, arts); err == nil {
		t.Fatal("expected failure for missing column, got nil")
	}
}

func TestVerifyArtifacts_MissingTableFails(t *testing.T) {
	cfg := LoadConfig()
	db, err := InitDB(cfg)
	if err != nil {
		t.Skip("dev db not available")
		return
	}
	SetDB(db)

	arts := []migrationArtifact{
		{Version: 1, Table: "definitely_absent_table_xyz", Desc: "synthetic"},
	}
	if err := verifyArtifacts(db, 999999, arts); err == nil {
		t.Fatal("expected failure for missing table, got nil")
	}
}

// 版本门控：工件版本 > 当前 DB 版本 → 跳过（不误报）。
func TestVerifyArtifacts_VersionGateSkips(t *testing.T) {
	arts := []migrationArtifact{
		{Version: 999999999, Table: "definitely_absent_table_xyz", Column: "nope", Desc: "future"},
	}
	if err := verifyArtifacts(nil, 1, arts); err != nil {
		t.Fatalf("expected skip for future-version artifact, got %v", err)
	}
}

// 集成：当前 dev 库（版本低于 007）不应因断言失败阻塞启动。
func TestVerifyMigrationArtifacts_NoFalsePositive(t *testing.T) {
	cfg := LoadConfig()
	db, err := InitDB(cfg)
	if err != nil {
		t.Skip("dev db not available")
		return
	}
	SetDB(db)
	if err := verifyMigrationArtifacts(db); err != nil {
		t.Fatalf("verifyMigrationArtifacts should not fail on current db: %v", err)
	}
}
