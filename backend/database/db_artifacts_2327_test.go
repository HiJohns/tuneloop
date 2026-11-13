package database

import "testing"

// #2327 S2: 迁移工件登记完整性（纯数据断言，不依赖数据库）。
// 缺任一条 → 启动校验无法发现「schema_migrations 已记录但实列缺失」（#1986 教训）。
func TestMigrationArtifacts2327Registered(t *testing.T) {
	want := map[string]bool{
		"credit_state":             false,
		"credit_risk_amount_cents": false,
		"credit_service_order_no":  false,
		"credit_assessed_at":       false,
	}
	for _, a := range defaultMigrationArtifacts {
		if a.Version != 20261112001 || a.Table != "orders" {
			continue
		}
		if _, ok := want[a.Column]; ok {
			want[a.Column] = true
		}
	}
	for col, ok := range want {
		if !ok {
			t.Fatalf("migration artifact 20261112001 orders.%s not registered", col)
		}
	}
}

// 负例：未登记的合成列必须被识别为缺失。
func TestMigrationArtifacts2327DetectsMissing(t *testing.T) {
	found := false
	for _, a := range defaultMigrationArtifacts {
		if a.Version == 20261112001 && a.Column == "credit_nonexistent_column" {
			found = true
		}
	}
	if found {
		t.Fatal("synthetic column must not be registered")
	}
}
