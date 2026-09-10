package xiaomi

import (
	"context"
	"errors"
	"github.com/BearHero520/miair-plus/internal/config"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type QRSession struct {
	ID       string `json:"session_id"`
	Image    string `json:"qrcode_url"`
	LoginURL string `json:"login_url"`
	State    string `json:"state"`
	Message  string `json:"message"`
	pollURL  string
	expires  time.Time
	cancel   context.CancelFunc
}
type QRManager struct {
	mu       sync.Mutex
	sessions map[string]*QRSession
	client   *Client
	ctx      context.Context
}

func NewQR(ctx context.Context, c *Client) *QRManager {
	return &QRManager{sessions: map[string]*QRSession{}, client: c, ctx: ctx}
}
func validAccountURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && (u.Hostname() == "account.xiaomi.com" || strings.HasSuffix(u.Hostname(), ".account.xiaomi.com"))
}
func (q *QRManager) Start(ctx context.Context) (QRSession, error) {
	q.mu.Lock()
	for id, s := range q.sessions {
		if time.Now().After(s.expires) {
			s.cancel()
			delete(q.sessions, id)
		}
	}
	if len(q.sessions) >= 4 {
		q.mu.Unlock()
		return QRSession{}, errors.New("扫码会话过多，请稍后再试")
	}
	q.mu.Unlock()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 35 * time.Second, Jar: jar}
	c := q.client
	r, err := c.request(ctx, client, "GET", c.AccountURL+"/pass/serviceLogin?sid=mijia&_json=true", nil, map[string]string{"sdkVersion": "3.8.6", "deviceId": c.Store.Snapshot().Xiaomi.DeviceID})
	if err != nil {
		return QRSession{}, err
	}
	if str(r["_sign"]) == "" {
		return QRSession{}, errors.New("小米未提供扫码参数")
	}
	params := url.Values{"sid": {"mijia"}, "_json": {"true"}, "_qrsize": {"240"}, "qs": {str(r["qs"])}, "_sign": {str(r["_sign"])}, "callback": {str(r["callback"])}}
	r, err = c.request(ctx, client, "GET", c.AccountURL+"/longPolling/loginUrl?"+params.Encode(), nil, nil)
	if err != nil {
		return QRSession{}, err
	}
	if !validAccountURL(str(r["lp"])) {
		return QRSession{}, errors.New("小米未提供有效扫码轮询地址")
	}
	sessionCtx, cancel := context.WithTimeout(q.ctx, 5*time.Minute)
	s := &QRSession{ID: config.Random(16), Image: str(r["qr"]), LoginURL: str(r["loginUrl"]), State: "waiting", Message: "使用米家 App 扫码并确认", pollURL: str(r["lp"]), expires: time.Now().Add(5 * time.Minute), cancel: cancel}
	q.mu.Lock()
	q.sessions[s.ID] = s
	result := *s
	q.mu.Unlock()
	go q.poll(sessionCtx, s, client)
	return result, nil
}
func (q *QRManager) set(s *QRSession, state, message string) {
	q.mu.Lock()
	s.State = state
	s.Message = message
	q.mu.Unlock()
}
func (q *QRManager) poll(ctx context.Context, s *QRSession, httpClient *http.Client) {
	defer s.cancel()
	for {
		if ctx.Err() != nil {
			q.set(s, "expired", "二维码已过期，请重新获取")
			return
		}
		r, err := q.client.request(ctx, httpClient, "GET", s.pollURL, nil, nil)
		if err != nil {
			select {
			case <-ctx.Done():
				q.set(s, "expired", "二维码已过期")
				return
			case <-time.After(time.Second):
				continue
			}
		}
		if number(r["code"]) != 0 {
			q.set(s, "expired", "二维码已失效，请重新获取")
			return
		}
		user, pass := str(r["userId"]), str(r["passToken"])
		if user == "" || pass == "" {
			select {
			case <-ctx.Done():
				continue
			case <-time.After(time.Second):
				continue
			}
		}
		if err = q.client.Login(ctx, "", "", "userId="+user+"; passToken="+pass); err != nil {
			q.set(s, "failed", err.Error())
			return
		}
		q.set(s, "confirmed", "小米账号已连接")
		return
	}
}
func (q *QRManager) Get(id string) (QRSession, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	s, ok := q.sessions[id]
	if !ok {
		return QRSession{}, false
	}
	return *s, true
}

func (q *QRManager) CancelAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, s := range q.sessions {
		s.cancel()
		delete(q.sessions, id)
	}
}
