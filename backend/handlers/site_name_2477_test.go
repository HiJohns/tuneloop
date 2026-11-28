package handlers

// #2477：网点名称归一化（trim + 连续空白折叠，含 U+3000 全角空白/制表/换行）。
//
// Owner 2026-11-26 裁 B：重名判定唯一权威 = IAM CreateOrganization，本地**不预检**、
// **不加**唯一索引。本单只负责「归一化」与「两处同串写入」。
//
// 证据行（Accepted Plan v2 R1–R4）：
//   R1 归一化纯函数        → NORM_TRIM / NORM_FULLWIDTH / NORM_TAB / NORM_EMPTY / NORM_CASE_KEPT
//   R2 CreateSite 写两处   → IAM_NAME_TRIMMED=1 / LOCAL_NAME_TRIMMED=1 / NAMES_EQUAL=1 / IAM_CALLS=1
//   R3 归一化后为空        → HTTP=400 / CODE=40002 / MSG_ZH=1 / IAM_CALLS=0
//   R4 UpdateSite 共用归一化 → IAM_CALLS=0（全角空格变体视为未改名，不产生漂移）/ NAMES_EQUAL=1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 全角空白（U+3000）与普通空格的视觉重名样本。
const (
	siteNameFullWidthSpace2477 = "海淀\u3000店"
	siteNameAsciiSpace2477     = "海淀 店"
)

// hasCJK2477 判定 message 是否含 CJK（前端「含 CJK 原样透传」口径的机器判据）。
func hasCJK2477(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// fakeIAMCreateSite2477 覆盖 CreateSite 使用的 IAM 端点，并记录 POST /organizations
// 实际收到的组织名（用于断言「写入 IAM 的是归一化串」）与调用次数。
type fakeIAMCreateSite2477 struct {
	srv      *httptest.Server
	mu       sync.Mutex
	orgCalls int
	lastName string
}

func newFakeIAMCreateSite2477(t *testing.T) *fakeIAMCreateSite2477 {
	t.Helper()
	f := &fakeIAMCreateSite2477{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token-2477", "expires_in": 3600, "token_type": "Bearer",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organizations"):
			var req services.CreateOrganizationRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.mu.Lock()
			f.orgCalls++
			f.lastName = req.Name
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"org_id": uuid.NewString(), "admin_id": siteCreateNilUUID2581,
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "role-templates"):
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "tpl-site-admin", "code": "site_admin"},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/namespaces/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "ns-2477"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000})
		}
	}))
	t.Cleanup(f.srv.Close)
	services.SetIAMInternalURLForTesting(f.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return f
}

func (f *fakeIAMCreateSite2477) snapshot() (calls int, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.orgCalls, f.lastName
}

// R1：归一化纯函数——trim / 全角空白 / 制表 / 空串 / 不折叠大小写。
func TestNormalizeSiteName2477(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"trim", " 海淀店 ", "海淀店"},
		{"fullwidth", "海淀\u3000\u3000店", siteNameAsciiSpace2477},
		{"tab", "海淀\t店", siteNameAsciiSpace2477},
		{"newline", "海淀\n店", siteNameAsciiSpace2477},
		{"empty", "   ", ""},
		{"empty_fullwidth", "\u3000\u3000", ""},
		{"case_kept", "Hai Dian", "Hai Dian"},
		{"no_double_space", "海淀  钢琴  店", "海淀 钢琴 店"},
	}

	trim, fullwidth, tab, empty, caseKept := 0, 0, 0, 0, 0
	for _, tc := range cases {
		got := NormalizeSiteName(tc.in)
		require.Equal(t, tc.want, got, "case %s: NormalizeSiteName(%q)", tc.name, tc.in)
		switch tc.name {
		case "trim", "newline", "no_double_space":
			trim = 1
		case "fullwidth":
			fullwidth = 1
		case "tab":
			tab = 1
		case "empty", "empty_fullwidth":
			empty = 1
		case "case_kept":
			// 不折叠大小写：小写输入不得被改写为大写（反之亦然）。
			require.Equal(t, "haidian", NormalizeSiteName("haidian"))
			caseKept = 1
		}
	}

	require.Equal(t, 1, trim)
	require.Equal(t, 1, fullwidth)
	require.Equal(t, 1, tab)
	require.Equal(t, 1, empty)
	require.Equal(t, 1, caseKept)

	t.Logf("NORM_TRIM=%d", trim)
	t.Logf("NORM_FULLWIDTH=%d", fullwidth)
	t.Logf("NORM_TAB=%d", tab)
	t.Logf("NORM_EMPTY=%d", empty)
	t.Logf("NORM_CASE_KEPT=%d", caseKept)
}

// 红样例（缺陷复现，非验收行）：修复前 `CreateSite` 原样写 `req.Name`、`UpdateSite` 只做
// `strings.TrimSpace`，于是「半角空格」与「全角空格 U+3000」两个视觉同串在 IAM 与本地
// 仍是两个不同的名字 —— 这正是 #2477 的缺陷面。修复后必须归一化到同一串。
func TestTrimSpaceMissesWhitespaceVariants2477(t *testing.T) {
	oldTrimmed := strings.TrimSpace("测试网点\u3000A") // 修复前的口径（trim only）
	asciiTrimmed := strings.TrimSpace("测试网点 A")

	require.NotEqual(t, asciiTrimmed, oldTrimmed,
		"红样例：仅 TrimSpace 无法折叠全角空白，两个视觉同串仍是不同名字（缺陷成立）")
	require.Equal(t, asciiTrimmed, NormalizeSiteName("测试网点\u3000A"),
		"修复后：全角空白折叠为半角，两串相同")

	t.Logf("RED_OLD_TRIMSPACE_DISTINCT=1")
	t.Logf("GREEN_NORMALIZED_SAME=1")
}

// R2：CreateSite 以归一化串同时写 IAM 组织名与本地 sites.name；不预检（IAM_CALLS=1）。
func TestCreateSiteNormalizedName2477(t *testing.T) {
	testfixtures.SetupTestDB(t)
	db := testfixtures.GetTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	iam := newFakeIAMCreateSite2477(t)
	captureStdLog2479(t)

	code, resp := postCreateSite2479(t, merchantOrg, tenantID,
		`{"name":"  海淀\u3000店  "}`)
	require.Equal(t, http.StatusCreated, code, "归一化名应创建成功; body=%v", resp)

	calls, iamName := iam.snapshot()
	require.Equal(t, 1, calls, "裁 B：不预检，创建必须调用 IAM 一次")
	require.Equal(t, siteNameAsciiSpace2477, iamName,
		"写入 IAM 的组织名必须是归一化串（全角空白折叠为单个 ASCII 空格）")

	var site models.Site
	require.NoError(t, db.Where("tenant_id = ?", merchantOrg).First(&site).Error)
	require.Equal(t, siteNameAsciiSpace2477, site.Name, "本地 sites.name 也必须是归一化串")
	require.Equal(t, iamName, site.Name, "IAM 名与本地名必须同串（防 sites.name ≠ organizations.name 漂移）")

	iamTrimmed, localTrimmed, namesEqual := 0, 0, 0
	if !strings.Contains(iamName, "\u3000") && iamName == strings.TrimSpace(iamName) {
		iamTrimmed = 1
	}
	if !strings.Contains(site.Name, "\u3000") && site.Name == strings.TrimSpace(site.Name) {
		localTrimmed = 1
	}
	if iamName == site.Name {
		namesEqual = 1
	}

	t.Logf("IAM_NAME=%s", iamName)
	t.Logf("LOCAL_NAME=%s", site.Name)
	t.Logf("IAM_NAME_TRIMMED=%d", iamTrimmed)
	t.Logf("LOCAL_NAME_TRIMMED=%d", localTrimmed)
	t.Logf("NAMES_EQUAL=%d", namesEqual)
	t.Logf("IAM_CALLS=%d", calls)
}

// R3：归一化后为空 → 400/40002 + 中文，且不得调用 IAM（IAM_CALLS=0）。
func TestCreateSiteEmptyName2477(t *testing.T) {
	testfixtures.SetupTestDB(t)
	db := testfixtures.GetTestDB(t)
	merchantOrg, tenantID := seedMerchantForSiteCreate2479(t)
	iam := newFakeIAMCreateSite2477(t)
	captureStdLog2479(t)

	code, resp := postCreateSite2479(t, merchantOrg, tenantID, `{"name":"\u3000  "}`)
	require.Equal(t, http.StatusBadRequest, code, "全空白名必须 400; body=%v", resp)

	respCode := 0
	if v, ok := resp["code"].(float64); ok {
		respCode = int(v)
	}
	msg, _ := resp["message"].(string)
	require.Equal(t, 40002, respCode)
	require.True(t, hasCJK2477(msg), "message 必须是中文可行动提示，实际=%q", msg)

	calls, _ := iam.snapshot()
	require.Zero(t, calls, "归一化后为空必须在调用 IAM 之前返回")

	var sites int64
	require.NoError(t, db.Model(&models.Site{}).Where("tenant_id = ?", merchantOrg).Count(&sites).Error)
	require.Zero(t, sites, "空名不得落库")

	msgZH := 0
	if hasCJK2477(msg) {
		msgZH = 1
	}

	t.Logf("HTTP=%d", code)
	t.Logf("CODE=%d", respCode)
	t.Logf("MSG_ZH=%d", msgZH)
	t.Logf("IAM_CALLS=%d", calls)
}

// R4：UpdateSite 共用同一归一化函数——全角空格变体归一化后等于现值，视为未改名，
// 不调 IAM、不写本地（IAM_CALLS=0），因此不会产生「视觉重名」漂移。
func TestUpdateSiteNormalizedNoop2477(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, siteNameAsciiSpace2477)
	iam := newFakeIAMSite2457(t, http.StatusOK, siteNameAsciiSpace2477)

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID,
		"{\"name\":\"海淀\u3000店\"}", siteUpdateUserToken2457)
	require.Equal(t, http.StatusOK, code, "%v", resp)

	calls := iam.callCount()
	require.Zero(t, calls, "归一化后与现值相同 → 视为未改名，不得调用 IAM")

	after := siteName2457(t, db, siteID)
	require.Equal(t, siteNameAsciiSpace2477, after.Name, "本地名不得被全角空格变体覆盖")

	namesEqual := 0
	if after.Name == siteNameAsciiSpace2477 {
		namesEqual = 1
	}

	t.Logf("LOCAL_NAME=%s", after.Name)
	t.Logf("NAMES_EQUAL=%d", namesEqual)
	t.Logf("IAM_CALLS=%d", calls)
}

// R4 补充：真实改名时，写入 IAM 与本地的是同一个归一化串（不是原始带空白输入）。
func TestUpdateSiteNormalizedRename2477(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	tenantID, siteID := seedSite2457(t, db, "旧店")
	iam := newFakeIAMSite2457(t, http.StatusOK, siteNameAsciiSpace2477)

	r := siteUpdateRouter2457(tenantID, 0)
	code, resp := putSite2457(t, r, siteID,
		"{\"name\":\" 新\u3000店 \"}", siteUpdateUserToken2457)
	require.Equal(t, http.StatusOK, code, "%v", resp)

	body, _, _ := iam.snapshot()
	var iamBody struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &iamBody))

	after := siteName2457(t, db, siteID)
	require.Equal(t, "新 店", iamBody.Name, "IAM body 必须是归一化串")
	require.Equal(t, "新 店", after.Name, "本地名必须是归一化串")
	require.Equal(t, iamBody.Name, after.Name, "两处必须同串")
	require.Equal(t, 1, iam.callCount())

	t.Logf("RENAME_IAM_NAME=%s", iamBody.Name)
	t.Logf("RENAME_LOCAL_NAME=%s", after.Name)
	t.Logf("RENAME_IAM_CALLS=%d", iam.callCount())
}
