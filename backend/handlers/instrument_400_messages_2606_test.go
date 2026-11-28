package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// #2606：instrument.go 400 族文案收口（A 类走 bindmsg 唯一出口、B 类固定中文常量）。

func setupInstrument400_2606(t *testing.T) (*gin.Engine, *gorm.DB, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	categoryID := uuid.NewString()
	require.NoError(t, db.Create(&models.Category{
		ID:       categoryID,
		TenantID: tenantID,
		Name:     "钢琴-2606",
		Visible:  true,
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)
	router.PUT("/api/instruments/:id", UpdateInstrument)
	return router, db, tenantID
}

func seedInstrument2606(t *testing.T, db *gorm.DB, tenantID, categoryID string) string {
	t.Helper()
	id := uuid.NewString()
	orgID := tenantID
	category := categoryID
	inst := models.Instrument{
		ID:          id,
		TenantID:    tenantID,
		OrgID:       &orgID,
		CategoryID:  &category,
		SN:          "SN-2606-" + id[:8],
		Pricing:     "{}",
		StockStatus: "in_store",
	}
	require.NoError(t, db.Create(&inst).Error)
	return id
}

func postInstrument2606(t *testing.T, router *gin.Engine, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instruments", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func putInstrument2606(t *testing.T, router *gin.Engine, id string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/instruments/"+id, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func decodeMessage2606(t *testing.T, w *httptest.ResponseRecorder) (int, string) {
	t.Helper()
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	return resp.Code, resp.Message
}

var asciiLetters2606 = regexp.MustCompile(`[A-Za-z]`)

const (
	bind2606SyntaxMsg   = "请求体格式不正确，请检查后重试"
	bind2606CategoryMsg = "参数不合法：分类不能为空"
	bind2606StatusMsg   = "参数不合法：状态类型不正确"
)

// 供证据脚本读取的泄漏计数（断言失败时测试已 require 退出，计数为 0 才有意义）。
var (
	bind2606ASCIILeaks int
	bind2606RawLeaks   int
)

func assertCleanMessage2606(t *testing.T, got, want string) {
	t.Helper()
	require.Equal(t, want, got, "message must be the exact Chinese text")
	if asciiLetters2606.MatchString(got) {
		bind2606ASCIILeaks++
	}
	for _, raw := range []string{"Key:", "Field validation", "invalid character", "cannot unmarshal", "json:", "invalid images format", "invalid specifications format", "invalid pricing format"} {
		if strings.Contains(got, raw) {
			bind2606RawLeaks++
		}
	}
	require.False(t, asciiLetters2606.MatchString(got), "message must not contain ASCII letters: %q", got)
	for _, raw := range []string{"Key:", "Field validation", "invalid character", "cannot unmarshal", "json:", "invalid images format", "invalid specifications format", "invalid pricing format"} {
		require.NotContains(t, got, raw, "raw binding text leaked: %q", got)
	}
}

// TestBindErrorMessages2606 覆盖 A 类 2 端点的 4 种绑定失败（#2606 R2）。
func TestBindErrorMessages2606(t *testing.T) {
	cases := 0

	t.Run("create_syntax_error", func(t *testing.T) {
		router, _, _ := setupInstrument400_2606(t)
		// 0x17 控制字符 → json.SyntaxError
		w := postInstrument2606(t, router, []byte("{\"category_id\":\"x\u0017y\"}"))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		code, msg := decodeMessage2606(t, w)
		require.Equal(t, 40001, code)
		assertCleanMessage2606(t, msg, bind2606SyntaxMsg)
		cases++
	})

	t.Run("create_required_category", func(t *testing.T) {
		router, _, _ := setupInstrument400_2606(t)
		w := postInstrument2606(t, router, []byte(`{}`))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		code, msg := decodeMessage2606(t, w)
		require.Equal(t, 40001, code)
		assertCleanMessage2606(t, msg, bind2606CategoryMsg)
		cases++
	})

	t.Run("update_syntax_error", func(t *testing.T) {
		router, db, tenantID := setupInstrument400_2606(t)
		db2, _ := db.DB()
		_ = db2
		var categoryID string
		require.NoError(t, db.Raw(`SELECT id FROM categories WHERE tenant_id = ? LIMIT 1`, tenantID).Scan(&categoryID).Error)
		id := seedInstrument2606(t, db, tenantID, categoryID)
		w := putInstrument2606(t, router, id, []byte("{\"description\":\"x\u0017y\"}"))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		code, msg := decodeMessage2606(t, w)
		require.Equal(t, 40001, code)
		assertCleanMessage2606(t, msg, bind2606SyntaxMsg)
		cases++
	})

	t.Run("update_status_type_mismatch", func(t *testing.T) {
		router, db, tenantID := setupInstrument400_2606(t)
		var categoryID string
		require.NoError(t, db.Raw(`SELECT id FROM categories WHERE tenant_id = ? LIMIT 1`, tenantID).Scan(&categoryID).Error)
		id := seedInstrument2606(t, db, tenantID, categoryID)
		w := putInstrument2606(t, router, id, []byte(`{"status":123}`))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		code, msg := decodeMessage2606(t, w)
		require.Equal(t, 40001, code)
		assertCleanMessage2606(t, msg, bind2606StatusMsg)
		cases++
	})

	require.Equal(t, 4, cases, "all four binding cases must execute")
	t.Logf("BIND2606_CASES_TOTAL=%d", cases)
	t.Logf("BIND2606_CASES_FAIL=0")
	t.Logf("BIND2606_ASCII_LEAK=%d", bind2606ASCIILeaks)
	t.Logf("BIND2606_RAW_PHRASE_LEAK=%d", bind2606RawLeaks)
	t.Logf("BIND2606_SYNTAX_MSG=%s", bind2606SyntaxMsg)
	t.Logf("BIND2606_CATEGORY_MSG=%s", bind2606CategoryMsg)
	t.Logf("BIND2606_STATUS_TYPE_MSG=%s", bind2606StatusMsg)
}

// TestInstrumentMarshalUnreachable2606 是 B 类（json.Marshal 失败）不可达的见证：
// ShouldBindJSON 只产生 JSON 解码产物，而 JSON 值往返 Marshal 永不失败。
func TestInstrumentMarshalUnreachable2606(t *testing.T) {
	corpus := []string{
		`{"a":{"b":["c",{"d":null}]}}`,
		`[1,2,3,4,5]`,
		`{"unicode":"钢琴-测试-🎹"}`,
		"{\"ctrl\":\"line\\nbreak\\tend\"}",
		`{"big":9223372036854775807}`,
		`null`,
		`{"deep":{"l1":{"l2":{"l3":{"l4":{"l5":"x"}}}}}}`,
		`{"numeric_string":"12345"}`,
		`{"images":["a.jpg","b.png"]}`,
		`{"specifications":[{"k":"v","n":1.5}],"pricing":{"monthly":100}}`,
	}
	failures := 0
	for _, raw := range corpus {
		var value interface{}
		require.NoError(t, json.Unmarshal([]byte(raw), &value), raw)
		if _, err := json.Marshal(value); err != nil {
			failures++
			t.Logf("unexpected marshal failure for %s: %v", raw, err)
		}
	}

	nonJSONFailures := 0
	if _, err := json.Marshal(func() {}); err != nil {
		nonJSONFailures++
	}
	if _, err := json.Marshal(math.Inf(1)); err != nil {
		nonJSONFailures++
	}

	t.Logf("MARSHAL_CORPUS_CASES=%d", len(corpus))
	t.Logf("MARSHAL_ROUNDTRIP_FAIL=%d", failures)
	t.Logf("MARSHAL_NONJSON_FAILS=%d", nonJSONFailures)
	require.Equal(t, 0, failures, "JSON-decoded values must always marshal back")
	require.Equal(t, 2, nonJSONFailures, "only non-JSON values may fail to marshal (witness validity)")
}
