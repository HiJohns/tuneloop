package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// newSubjectMockIAM (#1735) starts an IAM stub serving client tokens plus a
// caller-defined /api/v1/users/<id> handler, and points the IAMClient at it.
func newSubjectMockIAM(t *testing.T, userHandler http.HandlerFunc) *services.IAMClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users/", userHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	return services.NewIAMClientWithCredentials("test-ns", "test-secret")
}

func subjectTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	return c, w
}

func subjectClaims(userID, role string, iat time.Time) *services.JWTClaims {
	return &services.JWTClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(iat),
		},
	}
}

func assertSemanticCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	require.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.EqualValues(t, want, body["code"])
}

// Deleted account → explicit account_not_found (40105), never anonymous
// pass-through or silent degradation to guest.
func TestEnforceSubjectValidity_UserDeleted_Returns40105(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newSubjectMockIAM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	c, w := subjectTestContext()
	ok := enforceSubjectValidity(c, client, subjectClaims("u-1", "customer", time.Now()))

	require.False(t, ok)
	assertSemanticCode(t, w, 40105)
}

// Deactivated account → explicit account_inactive (40106).
func TestEnforceSubjectValidity_UserInactive_Returns40106(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newSubjectMockIAM(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]interface{}{"id": "u-1", "status": "inactive", "token_version": 0},
		})
	})

	c, w := subjectTestContext()
	ok := enforceSubjectValidity(c, client, subjectClaims("u-1", "customer", time.Now()))

	require.False(t, ok)
	assertSemanticCode(t, w, 40106)
}

// Token issued before the latest token_version bump (password change) →
// explicit token_revoked (40107).
func TestEnforceSubjectValidity_TokenPredatesTokenVersion_Returns40107(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bumpMs := time.Now().Add(10 * time.Second).UnixMilli() // bumped after token issuance
	client := newSubjectMockIAM(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]interface{}{"id": "u-1", "status": "active", "token_version": bumpMs},
		})
	})

	c, w := subjectTestContext()
	ok := enforceSubjectValidity(c, client, subjectClaims("u-1", "customer", time.Now().Add(-time.Minute)))

	require.False(t, ok)
	assertSemanticCode(t, w, 40107)
}

// Token issued at/after the bump passes; iat equal to token_version is valid.
func TestEnforceSubjectValidity_ActiveUserFreshToken_Passes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	iat := time.Now()
	client := newSubjectMockIAM(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]interface{}{"id": "u-1", "status": "active", "token_version": iat.UnixMilli()},
		})
	})

	c, _ := subjectTestContext()
	ok := enforceSubjectValidity(c, client, subjectClaims("u-1", "customer", iat))

	require.True(t, ok)
}

// Locally-issued GUEST tokens have no backing IAM user and skip the check.
func TestEnforceSubjectValidity_GuestRole_SkipsCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Server would 404 everything — must never be consulted for GUEST.
	client := newSubjectMockIAM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	c, _ := subjectTestContext()
	ok := enforceSubjectValidity(c, client, subjectClaims("", "GUEST", time.Now()))

	require.True(t, ok)
}

// Transient IAM failures fail open so beaconiam hiccups don't log everyone out.
func TestEnforceSubjectValidity_IAMUnavailable_FailsOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newSubjectMockIAM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c, w := subjectTestContext()
	ok := enforceSubjectValidity(c, client, subjectClaims("u-1", "customer", time.Now()))

	require.True(t, ok, "network/transient errors must not reject the request")
	require.NotEqual(t, http.StatusUnauthorized, w.Code)
}

// Nil claims / nil client are defensive no-ops.
func TestEnforceSubjectValidity_NilInputs_PassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := subjectTestContext()
	require.True(t, enforceSubjectValidity(c, nil, subjectClaims("u-1", "customer", time.Now())))
	require.True(t, enforceSubjectValidity(c, nil, nil))
}
