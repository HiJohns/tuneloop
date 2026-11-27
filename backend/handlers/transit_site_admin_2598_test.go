package handlers

// #2598 R2/R3：中转建组织失败的「持久 vs 瞬时」分流。
//
// 缺陷（项 1）：`CreateAdminTransitSite` 把 CreateOrganization 的**非冲突**失败
// 一律映射为 502/50200 +「中转网点组织创建失败，请稍后再试」，其中包含上游
// 400（父组织非法/参数契约不符）、403（权限/角色模板缺失）、404（命名空间/资源
// 不存在）这些**持久**失败——重试一万次同样失败，平台管理员必须改数据/权限。
//
// 期望（Decision Lock D4）：
//   - 上游 400/403/404（UserActionAsConfig=true，含 400/422）→ 500 / 50015 +
//     「身份服务配置异常，请联系平台管理员处理」，响应**不含**「请稍后再试」；
//   - 上游 500 / IAM 不可达（传输层）→ 502 / 50200 + 「请稍后再试」（文案不变）；
//   - 失败时不得建出任何站点行。
//
// Token lines are printed with fmt.Printf so scripts/verify-2598.sh can grep them
// regardless of go test -v indentation.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// transitTriple2598 从响应体抽取 (code, message)，避免断言里反复断言类型。
func transitTriple2598(t *testing.T, resp map[string]interface{}) (int, string) {
	t.Helper()
	code := 0
	if raw, ok := resp["code"].(float64); ok {
		code = int(raw)
	}
	msg := ""
	if raw, ok := resp["message"].(string); ok {
		msg = raw
	}
	return code, msg
}

// TestTransitSiteCreate2598_PersistentFailure：上游 400 / 403 → 500/50015（不可重试）。
func TestTransitSiteCreate2598_PersistentFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs("2598aaaa0001")
	testfixtures.SeedTenantFK(t, db, tenantID)

	rejectedStatus := map[string]int{}
	rejectedCode := map[string]int{}
	actionable := 1
	noRetry := 0
	var sitesCreated int64

	for _, mode := range []string{"rejected400", "forbidden403"} {
		rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: mode, primaries: 1}
		srv := installTransitIAM(t, rec)
		router := transitActorRouter2552()

		name := "持久失败中转站-" + mode
		httpStatus, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
			map[string]interface{}{"name": name, "address": "x", "phone": "1"})
		code, message := transitTriple2598(t, resp)
		rejectedStatus[mode] = httpStatus
		rejectedCode[mode] = code

		if !strings.Contains(message, "联系平台管理员") {
			actionable = 0
			t.Errorf("%s: 持久失败文案必须可行动（含「联系平台管理员」），got %q", mode, message)
		}
		if strings.Contains(message, "请稍后再试") {
			noRetry = 1
			t.Errorf("%s: 持久失败文案不得出现「请稍后再试」，got %q", mode, message)
		}

		_, _, creates := rec.snapshot()
		require.Len(t, creates, 1, "%s: 必须发起一次建组织请求", mode)

		var count int64
		require.NoError(t, db.Model(&models.Site{}).Where("name = ?", name).Count(&count).Error)
		sitesCreated += count
		require.Zero(t, count, "%s: 建组织失败不得建出站点", mode)
		srv.Close()
	}

	// 400 与 403 必须同分流同码（同一「持久配置」类别）。
	require.Equal(t, rejectedStatus["rejected400"], rejectedStatus["forbidden403"],
		"上游 400 与 403 必须同状态: 400=%d 403=%d", rejectedStatus["rejected400"], rejectedStatus["forbidden403"])
	require.Equal(t, rejectedCode["rejected400"], rejectedCode["forbidden403"],
		"上游 400 与 403 必须同码: 400=%d 403=%d", rejectedCode["rejected400"], rejectedCode["forbidden403"])

	fmt.Printf("TRANSIT_REJECTED_STATUS=%d\n", rejectedStatus["rejected400"])
	fmt.Printf("TRANSIT_REJECTED_CODE=%d\n", rejectedCode["rejected400"])
	fmt.Printf("TRANSIT_REJECTED_MSG_ACTIONABLE=%d\n", actionable)
	fmt.Printf("TRANSIT_REJECTED_MSG_NO_RETRY=%d\n", noRetry)
	fmt.Printf("TRANSIT_REJECTED_SITES_CREATED=%d\n", sitesCreated)

	require.Equal(t, http.StatusInternalServerError, rejectedStatus["rejected400"], "持久失败必须 500（非 502）")
	require.Equal(t, 50015, rejectedCode["rejected400"], "持久失败必须 50015（非 50200）")
	require.Equal(t, 1, actionable)
	require.Equal(t, 0, noRetry)
	require.Zero(t, sitesCreated)
}

// dropCreateIAM2598 让**建组织**这一跳在传输层失败（连接被对端直接关闭），
// 而命名空间/主组织解析（GET）仍然成功。
//
// 为什么不是「IAM URL 指向已关闭端口」：那样连 ResolveNamespacePrimaryOrg 也会失败，
// handler 会在更早的、本单 Out of Scope（D10）的分支返回 500/50000「无法解析命名空间
// 主组织」——根本到不了 CreateOrganization 的失败分类。本夹具保持同一失败类别
// （传输层错误、无上游状态），只把注入点收窄到被修改的那一跳。
func dropCreateIAM2598(t *testing.T, rec *transitIAMRecorder) *httptest.Server {
	t.Helper()
	inner := rec.handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organizations") {
			if hijacker, ok := w.(http.Hijacker); ok {
				if conn, _, err := hijacker.Hijack(); err == nil {
					_ = conn.Close()
					return
				}
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return srv
}

// TestTransitSiteCreate2598_TransientFailure：上游 500 / 建组织传输层失败 → 502/50200（可重试）。
func TestTransitSiteCreate2598_TransientFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID, _, _ := testfixtures.NewTenantIDs("2598aaaa0002")
	testfixtures.SeedTenantFK(t, db, tenantID)

	// 1) 上游 500（有上游状态，>= 500 → 瞬时）。
	rec := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "error", primaries: 1}
	srv := installTransitIAM(t, rec)
	router := transitActorRouter2552()
	status500, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
		map[string]interface{}{"name": "瞬时失败中转站-upstream500", "address": "x", "phone": "1"})
	code500, msg500 := transitTriple2598(t, resp)
	srv.Close()

	// 2) 建组织传输层失败（连接被关闭 → 无上游状态）。
	rec2 := &transitIAMRecorder{primaryID: uuid.New().String(), createMode: "ok", primaries: 1}
	dropCreateIAM2598(t, rec2)
	statusTransport, resp := doTransitJSON2552(t, router, http.MethodPost, "/api/admin/transit-sites", tenantID,
		map[string]interface{}{"name": "瞬时失败中转站-transport", "address": "x", "phone": "1"})
	codeTransport, msgTransport := transitTriple2598(t, resp)

	retryHint := 1
	notConfig := 1
	for name, msg := range map[string]string{"upstream500": msg500, "transport": msgTransport} {
		if !strings.Contains(msg, "请稍后再试") {
			retryHint = 0
			t.Errorf("%s: 瞬时失败文案必须含「请稍后再试」，got %q", name, msg)
		}
		if msg == iamConfigErrorMessage || strings.Contains(msg, "联系平台管理员") {
			notConfig = 0
			t.Errorf("%s: 瞬时失败不得提示配置问题，got %q", name, msg)
		}
		var sites int64
		require.NoError(t, db.Model(&models.Site{}).
			Where("name = ?", "瞬时失败中转站-"+name).Count(&sites).Error)
		require.Zero(t, sites, "%s: 失败时不得建出站点", name)
	}

	fmt.Printf("TRANSIT_TRANSIENT_STATUS=%d\n", status500)
	fmt.Printf("TRANSIT_TRANSIENT_CODE=%d\n", code500)
	fmt.Printf("TRANSIT_TRANSIENT_MSG_RETRY=%d\n", retryHint)
	fmt.Printf("TRANSIT_TRANSIENT_NOT_CONFIG=%d\n", notConfig)

	require.Equal(t, http.StatusBadGateway, status500, "上游 5xx 必须 502")
	require.Equal(t, http.StatusBadGateway, statusTransport, "建组织传输层失败必须 502")
	require.Equal(t, 50200, code500)
	require.Equal(t, 50200, codeTransport)
	require.Equal(t, 1, retryHint)
	require.Equal(t, 1, notConfig)
}
