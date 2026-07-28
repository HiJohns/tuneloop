package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type wxTokenResponse struct {
	AccessToken string `json:"access_token"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

var (
	cachedToken     string
	cachedTokenExp  time.Time
	tokenCacheMutex sync.RWMutex
)

func GetWxAccessToken() (string, error) {
	tokenCacheMutex.RLock()
	if cachedToken != "" && time.Now().Before(cachedTokenExp) {
		defer tokenCacheMutex.RUnlock()
		return cachedToken, nil
	}
	tokenCacheMutex.RUnlock()

	tokenCacheMutex.Lock()
	defer tokenCacheMutex.Unlock()
	if cachedToken != "" && time.Now().Before(cachedTokenExp) {
		return cachedToken, nil
	}

	appID := os.Getenv("WX_APPID")
	appSecret := os.Getenv("WX_APPSECRET")
	if appSecret == "" {
		appSecret = os.Getenv("WX_SECRET")
	}
	if appID == "" || appSecret == "" {
		return "", fmt.Errorf("WX_APPID or WX_APPSECRET not configured")
	}

	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s", appID, appSecret)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get WeChat access token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read WeChat token response: %w", err)
	}

	var result wxTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse WeChat token response: %w", err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("WeChat token error: %s (errcode=%d)", result.ErrMsg, result.ErrCode)
	}

	cachedToken = result.AccessToken
	cachedTokenExp = time.Now().Add(7100 * time.Second)
	return result.AccessToken, nil
}

type wxacodeRequest struct {
	Scene string `json:"scene"`
	Page  string `json:"page"`
	Width int    `json:"width"`
}

func GetWxacodeUnlimited(accessToken, scene, page string) ([]byte, error) {
	url := fmt.Sprintf("https://api.weixin.qq.com/wxa/getwxacodeunlimit?access_token=%s", accessToken)
	reqBody := wxacodeRequest{
		Scene: scene,
		Page:  page,
		Width: 430,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal wxacode request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to call getwxacodeunlimit: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read wxacode response: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "" && ct != "image/jpeg" && ct != "image/png" {
		var wxErr struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		if json.Unmarshal(data, &wxErr) == nil && wxErr.ErrCode != 0 {
			return nil, fmt.Errorf("wxacode error: %s (errcode=%d)", wxErr.ErrMsg, wxErr.ErrCode)
		}
	}

	return data, nil
}
