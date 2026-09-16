package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ECS instance RAM role credentials provider (#1914).
//
// The SDK ships environment-variable and custom providers but no built-in
// ECS metadata provider — implemented here so production/prerelease on
// cadenza (an Aliyun ECS) can run WITHOUT any AccessKey on disk: the
// provider fetches short-lived STS credentials from the metadata service
// (http://100.100.100.100/...) and caches them until shortly before
// expiration, then refreshes automatically.

const (
	ecsMetadataRoleListDefault = "http://100.100.100.100/latest/meta-data/ram/security-credentials"
	ecsTokenRefreshMargin      = 5 * time.Minute
	ecsMetadataTimeout         = 3 * time.Second
)

type ecsRoleCredential struct {
	AccessKeyID     string `json:"AccessKeyId"`
	AccessKeySecret string `json:"AccessKeySecret"`
	SecurityToken   string `json:"SecurityToken"`
	Expiration      string `json:"Expiration"`
	Code            string `json:"Code"`
}

func (c *ecsRoleCredential) GetAccessKeyID() string     { return c.AccessKeyID }
func (c *ecsRoleCredential) GetAccessKeySecret() string { return c.AccessKeySecret }
func (c *ecsRoleCredential) GetSecurityToken() string   { return c.SecurityToken }

// ECSRoleCredentialsProvider implements oss.CredentialsProvider backed by
// the ECS instance RAM role metadata endpoint.
type ECSRoleCredentialsProvider struct {
	baseURL string
	client  *http.Client
	now     func() time.Time

	mu     sync.Mutex
	access string
	secret string
	token  string
	expire time.Time
}

// NewECSRoleCredentialsProvider builds the provider for the ECS metadata
// service. baseURL injectable for tests.
func NewECSRoleCredentialsProvider(baseURL string) (*ECSRoleCredentialsProvider, error) {
	if baseURL == "" {
		baseURL = ecsMetadataRoleListDefault
	}
	return &ECSRoleCredentialsProvider{
		baseURL: baseURL,
		client:  &http.Client{Timeout: ecsMetadataTimeout},
		now:     time.Now,
	}, nil
}

func (p *ECSRoleCredentialsProvider) get(path string) (string, error) {
	resp, err := p.client.Get(p.baseURL + path)
	if err != nil {
		return "", fmt.Errorf("ecs metadata request %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ecs metadata %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", fmt.Errorf("ecs metadata read %s: %w", path, err)
	}
	return string(data), nil
}

func (p *ECSRoleCredentialsProvider) refresh() error {
	roleList, err := p.get("/")
	if err != nil {
		return err
	}
	roleName := strings.TrimSpace(roleList)
	if roleName == "" {
		return fmt.Errorf("ecs metadata returned empty instance role name")
	}

	raw, err := p.get("/" + roleName)
	if err != nil {
		return err
	}
	var c ecsRoleCredential
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return fmt.Errorf("parse ecs metadata credentials: %w", err)
	}
	if c.Code != "" && c.Code != "Success" {
		return fmt.Errorf("ecs metadata credentials code=%s", c.Code)
	}
	if c.AccessKeyID == "" || c.AccessKeySecret == "" || c.SecurityToken == "" {
		return fmt.Errorf("ecs metadata credentials missing fields (role=%s)", roleName)
	}
	exp, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return fmt.Errorf("parse credential expiration %q: %w", c.Expiration, err)
	}

	p.access, p.secret, p.token, p.expire = c.AccessKeyID, c.AccessKeySecret, c.SecurityToken, exp
	return nil
}

// GetCredentials returns fresh credentials, refreshing (and caching) from
// the metadata service when unset or near expiry.
func (p *ECSRoleCredentialsProvider) GetCredentials() oss.Credentials {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.access == "" || p.now().After(p.expire.Add(-ecsTokenRefreshMargin)) {
		if err := p.refresh(); err != nil {
			// Return last known credentials if present; otherwise empty —
			// the SDK will surface auth errors on the first request.
			if p.access == "" {
				return &ecsRoleCredential{}
			}
		}
	}
	return &ecsRoleCredential{
		AccessKeyID:     p.access,
		AccessKeySecret: p.secret,
		SecurityToken:   p.token,
		Expiration:      p.expire.Format(time.RFC3339),
	}
}

// resolveCredentialsProvider implements the #1914 credential chain:
//  1. env AccessKey (local dev fallback, honored first when set)
//  2. ECS instance RAM role (production/prerelease on cadenza, no AK on disk)
func resolveCredentialsProvider() (oss.CredentialsProvider, error) {
	if os.Getenv("OSS_ACCESS_KEY_ID") != "" {
		p, err := oss.NewEnvironmentVariableCredentialsProvider()
		if err != nil {
			return nil, fmt.Errorf("environment credentials provider: %w", err)
		}
		return &p, nil
	}
	return NewECSRoleCredentialsProvider("")
}
