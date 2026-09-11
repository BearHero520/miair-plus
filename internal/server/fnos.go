package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

var fnosHTTP = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/trim_open_gateway_apiscope.socket")
	}},
}

func fnosCall(ctx context.Context, client *http.Client, method string, data, output any) error {
	token := os.Getenv("TRIM_API_TOKEN")
	if token == "" {
		return errors.New("飞牛 API 尚未就绪，请在应用中心重新启动 MiAir Plus")
	}
	body, err := json.Marshal(map[string]any{"req": method, "appName": "miair-plus", "data": data})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/trimapp", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("无法连接飞牛开放 API，请检查 fnOS 版本及应用状态")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("飞牛 API 请求失败（HTTP %d）", resp.StatusCode)
	}
	var envelope struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil || envelope.Code == nil {
		return errors.New("飞牛 API 返回格式无效")
	}
	if *envelope.Code != 0 {
		return fmt.Errorf("飞牛 API 调用失败（错误码 %d），请检查应用授权", *envelope.Code)
	}
	return json.Unmarshal(envelope.Data, output)
}

func gatewayUID(r *http.Request) (int, error) {
	gateway, _ := r.Context().Value(gatewayRequestKey{}).(bool)
	uid, err := strconv.Atoi(r.Header.Get("X-Trim-Userid"))
	if !gateway || err != nil || uid < 0 {
		return 0, errors.New("请从飞牛应用入口选择 NAS 文件，以确认当前用户权限；直接端口访问可使用上传音乐")
	}
	return uid, nil
}

func checkFnosRead(ctx context.Context, client *http.Client, uid int, path string) error {
	var permissions []struct {
		Path     string `json:"path"`
		Readable bool   `json:"readable"`
	}
	if err := fnosCall(ctx, client, "trim.file.checkUserACL", map[string]any{"uid": uid, "path": path}, &permissions); err != nil {
		return err
	}
	for _, p := range permissions {
		if p.Path == path && p.Readable {
			return nil
		}
	}
	return errors.New("当前飞牛用户没有此文件的读取权限，请重新选择并授权")
}
