package handlers

// #2451：公共乐器列表 N+1 修复的回归测试。
//
// 锁定四件事：
//  1. 每页查询数与 pageSize 解耦（q10 == q100 == 6，富化 4）；
//  2. 响应 21 键契约与派生值（site_address 受控中转覆盖、cover_image == thumbnail）；
//  3. 缺省值行为（无 site / 无 tenant 行 → "-" / 空，不 panic、不新增查询）；
//  4. 批量商户 helper 与单条 helper 逐字段等价（含「每 tenant 取 id 最小」语义）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// countingLogger 拦截 GORM 每条 SQL，用于「请求前后快照差值」式查询计数。
type countingLogger struct {
	gormlogger.Interface
	mu    sync.Mutex
	count int
}

func (l *countingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.mu.Lock()
	l.count++
	l.mu.Unlock()
}

// Count 返回已拦截的 SQL 条数（线程安全）。
func (l *countingLogger) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.count
}

// newCountedDB 把全局 DB 换成带计数 logger 的会话（测试结束自动还原）。
func newCountedDB(t *testing.T) (*gorm.DB, *countingLogger) {
	t.Helper()
	base := testfixtures.SetupTestDB(t)
	cl := &countingLogger{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	db := base.Session(&gorm.Session{Logger: cl})
	testutil.SetDB(t, db)
	return db, cl
}

// n1JSON 是响应体的最小解析结构：item 保留原始 map 以便断言键集合。
type n1JSON struct {
	Code int `json:"code"`
	Data struct {
		List     []map[string]interface{} `json:"list"`
		Total    int64                    `json:"total"`
		Page     int                      `json:"page"`
		PageSize int                      `json:"pageSize"`
	} `json:"data"`
}

func n1Router() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/public/instruments", GetPublicInstruments)
	return router
}

func n1Get(t *testing.T, router *gin.Engine, url string) (n1JSON, []byte) {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp n1JSON
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	require.Equal(t, 20000, resp.Code, w.Body.String())
	return resp, w.Body.Bytes()
}

func n1Queries(t *testing.T, router *gin.Engine, cl *countingLogger, url string) int {
	t.Helper()
	before := cl.Count()
	n1Get(t, router, url)
	return cl.Count() - before
}

func seedN1Tenant(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	id := uuid.New().String()
	require.NoError(t, db.Create(&models.Tenant{ID: id, Name: name, Status: "active"}).Error)
	return id
}

func seedN1Site(t *testing.T, db *gorm.DB, tenantID, name, address, phone string) string {
	t.Helper()
	id := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID:       id,
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     name,
		Address:  address,
		Phone:    phone,
		Type:     "normal",
		Status:   "active",
	}).Error)
	return id
}

func seedN1Merchant(t *testing.T, db *gorm.DB, merchantID, tenantID, merchantType, transitAddress string) {
	t.Helper()
	require.NoError(t, db.Omit("AdminUID").Create(&models.Merchant{
		ID:                 merchantID,
		TenantID:           tenantID,
		OrgID:              tenantID,
		Name:               "merchant-" + merchantID[len(merchantID)-4:],
		Code:               "code-" + merchantID[len(merchantID)-8:],
		Status:             "active",
		MerchantType:       merchantType,
		TransitAddress:     transitAddress,
		TransitPhone:       "13800000000",
		TransitContactName: "Transit Contact",
	}).Error)
}

// seedN1Instruments 批量建乐器；createdAt 逐件递增保证 ORDER BY created_at DESC 稳定，
// 分页断言（无重复/无遗漏）才可重放。
func seedN1Instruments(t *testing.T, db *gorm.DB, tenantID string, siteID *string, count int, start time.Time, snPrefix string) []string {
	t.Helper()
	rate := 100.0
	cents := models.ToCentsPtr(&rate)
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		inst := models.Instrument{
			ID:             uuid.New().String(),
			TenantID:       tenantID,
			SN:             fmt.Sprintf("%s-%03d", snPrefix, i),
			Level:          "entry",
			StockStatus:    models.StockStatusAvailable,
			Images:         "[]",
			Pricing:        `{"daily_rent":100.0}`,
			Specifications: "{}",
			Properties:     "{}",
			BaseDailyRate:  cents,
			CreatedAt:      start.Add(time.Duration(i) * time.Second),
			UpdatedAt:      start.Add(time.Duration(i) * time.Second),
		}
		if siteID != nil {
			parsed := uuid.MustParse(*siteID)
			inst.SiteID = &parsed
		}
		require.NoError(t, db.Create(&inst).Error)
		ids = append(ids, inst.ID)
	}
	return ids
}

func seedN1DisplayMedia(t *testing.T, db *gorm.DB, tenantID, instrumentID, storageKey string) {
	t.Helper()
	require.NoError(t, db.Create(&models.InstrumentMedia{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        tenantID,
		InstrumentID: &instrumentID,
		ObjectType:   "instrument",
		BatchID:      uuid.New().String(),
		BatchType:    "create",
		FileName:     storageKey + ".jpg",
		FileType:     "image",
		StorageKey:   storageKey,
		IsDisplay:    true,
		SortOrder:    0,
	}).Error)
}

// R1：每页总查询 ≤6（COUNT + Find + 4 富化），且与 pageSize 无关。
func TestPublicInstruments_QueryCountInvariant_2451(t *testing.T) {
	db, cl := newCountedDB(t)
	tenantA := seedN1Tenant(t, db, "Tenant A")
	siteA := seedN1Site(t, db, tenantA, "Site A", "Addr A", "010-1111")
	seedN1Merchant(t, db, uuid.New().String(), tenantA, models.MerchantTypeControlled, "Transit A")

	seedN1Instruments(t, db, tenantA, &siteA, 100, time.Now().Add(-time.Hour), "SN-QC")
	router := n1Router()

	q10 := n1Queries(t, router, cl, "/api/public/instruments?page=1&pageSize=10")
	q100 := n1Queries(t, router, cl, "/api/public/instruments?page=1&pageSize=100")
	enrich := q100 - 2 // COUNT + 分页 Find 是分页元数据，不属于逐行富化

	t.Logf("QUERY_COUNT pageSize=10=%d pageSize=100=%d ENRICH=%d", q10, q100, enrich)
	require.Equal(t, q10, q100, "查询数必须与 pageSize 无关")
	require.LessOrEqual(t, q100, 6, "每页总查询不得超过 6")
	require.Equal(t, 4, enrich, "富化查询固定 4：缩略图 + sites + tenants + merchants")
}

// R2：响应 21 键齐全 + 派生值正确（受控中转覆盖 / full 用站点地址 / 封面=缩略图）。
func TestPublicInstruments_ResponseContract_2451(t *testing.T) {
	db, _ := newCountedDB(t)
	tenantA := seedN1Tenant(t, db, "Tenant A")
	tenantB := seedN1Tenant(t, db, "Tenant B")
	siteA := seedN1Site(t, db, tenantA, "Site A", "Addr A", "010-1111")
	siteB := seedN1Site(t, db, tenantB, "Site B", "Addr B", "010-2222")
	seedN1Merchant(t, db, uuid.New().String(), tenantA, models.MerchantTypeControlled, "Transit A")
	seedN1Merchant(t, db, uuid.New().String(), tenantB, models.MerchantTypeFull, "")

	idsA := seedN1Instruments(t, db, tenantA, &siteA, 60, time.Now().Add(-2*time.Hour), "SN-CA")
	idsB := seedN1Instruments(t, db, tenantB, &siteB, 40, time.Now().Add(-time.Hour), "SN-CB")

	// 封面：instrument.CoverImage 为空 → 走 instrument_media 缩略图。
	mediaInstrument := idsB[0]
	seedN1DisplayMedia(t, db, tenantB, mediaInstrument, "instruments/2451/cover")

	router := n1Router()
	resp, _ := n1Get(t, router, "/api/public/instruments?page=1&pageSize=100")
	require.Len(t, resp.Data.List, 100)
	require.Equal(t, int64(100), resp.Data.Total)

	expectedKeys := []string{
		"id", "sn", "category_id", "category_name", "level_name", "level_id",
		"images", "cover_image", "pricing", "base_daily_rate", "daily_rate_cents",
		"total_price", "stock_status", "tenant_id", "tenant_name", "site_id",
		"site_name", "site_address", "site_phone", "description", "thumbnail",
	}
	byID := make(map[string]map[string]interface{}, len(resp.Data.List))
	for _, item := range resp.Data.List {
		require.Len(t, item, len(expectedKeys), "item 键数量必须等于 21")
		for _, k := range expectedKeys {
			_, ok := item[k]
			require.True(t, ok, "缺少键 %s", k)
		}
		byID[item["id"].(string)] = item
	}

	for _, id := range idsA {
		item, ok := byID[id]
		require.True(t, ok, "tenant A 乐器缺失: %s", id)
		require.Equal(t, "Tenant A", item["tenant_name"])
		require.Equal(t, "Site A", item["site_name"])
		require.Equal(t, "Transit A", item["site_address"], "受控商户必须用中转地址覆盖站点地址")
		require.Equal(t, "010-1111", item["site_phone"])
	}
	for _, id := range idsB {
		item, ok := byID[id]
		require.True(t, ok, "tenant B 乐器缺失: %s", id)
		require.Equal(t, "Tenant B", item["tenant_name"])
		require.Equal(t, "Site B", item["site_name"])
		require.Equal(t, "Addr B", item["site_address"], "full 商户保留站点地址")
		require.Equal(t, "010-2222", item["site_phone"])
	}

	// 分契约：daily_rate_cents == base_daily_rate（均为分）。
	for _, item := range resp.Data.List {
		require.Equal(t, int64(item["base_daily_rate"].(float64)), int64(item["daily_rate_cents"].(float64)))
	}

	mediaItem := byID[mediaInstrument]
	require.Equal(t, "/uploads/media/instruments/2451/cover", mediaItem["thumbnail"])
	require.Equal(t, "/uploads/media/instruments/2451/cover", mediaItem["cover_image"])

	t.Logf("CONTRACT_KEYS=%d", len(expectedKeys))
	t.Log("CONTRACT_OK")
}

// R3：无 site / 无 tenant 行时保持既有缺省值，且不新增查询。
func TestPublicInstruments_MissingSiteAndTenantDefaults_2451(t *testing.T) {
	db, cl := newCountedDB(t)
	tenant := seedN1Tenant(t, db, "Tenant Normal")
	site := seedN1Site(t, db, tenant, "Site Normal", "Addr Normal", "010-3333")
	seedN1Merchant(t, db, uuid.New().String(), tenant, models.MerchantTypeFull, "")
	seedN1Instruments(t, db, tenant, &site, 3, time.Now().Add(-time.Hour), "SN-OK")

	// tenant 行不存在（instruments.tenant_id 无 FK）+ site_id = NULL。
	orphanTenant := uuid.New().String()
	orphanIDs := seedN1Instruments(t, db, orphanTenant, nil, 1, time.Now(), "SN-ORPHAN")

	router := n1Router()
	before := cl.Count()
	resp, _ := n1Get(t, router, "/api/public/instruments?page=1&pageSize=10")
	queries := cl.Count() - before

	var orphan map[string]interface{}
	for _, item := range resp.Data.List {
		if item["id"].(string) == orphanIDs[0] {
			orphan = item
			break
		}
	}
	require.NotNil(t, orphan, "缺省值样本必须出现在结果中")
	require.Equal(t, "-", orphan["site_name"])
	require.Equal(t, "-", orphan["site_address"])
	require.Equal(t, "-", orphan["site_phone"])
	require.Equal(t, "", orphan["tenant_name"])
	require.LessOrEqual(t, queries, 6, "缺省值路径不得新增查询")

	t.Logf("DEFAULTS_OK site_name=%s site_address=%s site_phone=%s tenant_name=%q queries=%d",
		orphan["site_name"], orphan["site_address"], orphan["site_phone"], orphan["tenant_name"], queries)
}

// R4：批量 helper 与单条 helper 逐字段等价；每 tenant 取 id 最小商户；无商户 tenant 不入 map。
func TestGetMerchantTransitInfos_BatchMatchesSingle_2451(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantA := seedN1Tenant(t, db, "Tenant A")
	tenantB := seedN1Tenant(t, db, "Tenant B")
	tenantC := seedN1Tenant(t, db, "Tenant C")

	firstA := "00000000-0000-0000-0000-000000000001"
	secondA := "00000000-0000-0000-0000-000000000002"
	seedN1Merchant(t, db, secondA, tenantA, models.MerchantTypeControlled, "A-second")
	seedN1Merchant(t, db, firstA, tenantA, models.MerchantTypeControlled, "A-first")
	seedN1Merchant(t, db, uuid.New().String(), tenantB, models.MerchantTypeFull, "B-only")

	batch := GetMerchantTransitInfos(context.Background(), []string{tenantA, tenantB, tenantC})
	require.Len(t, batch, 2, "tenant C 无商户，不得写入 map")
	require.Equal(t, "A-first", batch[tenantA].Address, "必须取 id 最小的商户（复刻 First()）")

	ok := 0
	for _, tenantID := range []string{tenantA, tenantB, tenantC} {
		single := GetMerchantTransitInfo(context.Background(), tenantID)
		got := batch[tenantID]
		if single == nil || got == nil {
			require.Equal(t, single, got, "nil 语义必须一致: tenant=%s", tenantID)
			ok++
			continue
		}
		require.Equal(t, single.MerchantType, got.MerchantType)
		require.Equal(t, single.Address, got.Address)
		require.Equal(t, single.Phone, got.Phone)
		require.Equal(t, single.ContactName, got.ContactName)
		ok++
	}
	require.Equal(t, 3, ok)
	t.Logf("MERCHANT_PARITY tenants=3 ok=%d", ok)
}

// R6：100 件页耗时（旧实现 ~5030ms；确定性门是 R1 的查询数）。
func TestPublicInstruments_100ItemsLatency_2451(t *testing.T) {
	db, _ := newCountedDB(t)
	tenant := seedN1Tenant(t, db, "Tenant Latency")
	site := seedN1Site(t, db, tenant, "Site Latency", "Addr L", "010-4444")
	seedN1Merchant(t, db, uuid.New().String(), tenant, models.MerchantTypeFull, "")
	seedN1Instruments(t, db, tenant, &site, 100, time.Now().Add(-time.Hour), "SN-LAT")

	router := n1Router()
	start := time.Now()
	resp, _ := n1Get(t, router, "/api/public/instruments?page=1&pageSize=100")
	elapsed := time.Since(start).Milliseconds()
	require.Len(t, resp.Data.List, 100)

	t.Logf("LATENCY pageSize=100 elapsed_ms=%d", elapsed)
	require.Less(t, elapsed, int64(1000), "100 件页耗时必须 < 1000ms")
}

// R9：219 件全量 3 页加载不劣化，且分页无重复/无遗漏。
func TestPublicInstruments_FullLoad219_2451(t *testing.T) {
	db, _ := newCountedDB(t)
	tenantA := seedN1Tenant(t, db, "Tenant A")
	tenantB := seedN1Tenant(t, db, "Tenant B")
	siteA := seedN1Site(t, db, tenantA, "Site A", "Addr A", "010-5555")
	siteB := seedN1Site(t, db, tenantB, "Site B", "Addr B", "010-6666")
	seedN1Merchant(t, db, uuid.New().String(), tenantA, models.MerchantTypeControlled, "Transit A")
	seedN1Merchant(t, db, uuid.New().String(), tenantB, models.MerchantTypeFull, "")

	idsA := seedN1Instruments(t, db, tenantA, &siteA, 119, time.Now().Add(-2*time.Hour), "SN-FA")
	idsB := seedN1Instruments(t, db, tenantB, &siteB, 100, time.Now().Add(-time.Hour), "SN-FB")

	router := n1Router()
	seen := make(map[string]struct{}, 219)
	start := time.Now()
	expectedPerPage := map[int]int{1: 100, 2: 100, 3: 19}
	for page := 1; page <= 3; page++ {
		resp, _ := n1Get(t, router, fmt.Sprintf("/api/public/instruments?page=%d&pageSize=100", page))
		require.Len(t, resp.Data.List, expectedPerPage[page], "page %d 行数", page)
		require.Equal(t, int64(219), resp.Data.Total)
		for _, item := range resp.Data.List {
			id := item["id"].(string)
			_, dup := seen[id]
			require.False(t, dup, "跨页重复 ID: %s", id)
			seen[id] = struct{}{}
		}
	}
	total := time.Since(start).Milliseconds()

	for _, id := range append(append([]string{}, idsA...), idsB...) {
		_, ok := seen[id]
		require.True(t, ok, "乐器遗漏: %s", id)
	}

	t.Logf("FULLLOAD pages=3 items=%d total_ms=%d", len(seen), total)
	require.Equal(t, 219, len(seen))
	require.Less(t, total, int64(1300), "三页总耗时必须 < 1300ms")
}
