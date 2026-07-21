package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type updateOrderDetailPathReq struct {
	Path string `json:"path"`
}

type orderDetailPathResp struct {
	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
	Path    string `json:"path,omitempty"`
}

func UpdateOrderDetailPath(path string) error {
	token, err := GetWxAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	reqBody := updateOrderDetailPathReq{Path: path}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://api.weixin.qq.com/wxa/sec/order/update_order_detail_path?access_token=%s", token)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("update_order_detail_path request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read update_order_detail_path response: %w", err)
	}

	var result orderDetailPathResp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse update_order_detail_path response: %w", err)
	}

	if result.Errcode != 0 {
		return fmt.Errorf("update_order_detail_path failed: %s (errcode=%d)", result.Errmsg, result.Errcode)
	}
	return nil
}

func GetOrderDetailPath() (string, error) {
	token, err := GetWxAccessToken()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	url := fmt.Sprintf("https://api.weixin.qq.com/wxa/sec/order/get_order_detail_path?access_token=%s", token)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("get_order_detail_path request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read get_order_detail_path response: %w", err)
	}

	var result orderDetailPathResp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse get_order_detail_path response: %w", err)
	}

	if result.Errcode != 0 {
		return "", fmt.Errorf("get_order_detail_path failed: %s (errcode=%d)", result.Errmsg, result.Errcode)
	}

	return result.Path, nil
}
