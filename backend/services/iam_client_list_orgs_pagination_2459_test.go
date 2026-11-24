package services

// #2459 审计阻断项 A 建议 3：唯一性预检必须看到**全部**同 namespace 组织。
// ListOrganizations 之前固定 page_size=1000 且不翻页 —— namespace 组织数 >1000 时
// 预检不完整。本用例验证分页循环：page=1..N，直到累计数达到 IAM 返回的 total。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListOrganizationsPagesUntilTotal_2459(t *testing.T) {
	const (
		nsID      = "ns-pagination-2459"
		svcToken  = "svc-token-pagination-2459"
		totalOrgs = iamOrgListPageSize + 2 // 首页满页 + 短页
	)

	var mu sync.Mutex
	var pages []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": svcToken, "expires_in": 3600, "token_type": "Bearer",
			})
		case len(r.URL.Path) >= len("/api/v1/namespaces/") && r.URL.Path[:len("/api/v1/namespaces/")] == "/api/v1/namespaces/":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": nsID})
		case r.URL.Path == "/api/v1/organizations":
			page := r.URL.Query().Get("page")
			pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
			mu.Lock()
			pages = append(pages, page)
			mu.Unlock()
			if pageSize <= 0 {
				pageSize = iamOrgListPageSize
			}
			pageNum, _ := strconv.Atoi(page)
			if pageNum == 0 {
				pageNum = 1
			}
			start := (pageNum - 1) * pageSize
			orgs := make([]map[string]string, 0, pageSize)
			for i := start; i < totalOrgs && i < start+pageSize; i++ {
				orgs = append(orgs, map[string]string{
					"id":   fmt.Sprintf("org-%04d", i),
					"name": fmt.Sprintf("组织-%04d", i),
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"organizations": orgs, "total": totalOrgs, "page": pageNum, "page_size": pageSize,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"unexpected path"}`))
		}
	}))
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })

	client := NewIAMClientWithCredentials(nsID, "secret")
	orgs, err := client.ListOrganizations()
	require.NoError(t, err)

	mu.Lock()
	gotPages := append([]string(nil), pages...)
	mu.Unlock()

	require.Len(t, orgs, totalOrgs, "必须翻页取全，而不是只返回首页")
	require.Equal(t, []string{"1", "2"}, gotPages, "必须从 page=1 顺序翻页直到短页")
	require.Equal(t, "org-0000", orgs[0].ID)
	require.Equal(t, fmt.Sprintf("org-%04d", totalOrgs-1), orgs[len(orgs)-1].ID)

	t.Logf("PAGES=%d", len(gotPages))
	t.Logf("TOTAL_ORGS=%d", len(orgs))
}
