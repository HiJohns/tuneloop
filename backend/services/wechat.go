package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type wxTokenResponse struct {
	AccessToken string `json:"access_token"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

func GetWxAccessToken() (string, error) {
	appID := os.Getenv("WX_APPID")
	appSecret := os.Getenv("WX_APPSECRET")
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

	return result.AccessToken, nil
}
