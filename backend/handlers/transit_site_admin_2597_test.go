package handlers

// #2597：中转网点创建失败的**提示层**分流。
//
// 缺陷：`CreateAdminTransitSite` 把 `ResolveNamespacePrimaryOrg` 的全部错误压平成
// `500/50000` +「无法解析命名空间主组织，请稍后再试」——但其中「命名空间存在 0 个
// 或 ≥2 个 is_primary 组织」是**持久配置不变量违反**，重试永不成功，必须由平台
// 管理员修数据。用户拿到「请稍后再试」既不能定位也不能行动。
//
// 本文件锁定两条互斥路径（R1 配置类 / R2 瞬时类）与 R3 的停用文案（不得外溢内部
// Issue 号）。Token 用 fmt.Printf 输出，供 scripts/verify-2597.sh grep。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// transitIAM2597 最小 fake IAM：只覆盖建站路径需要的端点。
//   - primaries  ∈ {0,1,2}：`GET /api/v1/organizations` 返回几条 is_primary
//   - listMode   ∈ {ok,error}：组织列表可用性（error = 瞬时故障）
type transitIAM2597 struct {
	mu        sync.Mutex
	primaries int
	listMode  string
	primaryID string
	creates   int
}

func (f *transitIAM2597) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/organizations", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		mode, primaries, primaryID := f.listMode, f.primaries, f.primaryID
		f.mu.Unlock()
		if mode == "error" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"iam list organizations boom"}`))
			return
		}
		orgs := make([]map[string]interface{}, 0, 2)
		for i := 0; i < primaries; i++ {
			orgs = append(orgs, map[string]interface{}{
				"id":         primaryID,
				"name":       "平台根组织",
				"is_primary": true,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"organizations": orgs})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/organizations"):
			f.mu.Lock()
			f.creates++
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"org_id": uuid.New().String(), "admin_id": uuid.New().String(),
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.New().String()})
		}
	})
	return mux
}

func (f *transitIAM2597) createCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates
}

func installTransitIAM2597(t *testing.T, rec *transitIAM2597) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(rec.handler())
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return srv
}

// transitActorRouter2597 复用 transitActorRouter2552 的 actor 注入（请求头带租户），
// 并补上停用端点（2552 router 未注册 DELETE）。
func transitActorRouter2597() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tid := c.GetHeader("X-Test-Tenant")
		actor := testutil.TestActor{
			TenantID: tid, OrgID: tid, UserID: uuid.New().String(), Role: "admin",
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/api/admin/transit-sites", CreateAdminTransitSite)
	r.DELETE("/api/admin/transit-sites/:id", DeleteAdminTransitSite)
	return r
}

func transitSetup2597(t *testing.T, suffix string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs(suffix)
	testfixtures.SeedTenantFK(t, db, tenantID)
	return tenantID
}

// R1：配置不变量违反（0 个 / 2 个主组织）→ 500/50014 + 可行动文案，不建组织/不建站，
// 计数与组织 id 只进日志。
func TestTransitSiteCreate2597_PrimaryConfigInvariant(t *testing.T) {
	tenantID := transitSetup2597(t, "2597aa0c000a")

	for _, primaries := range []int{0, 2} {
		rec := &transitIAM2597{primaryID: uuid.New().String(), listMode: "ok", primaries: primaries}
		srv := installTransitIAM2597(t, rec)
		router := transitActorRouter2597()

		var logBuf bytes.Buffer
		prevLogWriter := log.Writer()
		log.SetOutput(&logBuf)
		status, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
			map[string]interface{}{
				"name": "主组织异常站", "address": "x", "phone": "1",
			})
		log.SetOutput(prevLogWriter)

		code, _ := resp["code"].(float64)
		msg, _ := resp["message"].(string)
		var sites int64
		require.NoError(t, database.GetDB().Model(&models.Site{}).
			Where("name = ?", "主组织异常站").Count(&sites).Error)

		codeOK := status == http.StatusInternalServerError && int(code) == 50014
		actionable := strings.Contains(msg, "联系平台管理员")
		noRetryHint := strings.Contains(msg, "请稍后再试")
		asciiLeak := strings.Contains(msg, "namespace has") || strings.Contains(msg, "expected exactly 1")
		orgIDLeak := primaries > 0 && strings.Contains(msg, rec.primaryID)
		logDetail := strings.Contains(logBuf.String(), "count=") && strings.Contains(logBuf.String(), "org_ids=")

		configCode := 0
		if codeOK && actionable && !noRetryHint && !asciiLeak && !orgIDLeak {
			configCode = int(code)
		}
		fmt.Printf("PRIMARY_CONFIG_CODE=%d\n", configCode)
		fmt.Printf("PRIMARY_CONFIG_MSG_HAS_ACTIONABLE=%d\n", boolToken2597(actionable))
		fmt.Printf("PRIMARY_CONFIG_MSG_NO_RETRY=%d\n", boolToken2597(noRetryHint))
		fmt.Printf("PRIMARY_CONFIG_ORGS_CREATED=%d\n", rec.createCount())
		fmt.Printf("PRIMARY_CONFIG_SITES_CREATED=%d\n", sites)
		fmt.Printf("PRIMARY_CONFIG_LOG_DETAIL=%d\n", boolToken2597(logDetail))
		fmt.Printf("PRIMARY_CONFIG_CASE primaries=%d status=%d\n", primaries, status)

		require.True(t, codeOK, "配置类失败必须 500/50014（primaries=%d）: %v", primaries, resp)
		require.True(t, actionable, "文案必须可行动（含「联系平台管理员」）: %q", msg)
		require.False(t, noRetryHint, "配置类文案不得出现「请稍后再试」（会误导重试）: %q", msg)
		require.False(t, asciiLeak, "后端英文内部诊断不得进响应: %q", msg)
		require.False(t, orgIDLeak, "组织 id 不得进响应: %q", msg)
		require.Equal(t, 0, rec.createCount(), "配置类失败不得建组织")
		require.Equal(t, int64(0), sites, "配置类失败不得建站")
		require.True(t, logDetail, "计数/组织 id 必须进日志（count=/org_ids=）: %q", logBuf.String())
		srv.Close()
	}
}

// R2：瞬时故障（IAM 组织列表不可用）→ 500/50000 +「稍后再试」，不得误判为配置类。
func TestTransitSiteCreate2597_PrimaryTransient(t *testing.T) {
	tenantID := transitSetup2597(t, "2597bb0c000b")

	rec := &transitIAM2597{primaryID: uuid.New().String(), listMode: "error", primaries: 1}
	installTransitIAM2597(t, rec)
	router := transitActorRouter2597()

	status, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
		map[string]interface{}{"name": "瞬时故障站", "address": "x", "phone": "1"})

	code, _ := resp["code"].(float64)
	msg, _ := resp["message"].(string)
	var sites int64
	require.NoError(t, database.GetDB().Model(&models.Site{}).
		Where("name = ?", "瞬时故障站").Count(&sites).Error)

	statusOK := status == http.StatusInternalServerError
	retryHint := strings.Contains(msg, "稍后再试")
	notConfigCode := int(code) != 50014

	countedCode := 0
	if statusOK && retryHint && notConfigCode {
		countedCode = int(code)
	}
	fmt.Printf("TRANSIENT_STATUS=%d\n", status)
	fmt.Printf("TRANSIENT_CODE=%d\n", countedCode)
	fmt.Printf("TRANSIENT_MSG_RETRY=%d\n", boolToken2597(retryHint))
	fmt.Printf("TRANSIENT_NOT_CONFIG_CODE=%d\n", boolToken2597(notConfigCode))
	fmt.Printf("TRANSIENT_SITES_CREATED=%d\n", sites)

	require.True(t, statusOK, "瞬时故障必须 500: %v", resp)
	require.Equal(t, 50000, countedCode, "瞬时故障必须保持 50000: %v", resp)
	require.True(t, retryHint, "瞬时故障文案必须含「稍后再试」: %q", msg)
	require.True(t, notConfigCode, "瞬时故障不得返回配置码 50014: %v", resp)
	require.Equal(t, int64(0), sites, "瞬时故障不得建站")
	require.Equal(t, 0, rec.createCount(), "瞬时故障不得建组织")
}

// R3：停用被拒文案去掉内部 Issue 号（接入统一错误层后会真实到达用户）。
func TestTransitSiteDeactivate2597_MessageNoInternalRef(t *testing.T) {
	tenantID := transitSetup2597(t, "2597cc0c000c")
	router := transitActorRouter2597()

	site := models.Site{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    uuid.New().String(),
		Name:     "上海中转站",
		Type:     "transit",
		Status:   "active",
	}
	require.NoError(t, database.GetDB().Create(&site).Error)
	route := models.TransitRoute{
		ID: uuid.New().String(), ControlledSiteID: uuid.New().String(),
		TransitSiteID: site.ID, CreatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(&route).Error)

	status, resp := doTransitJSON2552(t, router, http.MethodDelete,
		"/api/admin/transit-sites/"+site.ID, tenantID, nil)
	msg, _ := resp["message"].(string)

	noInternalRef := !strings.Contains(msg, "#1935")
	keepsKeyword := strings.Contains(msg, "路由引用")
	token := 0
	if status == http.StatusBadRequest && noInternalRef && keepsKeyword {
		token = 1
	}
	fmt.Printf("DEACTIVATE_MSG_NO_INTERNAL_REF=%d\n", token)
	fmt.Printf("DEACTIVATE_MSG=%s\n", msg)

	require.Equal(t, http.StatusBadRequest, status, "被路由引用时停用必须 400: %v", resp)
	require.True(t, keepsKeyword, "文案必须保留可行动关键词「路由引用」: %q", msg)
	require.True(t, noInternalRef, "文案不得外溢内部 Issue 号 #1935: %q", msg)
}

func boolToken2597(v bool) int {
	if v {
		return 1
	}
	return 0
}
