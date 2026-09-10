// Xiaomi account/MiNA wire formats are based on MiAir Next and miservice-fork.
// This implementation uses Go net/http, verified TLS and per-client timeouts.
package xiaomi

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/diagnostics"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	Store              *config.Store
	HTTP               *http.Client
	AccountURL, APIURL string
	authMu             sync.Mutex
	lastRefresh        time.Time
}

func New(s *config.Store) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{Store: s, HTTP: &http.Client{Timeout: 15 * time.Second, Jar: jar}, AccountURL: "https://account.xiaomi.com", APIURL: "https://api2.mina.mi.com"}
}
func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return string(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}
func number(v any) int { n, _ := strconv.Atoi(str(v)); return n }
func (c *Client) ua() string {
	return "Android-7.1.1-1.0.0-ONEPLUS A3010-136-" + c.Store.Snapshot().Xiaomi.DeviceID + " APP/xiaomi.smarthome APPV/62830"
}
func (c *Client) request(ctx context.Context, client *http.Client, method, uri string, data url.Values, cookies map[string]string) (map[string]any, error) {
	var body io.Reader
	if data != nil {
		body = strings.NewReader(data.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, uri, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua())
	if data != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("小米网络请求失败: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 {
		return map[string]any{"code": json.Number("401")}, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("小米接口 HTTP %d", resp.StatusCode)
	}
	b = bytes.TrimPrefix(b, []byte("&&&START&&&"))
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var result map[string]any
	if err = dec.Decode(&result); err != nil {
		return nil, errors.New("小米接口返回了非 JSON 响应")
	}
	return result, nil
}
func (c *Client) finish(ctx context.Context, r map[string]any, previous config.Credentials) (config.Credentials, error) {
	location, security := str(r["location"]), str(r["ssecurity"])
	if location == "" || security == "" {
		return previous, errors.New("小米登录缺少凭据，可能需要二次验证")
	}
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "https" || !(strings.HasSuffix(u.Hostname(), ".xiaomi.com") || strings.HasSuffix(u.Hostname(), ".mi.com")) {
		return previous, errors.New("小米登录返回了非预期的认证地址")
	}
	digest := sha1.Sum([]byte("nonce=" + str(r["nonce"]) + "&" + security))
	q := u.Query()
	q.Set("clientSign", base64.StdEncoding.EncodeToString(digest[:]))
	q.Set("_userIdNeedEncrypt", "true")
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("User-Agent", c.ua())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return previous, errors.New("小米凭据换发网络失败")
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	token := ""
	for _, cookie := range append(resp.Cookies(), c.HTTP.Jar.Cookies(resp.Request.URL)...) {
		if cookie.Name == "serviceToken" {
			token = cookie.Value
		}
	}
	if token == "" {
		return previous, errors.New("小米未返回 serviceToken")
	}
	next := previous
	next.ServiceToken = token
	next.Security = security
	next.IssuedAt = time.Now().Unix()
	if v := str(r["userId"]); v != "" {
		next.UserID = v
	}
	if v := str(r["passToken"]); v != "" {
		next.PassToken = v
	}
	return next, nil
}
func (c *Client) exchange(ctx context.Context, credentials config.Credentials, username, password string) (config.Credentials, error) {
	cookies := map[string]string{"sdkVersion": "3.8.6", "deviceId": credentials.DeviceID, "userId": credentials.UserID, "passToken": credentials.PassToken}
	r, err := c.request(ctx, c.HTTP, "GET", c.AccountURL+"/pass/serviceLogin?sid=micoapi&_json=true", nil, cookies)
	if err != nil {
		return credentials, err
	}
	if str(r["location"]) == "" && username != "" && password != "" {
		hash := md5.Sum([]byte(password))
		data := url.Values{"_json": {"true"}, "sid": {"micoapi"}, "user": {username}, "hash": {strings.ToUpper(fmt.Sprintf("%x", hash))}, "qs": {str(r["qs"])}, "_sign": {str(r["_sign"])}, "callback": {str(r["callback"])}}
		r, err = c.request(ctx, c.HTTP, "POST", c.AccountURL+"/pass/serviceLoginAuth2", data, cookies)
		if err != nil {
			return credentials, err
		}
	}
	if str(r["notificationUrl"]) != "" || str(r["captchaUrl"]) != "" {
		return credentials, errors.New("小米要求验证码或二次验证，请使用米家扫码登录")
	}
	if number(r["code"]) != 0 {
		return credentials, fmt.Errorf("小米登录未成功（代码 %d），请重新扫码", number(r["code"]))
	}
	return c.finish(ctx, r, credentials)
}
func (c *Client) Login(ctx context.Context, username, password, cookie string) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	credentials := c.Store.Snapshot().Xiaomi
	if cookie != "" {
		credentials.ServiceToken = ""
		for _, part := range strings.Split(cookie, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok {
				switch k {
				case "userId":
					credentials.UserID = v
				case "passToken":
					credentials.PassToken = v
				}
			}
		}
		if credentials.UserID == "" || credentials.PassToken == "" {
			return errors.New("Cookie 需包含 userId 和 passToken")
		}
	}
	next, err := c.exchange(ctx, credentials, username, password)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return c.Store.Update(func(s *config.State) error { s.Xiaomi = next; return nil })
}
func (c *Client) refresh(ctx context.Context, force bool) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	credentials := c.Store.Snapshot().Xiaomi
	if !force && credentials.ServiceToken != "" && time.Since(time.Unix(credentials.IssuedAt, 0)) < 9*time.Hour {
		return nil
	}
	if time.Since(c.lastRefresh) < time.Minute {
		if credentials.ServiceToken != "" && !force {
			return nil
		}
		return errors.New("小米凭据续期冷却中，请稍后重试")
	}
	c.lastRefresh = time.Now()
	if credentials.UserID == "" || credentials.PassToken == "" {
		return errors.New("请先连接小米账号")
	}
	next, err := c.exchange(ctx, credentials, "", "")
	if err != nil {
		return err
	}
	return c.Store.Update(func(s *config.State) error { s.Xiaomi = next; return nil })
}
func (c *Client) call(ctx context.Context, path string, data url.Values) (map[string]any, error) {
	credentials := c.Store.Snapshot().Xiaomi
	if credentials.ServiceToken == "" {
		if err := c.refresh(ctx, false); err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		credentials = c.Store.Snapshot().Xiaomi
		uri := c.APIURL + path
		requestID := "app_ios_" + config.Random(15)
		if data == nil {
			sep := "?"
			if strings.Contains(uri, "?") {
				sep = "&"
			}
			uri += sep + "requestId=" + requestID
		} else {
			data.Set("requestId", requestID)
		}
		method := "GET"
		if data != nil {
			method = "POST"
		}
		r, err := c.request(ctx, c.HTTP, method, uri, data, map[string]string{"userId": credentials.UserID, "serviceToken": credentials.ServiceToken})
		if err != nil {
			return nil, err
		}
		if number(r["code"]) == 0 {
			return r, nil
		}
		if attempt == 0 && (number(r["code"]) == 401 || strings.Contains(strings.ToLower(str(r["message"])), "auth")) {
			if err = c.refresh(ctx, true); err != nil {
				return nil, err
			}
			continue
		}
		return nil, fmt.Errorf("小米接口失败（代码 %d）", number(r["code"]))
	}
	return nil, errors.New("小米认证失败")
}
func (c *Client) Devices(ctx context.Context) ([]config.Speaker, error) {
	r, err := c.call(ctx, "/admin/v2/device_list?master=0", nil)
	if err != nil {
		return nil, err
	}
	items, _ := r["data"].([]any)
	result := make([]config.Speaker, 0, len(items))
	for _, item := range items {
		d, ok := item.(map[string]any)
		if !ok {
			continue
		}
		did := str(d["miotDID"])
		if did == "" {
			did = str(d["deviceID"])
		}
		if did == "" {
			continue
		}
		result = append(result, config.Speaker{DID: did, DeviceID: str(d["deviceID"]), Name: str(d["name"]), Hardware: str(d["hardware"])})
	}
	return result, nil
}
func (c *Client) Ubus(ctx context.Context, device, method string, message any) (map[string]any, error) {
	b, _ := json.Marshal(message)
	r, err := c.call(ctx, "/remote/ubus", url.Values{"deviceId": {device}, "method": {method}, "path": {"mediaplayer"}, "message": {string(b)}})
	if err != nil {
		return nil, err
	}
	data, ok := r["data"].(map[string]any)
	if !ok {
		return nil, errors.New("音箱未返回操作结果")
	}
	if number(data["code"]) != 0 {
		return nil, fmt.Errorf("音箱拒绝操作（代码 %d）", number(data["code"]))
	}
	return data, nil
}
func (c *Client) Play(ctx context.Context, s config.Speaker, uri string) error {
	if s.Compatibility {
		_, err := c.Ubus(ctx, s.DeviceID, "player_play_url", map[string]any{"url": uri, "type": 2, "media": "app_ios"})
		return err
	}
	id := c.Store.Snapshot().AudioID
	music := map[string]any{"payload": map[string]any{"audio_type": "", "audio_items": []any{map[string]any{"item_id": map[string]any{"audio_id": id, "cp": map[string]any{"album_id": "-1", "episode_index": 0, "id": "355454500", "name": "xiaowei"}}, "stream": map[string]any{"url": uri}}}, "list_params": map[string]any{"listId": "-1", "loadmore_offset": 0, "origin": "xiaowei", "type": "MUSIC"}}, "play_behavior": "REPLACE_ALL"}
	b, _ := json.Marshal(music)
	_, err := c.Ubus(ctx, s.DeviceID, "player_play_music", map[string]any{"startaudioid": id, "music": string(b)})
	return err
}
func (c *Client) Operation(ctx context.Context, s config.Speaker, action string) error {
	_, err := c.Ubus(ctx, s.DeviceID, "player_play_operation", map[string]any{"action": action, "media": "app_ios"})
	return err
}
func (c *Client) Volume(ctx context.Context, s config.Speaker, volume int) error {
	_, err := c.Ubus(ctx, s.DeviceID, "player_set_volume", map[string]any{"volume": volume, "media": "app_ios"})
	return err
}
func (c *Client) Status(ctx context.Context, s config.Speaker) (map[string]any, error) {
	d, err := c.Ubus(ctx, s.DeviceID, "player_get_play_status", map[string]any{"media": "app_ios"})
	if err != nil {
		return nil, err
	}
	var info map[string]any
	dec := json.NewDecoder(strings.NewReader(str(d["info"])))
	dec.UseNumber()
	if err = dec.Decode(&info); err != nil {
		return nil, errors.New("音箱状态格式异常")
	}
	return info, nil
}
func (c *Client) Maintain(ctx context.Context) {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if c.Store.Snapshot().Xiaomi.UserID != "" {
				if err := c.refresh(ctx, false); err != nil {
					diagnostics.Event("warn", "小米账号", "凭据自动续期失败", map[string]string{"原因": err.Error(), "建议": "检查网络，必要时重新扫码登录"})
				}
			}
		}
	}
}

func (c *Client) Logout() error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.Store.Update(func(s *config.State) error {
		s.Xiaomi = config.Credentials{DeviceID: s.Xiaomi.DeviceID}
		s.Speakers = map[string]config.Speaker{}
		return nil
	})
}
