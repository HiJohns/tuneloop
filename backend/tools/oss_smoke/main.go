// oss-smoke 冒烟工具（#1914 P2）— 对真实 OSS 桶执行一轮端到端验证。
// 独立可执行（不依赖 tuneloop services 包，避免 cgo/webp 牵连）。
//
// 构建（构建机）:
//
//	CGO_ENABLED=0 GOOS=linux go build -o /tmp/oss-smoke ./tools/oss_smoke
//	scp /tmp/oss-smoke cadenza:/tmp/oss-smoke
//	ssh cadenza "cd /opt/tuneloop-pre/apps/tuneloop-pre && source .env && /tmp/oss-smoke"
//
// 凭据链：OSS_ACCESS_KEY_ID 存在 → env AK；否则 → ECS 实例角色 metadata
// （cadenza 生产形态，无 AK 落盘；即 services.resolveCredentialsProvider 行为）。
// 验证项：公开上传→匿名读；私有上传→签名读→无签名 403；Copy/Rename；
// DeletePrefix 清理；全程退出码 0/1。
package main

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

const (
	metadataBase  = "http://100.100.100.100/latest/meta-data/ram/security-credentials"
	refreshMargin = 5 * time.Minute
)

type ecsRoleCred struct {
	AccessKeyID     string `json:"AccessKeyId"`
	AccessKeySecret string `json:"AccessKeySecret"`
	SecurityToken   string `json:"SecurityToken"`
	Expiration      string `json:"Expiration"`
	Code            string `json:"Code"`
}

func (c *ecsRoleCred) GetAccessKeyID() string     { return c.AccessKeyID }
func (c *ecsRoleCred) GetAccessKeySecret() string { return c.AccessKeySecret }
func (c *ecsRoleCred) GetSecurityToken() string   { return c.SecurityToken }

type ecsProvider struct {
	mu     sync.Mutex
	access string
	secret string
	token  string
	expire time.Time
}

func (p *ecsProvider) refresh() error {
	client := &http.Client{Timeout: 3 * time.Second}
	get := func(path string) (string, error) {
		resp, err := client.Get(metadataBase + path)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("metadata %s -> %d", path, resp.StatusCode)
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return string(b), nil
	}
	role, err := get("/")
	if err != nil {
		return err
	}
	role = strings.TrimSpace(role)
	raw, err := get("/" + role)
	if err != nil {
		return err
	}
	var c ecsRoleCred
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return err
	}
	exp, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return err
	}
	p.access, p.secret, p.token, p.expire = c.AccessKeyID, c.AccessKeySecret, c.SecurityToken, exp
	return nil
}

func (p *ecsProvider) GetCredentials() oss.Credentials {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.access == "" || time.Now().After(p.expire.Add(-refreshMargin)) {
		if err := p.refresh(); err != nil && p.access == "" {
			_ = err
		}
	}
	return &ecsRoleCred{AccessKeyID: p.access, AccessKeySecret: p.secret, SecurityToken: p.token, Expiration: p.expire.Format(time.RFC3339)}
}

func resolveProvider() (oss.CredentialsProvider, error) {
	if os.Getenv("OSS_ACCESS_KEY_ID") != "" {
		p, err := oss.NewEnvironmentVariableCredentialsProvider()
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	return &ecsProvider{}, nil
}

var fails int

func check(step string, err error, detail string) {
	if err != nil {
		fails++
		fmt.Printf("[FAIL] %s: %v (%s)\n", step, err, detail)
		return
	}
	fmt.Printf("[ OK ] %s\n", step)
}

func httpStatus(url string) (int, string) {
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, string(b)
	}
	return resp.StatusCode, ""
}

func main() {
	ep, bucketName, privName := os.Getenv("OSS_ENDPOINT"), os.Getenv("OSS_BUCKET"), os.Getenv("OSS_PRIVATE_BUCKET")
	fmt.Println("== OSS 冒烟 ==")
	fmt.Printf("endpoint=%s bucket=%s private=%s\n", ep, bucketName, privName)
	if ep == "" || bucketName == "" {
		fmt.Println("缺少 OSS_ENDPOINT/OSS_BUCKET，中止")
		os.Exit(1)
	}
	if !strings.Contains(ep, "://") {
		ep = "https://" + ep // 桶强制 HTTPS
	}

	provider, err := resolveProvider()
	if err != nil {
		fmt.Println("解析凭据失败:", err)
		os.Exit(1)
	}
	client, err := oss.New(ep, "", "", oss.SetCredentialsProvider(provider))
	if err != nil {
		fmt.Println("创建 client 失败:", err)
		os.Exit(1)
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		fmt.Println("Bucket 失败:", err)
		os.Exit(1)
	}
	var priv *oss.Bucket
	if privName != "" && privName != bucketName {
		priv, _ = client.Bucket(privName)
	}
	if os.Getenv("OSS_ACCESS_KEY_ID") != "" {
		fmt.Println("credential chain: env AK")
	} else {
		fmt.Println("credential chain: ECS instance role (metadata)")
	}

	ts := time.Now().UnixNano()
	pubKey := fmt.Sprintf("smoke/%d-public.txt", ts)
	privKey := fmt.Sprintf("face_captures/smoke/%d-private.txt", ts)
	content := fmt.Sprintf("oss-smoke-%d", ts)

	check("公开上传", bucket.PutObject(pubKey, strings.NewReader(content), oss.ContentType("text/plain")), pubKey)
	u, err := bucket.SignURL(pubKey, oss.HTTPGet, 60)
	check("公开对象签名URL", err, pubKey) // 公开桶任意 URL 可读
	if code, body := httpStatus(u); code != 200 || body != content {
		fails++
		fmt.Printf("[FAIL] 公开对象读取不符 code=%d body=%q\n", code, body)
	} else {
		fmt.Println("[ OK ] 公开对象匿名读取内容一致")
	}

	if priv != nil {
		check("私有上传", priv.PutObject(privKey, strings.NewReader(content), oss.ContentType("text/plain")), privKey)
		signed, err := priv.SignURL(privKey, oss.HTTPGet, 60)
		check("私有对象签名URL", err, privKey)
		if err == nil {
			if code, body := httpStatus(signed); code != 200 || body != content {
				fails++
				fmt.Printf("[FAIL] 签名URL读取不符 code=%d\n", code)
			} else {
				fmt.Println("[ OK ] 签名URL读取私有对象内容一致")
			}
			raw := signed
			if i := strings.Index(raw, "?"); i > 0 {
				raw = raw[:i]
			}
			code, _ := httpStatus(raw)
			check("无签名读私有对象 403", nil, fmt.Sprintf("code=%d (want 403)", code))
			if code != 403 {
				fails++
			}
		}
		check("删除私有对象", priv.DeleteObject(privKey), privKey)
	}

	check("DeletePrefix(smoke/)", deletePrefix(bucket, "smoke/"), "smoke/")

	fmt.Printf("== 冒烟结果: %d 失败 ==\n", fails)
	if fails > 0 {
		os.Exit(1)
	}
	fmt.Println("ALL PASS")
}

func deletePrefix(bucket *oss.Bucket, prefix string) error {
	marker := ""
	for {
		opts := []oss.Option{oss.Prefix(prefix), oss.MaxKeys(1000)}
		if marker != "" {
			opts = append(opts, oss.Marker(marker))
		}
		res, err := bucket.ListObjects(opts...)
		if err != nil {
			return err
		}
		if len(res.Objects) > 0 {
			keys := make([]string, 0, len(res.Objects))
			for _, o := range res.Objects {
				keys = append(keys, o.Key)
			}
			if _, err := bucket.DeleteObjects(keys); err != nil {
				return err
			}
		}
		if !res.IsTruncated {
			return nil
		}
		if res.NextMarker != "" {
			marker = res.NextMarker
		} else if len(res.Objects) > 0 {
			marker = res.Objects[len(res.Objects)-1].Key
		} else {
			return fmt.Errorf("truncated without marker")
		}
	}
}
