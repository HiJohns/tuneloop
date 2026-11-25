package services

// #2470 R1/R2：IAM 孤儿候选端点的客户端契约。
//
// R1：GET /api/v1/users/orphans?min_age_minute=…&page_size=…&page=…，解析 users/total/scanned_pending，
//     Unsupported=false。
// R2：旧 IAM（404/405）或协议缺 users 字段 → 回退 GET /users?status=pending，
//     Unsupported=true，日志含 fallback（D2：先发 tuneloop 不炸）。

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	orphanClientUserToken2470 = "user-token-2470"
	orphanClientUserID2470    = "pending-norel-2470"
)

type orphanClientRecorder2470 struct {
	mu      sync.Mutex
	queries []url.Values
	paths   []string
}

func (r *orphanClientRecorder2470) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, req.URL.Path)
	r.queries = append(r.queries, req.URL.Query())
}

func (r *orphanClientRecorder2470) snapshot() ([]string, []url.Values) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...), append([]url.Values(nil), r.queries...)
}

func newOrphanClientMock2470(t *testing.T, rec *orphanClientRecorder2470, orphanStatus int, orphanBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "svc-token-2470",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users/orphans", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer "+orphanClientUserToken2470, r.Header.Get("Authorization"), "必须用管理员用户 token 查询孤儿")
		w.Header().Set("Content-Type", "application/json")
		if orphanStatus != http.StatusOK {
			w.WriteHeader(orphanStatus)
			_, _ = w.Write([]byte(orphanBody))
			return
		}
		_, _ = w.Write([]byte(orphanBody))
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[{"id":"` + orphanClientUserID2470 + `","status":"pending","created_at":"2026-01-01T00:00:00Z"}],"page_size":100}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })
	return srv
}

func TestListOrphanCandidates_2470(t *testing.T) {
	rec := &orphanClientRecorder2470{}
	newOrphanClientMock2470(t, rec, http.StatusOK,
		`{"users":[{"id":"`+orphanClientUserID2470+`","status":"pending","username":"norel","created_at":"2026-01-01T00:00:00Z"}],"total":1,"scanned_pending":2,"page_size":100}`)

	client := NewIAMClient()
	result, err := client.ListOrphanCandidatesWithToken(orphanClientUserToken2470, 60)
	require.NoError(t, err)
	require.NotNil(t, result)

	paths, queries := rec.snapshot()
	require.Len(t, paths, 1, "R1：命中新端点时不得再调旧 pending 列表")
	assert.Equal(t, "/api/v1/users/orphans", paths[0])
	assert.Equal(t, "60", queries[0].Get("min_age_minute"))
	assert.Equal(t, "100", queries[0].Get("page_size"))
	assert.Equal(t, "1", queries[0].Get("page"))
	assert.False(t, result.Unsupported, "新端点可用时 Unsupported 必须为 false")
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 2, result.ScannedPending, "scanned 必须取 IAM 的全量 pending 数")
	require.Len(t, result.Users, 1)
	assert.Equal(t, orphanClientUserID2470, result.Users[0].ID)
	// 证据合同（R1）机器可判定键
	t.Logf("PATH=%s", paths[0])
	t.Logf("MIN_AGE=%s", queries[0].Get("min_age_minute"))
	t.Logf("UNSUPPORTED=0")
}

func TestListOrphanCandidatesFallback_2470(t *testing.T) {
	var logBuf bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldWriter) })

	rec := &orphanClientRecorder2470{}
	newOrphanClientMock2470(t, rec, http.StatusNotFound, `{"error":"not found"}`)

	client := NewIAMClient()
	result, err := client.ListOrphanCandidatesWithToken(orphanClientUserToken2470, 60)
	require.NoError(t, err)
	require.NotNil(t, result)

	paths, queries := rec.snapshot()
	require.Len(t, paths, 2, "R2：404 后必须回退到旧 pending 列表")
	assert.Equal(t, "/api/v1/users/orphans", paths[0])
	assert.Equal(t, "/api/v1/users", paths[1])
	assert.Equal(t, "pending", queries[1].Get("status"))
	assert.True(t, result.Unsupported, "回退路径必须标记 Unsupported=true")
	assert.Equal(t, len(result.Users), result.ScannedPending, "回退路径 scanned = len(pending)")
	assert.Contains(t, logBuf.String(), "fallback", "回退必须在日志可见（不静默）")
	// 证据合同（R2）机器可判定键
	t.Logf("UNSUPPORTED=1")
	t.Logf("FALLBACK=1")
}

func TestListOrphanCandidatesProtocolErrorFallsBack_2470(t *testing.T) {
	rec := &orphanClientRecorder2470{}
	// 200 但缺 users 字段 → 协议错误，必须回退而不是当成空列表。
	newOrphanClientMock2470(t, rec, http.StatusOK, `{"total":0,"scanned_pending":0}`)

	client := NewIAMClient()
	result, err := client.ListOrphanCandidatesWithToken(orphanClientUserToken2470, 60)
	require.NoError(t, err)
	require.NotNil(t, result)
	paths, _ := rec.snapshot()
	require.Len(t, paths, 2, "缺 users 字段必须回退旧路径")
	assert.Equal(t, "/api/v1/users", paths[1])
	assert.True(t, result.Unsupported)
	assert.Len(t, result.Users, 1, "回退后应返回旧列表内容")
}
