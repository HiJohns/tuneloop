package database

// #1929: 包过期防御单测（maxLocalMigrationVersion / checkPackageFreshness）。

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxLocalMigrationVersion(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"031_add_photo_tables.up.sql",         // 3 位旧格式
		"031_add_photo_tables.down.sql",       // down 一并覆盖
		"20260914001_add_second_doc.up.sql",   // 9 位新格式
		"20260914001_add_second_doc.down.sql", //
		"20260916001_add_transit_fees.up.sql", // 最大值
		"README.md",                           // 忽略
		"not_a_migration.sql",                 // 忽略
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("--"), 0o644))
	}
	max, err := maxLocalMigrationVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, uint(20260916001), max)
}

func TestMaxLocalMigrationVersion_EmptyDir(t *testing.T) {
	max, err := maxLocalMigrationVersion(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, uint(0), max)
}

func TestMaxLocalMigrationVersion_MissingDir(t *testing.T) {
	_, err := maxLocalMigrationVersion(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.Error(t, err)
}

func TestCheckPackageFreshness(t *testing.T) {
	// DB 超前于包 → 报「包过期」并含两版本号
	err := checkPackageFreshness(20260917001, 20260914001)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AHEAD")
	assert.Contains(t, err.Error(), "20260917001")
	assert.Contains(t, err.Error(), "20260914001")

	// 相等 / 落后 / 空库 → 通过
	assert.NoError(t, checkPackageFreshness(20260914001, 20260914001))
	assert.NoError(t, checkPackageFreshness(20260914001, 20260917001))
	assert.NoError(t, checkPackageFreshness(0, 20260917001))
}
