package handlers

// #2284：测试数据库连接池单例 + 连接上限，避免连接耗尽导致隐式 SKIP。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedTestDB_Singleton_2284(t *testing.T) {
	db1 := testfixtures.GetTestDB(t)
	db2 := testfixtures.GetTestDB(t)
	require.NotNil(t, db1)
	sqlDB1, err := db1.DB()
	require.NoError(t, err)
	sqlDB2, err := db2.DB()
	require.NoError(t, err)
	assert.Same(t, sqlDB1, sqlDB2, "GetTestDB 必须复用同一 sql.DB 连接池")
	assert.LessOrEqual(t, sqlDB1.Stats().OpenConnections, 8, "连接数不得超过池上限")
}

// TestNoDatabaseUnavailableSkip_2284 防止「连不上测试库就 SKIP」的假通过回归。
func TestNoDatabaseUnavailableSkip_2284(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	require.NoError(t, err)
	for _, f := range files {
		if f == "shared_test_db_2284_test.go" {
			continue
		}
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "test database not available",
			"%s 不得在测试库不可用时 SKIP（应 Fatal，避免假通过）", f)
	}
}

func TestResolveTestDBName_2288(t *testing.T) {
	t.Setenv("TUNELOOP_TEST_DB", "")
	t.Setenv("OPENCODE_NODE_ID", "")
	n, err := resolveTestDBName()
	require.NoError(t, err)
	assert.Equal(t, "tuneloop_test", n, "无覆盖时保持默认库名")

	t.Setenv("OPENCODE_NODE_ID", "node-2")
	n, err = resolveTestDBName()
	require.NoError(t, err)
	assert.Regexp(t, `^tuneloop_test_node_2_[a-f0-9]{8}$`, n, "节点库名带短 hash")

	t.Setenv("OPENCODE_NODE_ID", "nodeA")
	a, err := resolveTestDBName()
	require.NoError(t, err)
	t.Setenv("OPENCODE_NODE_ID", "nodea")
	b, err := resolveTestDBName()
	require.NoError(t, err)
	assert.NotEqual(t, a, b, "大小写不同的节点 ID 必须映射到不同库")

	t.Setenv("TUNELOOP_TEST_DB", "tuneloop_test_custom")
	n, err = resolveTestDBName()
	require.NoError(t, err)
	assert.Equal(t, "tuneloop_test_custom", n, "显式测试库覆盖保留")

	for _, bad := range []string{"tuneloop", "beaconiam", "postgres", "tuneloop_test_", "prod_tuneloop_test", strings.Repeat("a", 70)} {
		t.Setenv("TUNELOOP_TEST_DB", bad)
		_, err := resolveTestDBName()
		assert.Error(t, err, "非白名单库名必须拒绝: %q", bad)
	}
}
