package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ECS metadata provider: role listing + credentials fetch, cache and
// refresh-on-expiry behavior.
func TestECSRoleCredentialsProvider(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/": // role list
			fmt.Fprint(w, "oss-role\n")
		case "/oss-role":
			calls++
			json.NewEncoder(w).Encode(map[string]interface{}{
				"AccessKeyId":     "AK" + fmt.Sprint(calls),
				"AccessKeySecret": "SK" + fmt.Sprint(calls),
				"SecurityToken":   "TOK" + fmt.Sprint(calls),
				"Expiration":      time.Now().Add(time.Hour).Format(time.RFC3339),
				"Code":            "Success",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p, err := NewECSRoleCredentialsProvider(srv.URL)
	require.NoError(t, err)

	c1 := p.GetCredentials()
	assert.Equal(t, "AK1", c1.GetAccessKeyID())
	assert.Equal(t, "SK1", c1.GetAccessKeySecret())
	assert.Equal(t, "TOK1", c1.GetSecurityToken())
	assert.Equal(t, 1, calls, "first call refreshes")

	// cached: no second metadata hit
	c2 := p.GetCredentials()
	assert.Equal(t, "AK1", c2.GetAccessKeyID())
	assert.Equal(t, 1, calls, "cached credentials must not re-fetch")

	// near expiry -> refresh
	p.expire = time.Now().Add(ecsTokenRefreshMargin - time.Minute)
	c3 := p.GetCredentials()
	assert.Equal(t, "AK2", c3.GetAccessKeyID())
	assert.Equal(t, 2, calls, "refresh expected before token expiry")
}

func TestECSRoleCredentialsProviderErrors(t *testing.T) {
	// 500 on everything
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, err := NewECSRoleCredentialsProvider(srv.URL)
	require.NoError(t, err)
	c := p.GetCredentials()
	assert.Empty(t, c.GetAccessKeyID(), "failed metadata must yield empty credentials, not panic")

	// empty role list
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "")
	}))
	defer srv2.Close()
	p2, _ := NewECSRoleCredentialsProvider(srv2.URL)
	assert.Empty(t, p2.GetCredentials().GetAccessKeyID())
}

// resolveCredentialsProvider prefers env AK over ECS metadata.
func TestResolveCredentialsProviderChain(t *testing.T) {
	t.Setenv("OSS_ACCESS_KEY_ID", "AK")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "SK")
	prov, err := resolveCredentialsProvider()
	require.NoError(t, err)
	c := prov.GetCredentials()
	assert.Equal(t, "AK", c.GetAccessKeyID())
	assert.Equal(t, "SK", c.GetAccessKeySecret())
}
